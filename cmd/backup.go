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

	"coolify-tools/internal/docker"
	internalssh "coolify-tools/internal/ssh"
	"coolify-tools/internal/utils"
)

var ouputDir string
var clean bool

type Metadata struct {
	Timestamp      string           `json:"timestamp"`
	CoolifyVersion string           `json:"coolifyVersion"`
	CoreVolume     VolumeBackup     `json:"coreVolume"`
	CoreDB         DatabaseBackup   `json:"coreDB"`
	Volumes        []VolumeBackup   `json:"volumes"`
	Databases      []DatabaseBackup `json:"databases"`
}

type VolumeBackup struct {
	Name          string `json:"name"`
	Image         string `json:"image"`
	ContainerName string `json:"containerName"`
	ArchiveName   string `json:"archiveName"`
	Destination   string `json:"destination"`
}

type DatabaseBackup struct {
	ContainerName string          `json:"containerName"`
	Engine        docker.DBEngine `json:"engine"`
	ArchiveName   string          `json:"archiveName"`
}

func getRunningContainers(client *ssh.Client, targetContainer string) []docker.Container {
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
	utils.HandleErr("failed to start ssh session", err)
	defer session.Close()

	out, err = session.Output(fmt.Sprintf("docker inspect %s", strings.Join(containerIds, " ")))
	utils.HandleErr("Failed to inspect containers", err, "%s", out)

	var containers []docker.Container

	if err := json.Unmarshal(out, &containers); err != nil {
		log.Fatalf("Failed to parse docker inspect output %v", err)
	}

	if targetContainer != "" {
		var filteredContainers []docker.Container

		for _, container := range containers {
			isNameMatch := container.Name == targetContainer || strings.Contains(container.Name, targetContainer)

			isIDMatch := container.ID == targetContainer || strings.HasPrefix(container.ID, targetContainer)

			composeProject := container.Config.Labels["com.docker.compose.project"]
			composeService := container.Config.Labels["com.docker.compose.service"]

			isComposeMatch := composeProject == targetContainer || composeService == targetContainer

			if isNameMatch || isIDMatch || isComposeMatch {
				// targetContainer can be the container id, name, compose project/service
				filteredContainers = append(filteredContainers, container)
			}
		}

		return filteredContainers
	}

	return containers
}

func streamToEncryptedTarArchive(client *ssh.Client, signer ssh.Signer, destination string, cmd string) error {

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
	utils.HandleErr("failed to create local path", err)
	defer localFile.Close()

	ageEncryptor, err := age.Encrypt(localFile, recipient)
	utils.HandleErr("Failed to init age", err)
	defer ageEncryptor.Close()

	session, err := client.NewSession()
	utils.HandleErr("failed to establish session", err)
	defer session.Close()

	session.Stdout = ageEncryptor
	session.Stderr = os.Stderr

	// --

	if err := session.Run(cmd); err != nil {
		// log.Println("Backup failed: %v", err)
		// deal with corrupted archive that is created
		return err
	}

	log.Println("Transfer complete for: ", destination)
	return nil
}

// backupCmd represents the backup command
var backupCmd = &cobra.Command{
	Use:   "backup",
	Short: "Creates an encrypted backup of a specified running container or Coolify core and all volumes of running containers",
	Long: `Connects to a Coolify instance, archives the /data/coolify directory alongside volumes, and encrypts everything locally with an SSH key. Or only a specific container. 

Example:
  coolify-tools backup <hostname> <path-to-ssh-key> <target-container?>
  coolify-tools backup server.example.com ~/.ssh/id_ed25519`,
	Args: cobra.MinimumNArgs(2),
	Run: func(cmd *cobra.Command, args []string) {
		// TODO: validate args
		var targetContainer string

		hostname := args[0]
		sshKey := args[1]
		if len(args) == 3 {
			targetContainer = args[2]
		}

		client, signer := internalssh.EstablishConnection(username, hostname, sshKey, sshPort, passphrase)

		defer client.Close()

		if err := exec.Command("mkdir", "-p", ouputDir).Run(); err != nil {
			// TODO: replace mkdir -p with os.MkdirAll(backupDir, 0755)
			log.Fatalf("failed to create dir %v", err)
		}

		timestamp := time.Now().Format("20060102_150405")
		coolifyVersion := utils.GetCoolifyVersion(client)
		var metadata = Metadata{Timestamp: timestamp, CoolifyVersion: coolifyVersion}

		backupDir := fmt.Sprintf("%s/%s", ouputDir, timestamp)

		if err := exec.Command("mkdir", "-p", backupDir).Run(); err != nil {
			log.Fatalf("failed to create dir %v", err)
		}

		// -------
		runningContainers := getRunningContainers(client, targetContainer)

		if runningContainers == nil {
			fmt.Println("no running containers")
		}

		if targetContainer == "" {
			// only backup core coolify core when doing a full sys backup
			metadata.CoreVolume.ArchiveName = "core.tar.gz.age"
			metadata.CoreVolume.Destination = "/data/coolify/"
			metadata.CoreVolume.ContainerName = "coolify"

			backupErr := streamToEncryptedTarArchive(client, signer, backupDir+"/core.tar.gz.age", "tar -czf - /data/coolify")
			if backupErr != nil {
				log.Printf("Backup failed for %s: %v", "/data/coolify", backupErr)
			}
		}

		fileVolumes, dbVolumes := docker.CategorizeVolumes(runningContainers)

		fmt.Printf("\nBacking up %d volumes\n", len(fileVolumes)+len(dbVolumes))

		for _, vol := range fileVolumes {

			// var destinations []string
			cleanContainerName := strings.TrimPrefix(vol.Name, "/")

			for _, mount := range vol.Mounts {
				if strings.Contains(mount.Destination, "/data/coolify") || mount.Type != "volume" {
					// this is a bind mount. we probably have it in /data/coolify/. skip.
					// TODO: handle this more robustly
					fmt.Println("Skipping mount: ", mount.Name, "-> ", mount.Destination)
					continue
				}

				remoteCmd := fmt.Sprintf("docker run --rm -v %s:/source:ro alpine tar -czf - -C /source .", mount.Name)

				archiveName := fmt.Sprintf("volume_%s.tar.gz.age", mount.Name)
				localPath := fmt.Sprintf("%s/%s", backupDir, archiveName)

				backupErr := streamToEncryptedTarArchive(client, signer, localPath, remoteCmd)
				if backupErr != nil {
					log.Printf("Backup failed for %s: %v", mount.Name, backupErr)
				}

				metadata.Volumes = append(metadata.Volumes, VolumeBackup{
					Name:          mount.Name,
					ContainerName: cleanContainerName,
					ArchiveName:   archiveName,
					Image:         vol.Config.Image,
					Destination:   mount.Destination,
				})
			}

		}

		for _, db := range dbVolumes {
			cleanDbName := strings.TrimPrefix(db.Name, "/")
			var engine docker.DBEngine

			for _, kw := range docker.DBKeywords {
				if strings.Contains(db.Config.Image, strings.ToLower(string(kw))) {
					engine = kw
				}
			}

			var extension, remoteCmd string

			// TODO: compress db dumps before archiving
			switch engine {
			case docker.EngineMongo:
				fmt.Printf("Skipping MongoDB database: %s\n", cleanDbName)
				continue
			case docker.EngineMariadb:
				fmt.Printf("Skipping MariaDB database: %s\n", cleanDbName)
				continue
			case docker.EnginePostgres:
				cleanFlag := ""
				if clean {
					cleanFlag = "--clean "
				}
				remoteCmd = fmt.Sprintf(`docker exec %s sh -c 'pg_dumpall %s-U ${POSTGRES_USER:-postgres}'`, cleanDbName, cleanFlag)
				extension = "sql"
			case docker.EngineMysql:
				cleanFlag := ""
				if clean {
					cleanFlag = "--add-drop-database --add-drop-table "
				}
				remoteCmd = fmt.Sprintf(`docker exec %s sh -c 'mysqldump %s-u root -p"${MYSQL_ROOT_PASSWORD}" --single-transaction --routines --triggers --all-databases'`, cleanDbName, cleanFlag)
				extension = "sql"
			case docker.EngineRedis:
				// TODO: look into REPLCONF capa error: NOAUTH Authentication required.
				// use a common env like REDIS_PASSWORD? or just ignore?

				// Native command to force an RDB snapshot and stream it to stdout
				// remoteCmd = fmt.Sprintf(`docker exec %s redis-cli --rdb /dev/stdout`, cleanDbName)
				// extension = "rdb"
				fmt.Printf("Skipping Redis: %s\n", cleanDbName)
				continue

			default:
				fmt.Printf("unsupported db %s, skipping", db.Name)
				continue
			}

			archiveName := fmt.Sprintf("db_%s.%s.age", cleanDbName, extension)

			localPath := fmt.Sprintf("%s/%s", backupDir, archiveName)

			backupErr := streamToEncryptedTarArchive(client, signer, localPath, remoteCmd)
			if backupErr != nil {
				log.Printf("Backup failed for %s: %v", cleanDbName, backupErr)
			}

			if cleanDbName != "coolify-db" {
				metadata.Databases = append(metadata.Databases, DatabaseBackup{
					ContainerName: cleanDbName,
					Engine:        engine,
					ArchiveName:   archiveName,
				})
			} else {
				metadata.CoreDB = DatabaseBackup{
					ContainerName: db.Name,
					Engine:        docker.EnginePostgres,
					ArchiveName:   archiveName,
				}
			}
		}

		fileData, err := json.MarshalIndent(metadata, "", "    ")
		utils.HandleErr("failed to marshal metadata", err)

		err = os.WriteFile(backupDir+"/metadata.json", fileData, 0644)
		utils.HandleErr("failed to write metadata file", err)
	},
}

func init() {
	backupCmd.PersistentFlags().StringVarP(&ouputDir, "out", "o", ".coolify", "where to save the archives. defaults to .coolify")
	backupCmd.PersistentFlags().BoolVar(&clean, "clean", false, "Include clean-up instructions (like DROP TABLE) in database dumps")

	rootCmd.AddCommand(backupCmd)
}
