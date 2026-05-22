/*
Copyright © 2026 Nehemia <nehemiahelibariki@gmail.com>
*/
package cmd

import (
	"encoding/json"
	"fmt"
	"log"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"golang.org/x/crypto/ssh"

	"filippo.io/age"
	"filippo.io/age/agessh"

	internalssh "coolify-tools/internal/ssh"
	"coolify-tools/internal/utils"
)

var username string
var sshPort string
var passpharse string

var ouputDir string

type Metadata struct {
	Timestamp      string         `json:"timestamp"`
	CoolifyVersion string         `json:"coolifyVersion"`
	Core           VolumeBackup   `json:"core"`
	Volumes        []VolumeBackup `json:"volumes"`
}

type VolumeBackup struct {
	Name          string `json:"name"`
	Image         string `json:"image"`
	ContainerName string `json:"containerName"`
	ArchiveName   string `json:"archiveName"`
	Destination   string `json:"destination"`
}

type Container struct {
	Name   string `json:"Name"`
	Config struct {
		Image string `json:"Image"`
	} `json:"Config"`
	Mounts []Mount `json:"Mounts"`
}

type Mount struct {
	Type        string `json:"Type"`
	Name        string `json:"Name"`
	Destination string `json:"Destination"`
}

func isDatabaseImage(image string) bool {
	image = strings.ToLower(image)

	dbKeywords := []string{
		"postgres", "mysql", "mariadb", "mongo", "redis", "clickhouse",
	}

	for _, kw := range dbKeywords {
		if strings.Contains(image, kw) {
			return true
		}
	}
	return false
}

func categorizeVolumes(containers []Container) (fileVolumes []Container, dbVolumes []Container) {
	fileVSet := make(map[string]Container)
	dbVSet := make(map[string]Container)

	for _, c := range containers {

		if isDatabaseImage(c.Config.Image) {
			dbVSet[c.Name] = c
		} else {
			fileVSet[c.Name] = c
		}
	}

	for containerName := range fileVSet {
		// TODO: Edge case: A volume shouldn't be backed up as a file if another container
		// identified it as a DB volume.
		fileVolumes = append(fileVolumes, fileVSet[containerName])
	}

	for containerName := range dbVSet {
		dbVolumes = append(dbVolumes, dbVSet[containerName])
	}

	return fileVolumes, dbVolumes
}

func getRunningContainers(client *ssh.Client) []Container {
	session, err := client.NewSession()
	utils.HandleErr("failed to start ssh session", err)

	out, err := session.Output("docker ps -q")
	utils.HandleErr("failed to run command", err, out)

	session.Close()

	containerIds := strings.Fields(string(out))

	if len(containerIds) == 0 {
		return nil
	}

	session, err = client.NewSession()
	defer session.Close()

	out, err = session.Output(fmt.Sprintf("docker inspect %s", strings.Join(containerIds, " ")))
	utils.HandleErr("Failed to inspect containers", err, "%s", out)

	var containers []Container

	if err := json.Unmarshal(out, &containers); err != nil {
		log.Fatalf("Failed to parse docker inspect output %v", err)
	}

	return containers
}

func streamToEncryptedTarArchive(client *ssh.Client, signer ssh.Signer, destination string, cmd string) {

	var recipient age.Recipient
	var err error

	switch signer.PublicKey().Type() {
	case "ssh-ed25519":
		recipient, err = agessh.NewEd25519Recipient(signer.PublicKey())

	case "ssh-rsa":
		recipient, err = agessh.NewRSARecipient(signer.PublicKey())
	default:
		log.Fatalf("Unsupported key type for age encryption: %s. Use Ed25519 or RSA.", signer.PublicKey())
	}

	utils.HandleErr("Failed to create age recipient from SSH key", err)

	localFile, err := os.Create(destination)
	utils.HandleErr("failed to read local path", err)
	defer localFile.Close()

	ageEncryptor, err := age.Encrypt(localFile, recipient)
	utils.HandleErr("Failed to init age", err)
	defer ageEncryptor.Close()

	session, err := client.NewSession()
	utils.HandleErr("failed to establish session", err)

	session.Stdout = ageEncryptor
	session.Stderr = os.Stderr

	// --

	if err := session.Run(cmd); err != nil {
		log.Fatalf("Backup failed: %v", err)
	}

	log.Println("Transfer complete for: ", destination)

}

// backupCmd represents the backup command
var backupCmd = &cobra.Command{
	Use:   "backup",
	Short: "Creates a full backup of a coolify instance into a .tar.xz.age file",
	Long: `Copies the contents of /data/coolify into a tar archive and encrypts it with a ssh key:

coolify-tools backup <hostname> <ssh-key>`,
	Args: cobra.MinimumNArgs(2),
	Run: func(cmd *cobra.Command, args []string) {
		// TODO: tie vars to args and validate them

		hostname := args[0]
		sshKey := args[1]

		client, signer := internalssh.EstablishConnection(username, hostname, sshKey, sshPort, passpharse)

		defer client.Close()

		fmt.Println(signer.PublicKey().Type())

		timestamp := time.Now().Format("20060102_150405")

		// TODO: set this to the actual version
		var metadata = Metadata{Timestamp: timestamp, CoolifyVersion: "v4.1.0"}

		if err := exec.Command("mkdir", "-p", ouputDir).Run(); err != nil {
			log.Fatal("failed to create dir %v", err)
		}

		backupDir := fmt.Sprintf("%s/%s", ouputDir, timestamp)

		// var backupDirCmd = fmt.Sprintf("mkdir -p %s", backupDir)

		if err := exec.Command("mkdir", "-p", backupDir).Run(); err != nil {
			log.Fatal("failed to create dir %v", err)
		}

		metadata.Core.ArchiveName = "core.tar.gz.age"
		metadata.Core.Destination = "/data/coolify/"

		streamToEncryptedTarArchive(client, signer, backupDir+"/core.tar.gz.age", "tar -czf - /data/coolify")

		runningContainers := getRunningContainers(client)

		if runningContainers == nil {
			fmt.Println("no running containers")
		}

		// TODO: skip the following except metadata stuff if running containers is nil
		fileVolumes, _ := categorizeVolumes(runningContainers)

		// '_' -> Db volumes are ignored for now.

		for _, vol := range fileVolumes {

			// var destinations []string
			cleanContainerName := strings.TrimPrefix(vol.Name, "/")

			for _, mount := range vol.Mounts {
				if mount.Type != "volume" {
					// this is a bind mount. we probably have it in /data/coolify/. skip.
					// TODO: handle this more robustly
					fmt.Println("Skipping mount: ", mount.Name, "-> ", mount.Destination)
					continue
				}

				remoteCmd := fmt.Sprintf("docker run --rm -v %s:/source:ro alpine tar -czf - -C /source .", mount.Name)

				// cleanName := strings.TrimPrefix(mount.Name, "/")
				archiveName := fmt.Sprintf("volume_%s.tar.gz.age", mount.Name)
				localPath := fmt.Sprintf("%s/%s", backupDir, archiveName)
				streamToEncryptedTarArchive(client, signer, localPath, remoteCmd)

				// destinations = append(destinations, mount.Destination)

				metadata.Volumes = append(metadata.Volumes, VolumeBackup{
					Name:          mount.Name,
					ContainerName: cleanContainerName,
					ArchiveName:   archiveName,
					Image:         vol.Config.Image,
					Destination:   mount.Destination,
				})
			}

		}

		fileData, err := json.MarshalIndent(metadata, "", "    ")
		utils.HandleErr("failed to marshal metadata", err)

		err = os.WriteFile(backupDir+"/metadata.json", fileData, 0644)
		utils.HandleErr("failed to write metadata file", err)
	},
}

func init() {

	rootCmd.PersistentFlags().StringVarP(&username, "username", "u", "root", "Username(apart from root)")
	rootCmd.PersistentFlags().StringVarP(&sshPort, "port", "p", "22", "custom ssh port")
	rootCmd.PersistentFlags().StringVar(&passpharse, "passphrase", "", "private key passphrase")

	backupCmd.PersistentFlags().StringVarP(&ouputDir, "out", "o", ".coolify", "where to save the archives. defaults to .coolify")

	rootCmd.AddCommand(backupCmd)
}
