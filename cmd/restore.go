/*
Copyright © 2026 Nehemia
*/
package cmd

import (
	"crypto/ed25519"
	"crypto/rsa"
	"encoding/json"
	"io"
	"path/filepath"
	"strings"

	"fmt"
	"log"
	"os"

	"filippo.io/age"
	"filippo.io/age/agessh"
	"golang.org/x/crypto/ssh"

	internalssh "coolify-tools/internal/ssh"
	"coolify-tools/internal/utils"

	"github.com/spf13/cobra"
)

var restoreClean bool

func getDecryptReader(client *ssh.Client, rawPrivateKey interface{}, localPath string) (io.Reader, *os.File) {
	var ageIdentity age.Identity

	if !utils.Exists(localPath) {
		log.Fatalf("Local path %s does not exist", localPath)
	}

	file, err := os.Open(localPath)
	utils.HandleErr("failed to read local path", err)

	switch k := rawPrivateKey.(type) {
	case *rsa.PrivateKey:
		ageIdentity, err = agessh.NewRSAIdentity(k)
		utils.HandleErr("failed to create age identity", err)
	case *ed25519.PrivateKey:
		ageIdentity, err = agessh.NewEd25519Identity(*k)
		utils.HandleErr("failed to create age identity", err)

	default:
		log.Fatalf("Unsupported key type: %T", k)

	}

	decryptedReader, err := age.Decrypt(file, ageIdentity)
	utils.HandleErr("Failed to create age decrypt reader", err)

	return decryptedReader, file
}

func readBackupMetadata(backupDir string) Metadata {
	file, err := os.ReadFile(backupDir + "metadata.json")

	utils.HandleErr("Failed to read backup metadata: ", err)

	var metadata Metadata
	if err := json.Unmarshal(file, &metadata); err != nil {
		log.Fatalf("Failed to parse metadata as json: %v", err)
	}
	return metadata
}

func streamDecryptedArchive(client *ssh.Client, rawPrivateKey interface{}, cmd string, localPath string) {

	session, err := client.NewSession()
	utils.HandleErr("Failed to create ssh session", err)
	defer session.Close()

	decryptedReader, file := getDecryptReader(client, rawPrivateKey, localPath)
	defer file.Close()

	session.Stdin = decryptedReader
	session.Stdout = os.Stdout
	session.Stderr = os.Stderr

	log.Printf("Decrypting and streaming %s", localPath)

	if err := session.Run(cmd); err != nil {
		log.Fatalf("Failed to run cmd %s: %v", cmd, err)
	}
}

func restoreDatabase(client *ssh.Client, rawPrivateKey interface{}, dbMetadata DatabaseBackup, localPath string) {
	session, err := client.NewSession()
	utils.HandleErr("Failed to create ssh session", err)
	defer session.Close()

	decryptedReader, file := getDecryptReader(client, rawPrivateKey, localPath)
	defer file.Close()

	session.Stdin = decryptedReader
	session.Stdout = os.Stdout
	session.Stderr = os.Stderr

	var remoteCmd string

	switch dbMetadata.Engine {
	case EnginePostgres:
		remoteCmd = fmt.Sprintf(`docker exec -i %s sh -c 'psql -U ${POSTGRES_USER:-postgres}'`, dbMetadata.ContainerName)
	case EngineMysql:
		remoteCmd = fmt.Sprintf(`docker exec -i %s sh -c 'mysql -u root -p"${MYSQL_ROOT_PASSWORD}"'`, dbMetadata.ContainerName)
	case EngineRedis:
		// Redis requires writing to disk and restarting
		remoteCmd = fmt.Sprintf(`cat > /tmp/%s.rdb && docker cp /tmp/%s.rdb %s:/data/dump.rdb && docker restart %s`,
			dbMetadata.ContainerName, dbMetadata.ContainerName, dbMetadata.ContainerName, dbMetadata.ContainerName)
	default:
		log.Fatalf("unsupported database engine: %s", dbMetadata.Engine)
	}

	log.Printf("Restoring %s", dbMetadata.ContainerName)

	if err := session.Run(remoteCmd); err != nil {
		log.Fatalf("Failed to run cmd %s: %v", remoteCmd, err)
	}
}

// TODO: Review function below. Mostly slop. 
func restoreSSHKeys(client *ssh.Client) {
	log.Println("Restoring Coolify SSH keys to authorized_keys...")

	runCmd := func(cmd string) (string, error) {
		session, err := client.NewSession()
		if err != nil {
			return "", err
		}
		defer session.Close()
		out, err := session.Output(cmd)
		return string(out), err
	}

	// Ensure ~/.ssh and authorized_keys exist
	_, err := runCmd("mkdir -p ~/.ssh && touch ~/.ssh/authorized_keys && chmod 700 ~/.ssh && chmod 600 ~/.ssh/authorized_keys")
	if err != nil {
		log.Printf("Failed to set up .ssh directory: %v", err)
	}

	// List files in ssh keys directory
	out, err := runCmd("ls -1 /data/coolify/ssh/keys/ 2>/dev/null || true")
	if err != nil {
		log.Printf("Failed to list ssh keys: %v", err)
		return
	}

	files := strings.Split(strings.TrimSpace(out), "\n")

	// Get existing authorized keys
	authKeysOut, _ := runCmd("cat ~/.ssh/authorized_keys")
	authorizedKeys := authKeysOut

	for _, file := range files {
		file = strings.TrimSpace(file)
		if file == "" || strings.HasSuffix(file, ".lock") {
			continue
		}

		keyPath := fmt.Sprintf("/data/coolify/ssh/keys/%s", file)

		// Check if it's a file
		isDirOut, _ := runCmd(fmt.Sprintf("if [ -f %s ]; then echo 'yes'; else echo 'no'; fi", keyPath))
		if strings.TrimSpace(isDirOut) != "yes" {
			continue
		}

		runCmd(fmt.Sprintf("chmod 600 %s", keyPath))

		pubKeyOut, err := runCmd(fmt.Sprintf("ssh-keygen -y -f %s 2>/dev/null", keyPath))
		if err == nil {
			pubKey := strings.TrimSpace(pubKeyOut)
			if pubKey != "" && !strings.Contains(authorizedKeys, pubKey) {
				runCmd(fmt.Sprintf("echo '%s' >> ~/.ssh/authorized_keys", pubKey))
				log.Printf("Added %s to authorized_keys", keyPath)
				authorizedKeys += "\n" + pubKey
			}
		}
	}
}

var restoreCmd = &cobra.Command{
	Use:   "restore",
	Short: "restore your coolify backups to a target machine",
	Long: `Runs a restore against a target server using a local backup
	coolify-tools restore <hostname> <sshKey> <backupDir>
	`,
	Args: cobra.ExactArgs(3),
	Run: func(cmd *cobra.Command, args []string) {
		hostname := args[0]
		sshKey := args[1]

		backupDir := args[2]

		if !utils.Exists(backupDir) {
			log.Fatalf("backup dir specified does not exist")
		}
		if !strings.HasSuffix(backupDir, "/") {
			backupDir = backupDir + "/"
		}

		backupMetadata := readBackupMetadata(backupDir)

		client, _ := internalssh.EstablishConnection(username, hostname, sshKey, sshPort, passphrase)
		rawPrivateKey := internalssh.GetRawPrivateKey(sshKey, passphrase)
		defer client.Close()

		utils.InstallCoolify(client, backupMetadata.CoolifyVersion)
		fmt.Println("Stopping Coolify services...")
		stopSession, err := client.NewSession()
		utils.HandleErr("Failed to create ssh session to stop coolify", err)

		stopSession.Stdout = os.Stdout
		stopSession.Stderr = os.Stderr
		// TODO: make this configurable. --verbose?

		err = stopSession.Run("docker stop coolify coolify-redis") // Notice how coolify-db is not stopped
		utils.HandleErr("failed to stop coolify containers", err)
		stopSession.Close()

		if restoreClean {
			fmt.Println("Cleaning existing data...")
			cleanSession, err := client.NewSession()
			utils.HandleErr("Failed to create ssh session for cleaning", err)
			// We use rm -rf but ignore errors if files don't exist
			cleanSession.Run("rm -rf /data/coolify/*")
			cleanSession.Close()

			for _, vol := range backupMetadata.Volumes {
				volCleanSession, err := client.NewSession()
				if err == nil {
					cleanCmd := fmt.Sprintf("docker run --rm -v %s:/target alpine sh -c 'rm -rf /target/*'", vol.Name)
					volCleanSession.Run(cleanCmd)
					volCleanSession.Close()
				}
			}
		}

		streamDecryptedArchive(client, rawPrivateKey, "tar -xzf - -C /", backupDir+backupMetadata.Config.ArchiveName)

		restoreSSHKeys(client)

		for _, vol := range backupMetadata.Volumes {
			volPath := backupDir + vol.ArchiveName

			restoreVolCmd := fmt.Sprintf("docker run -i --rm -v %s:/target alpine tar -xzf - -C /target", vol.Name)

			streamDecryptedArchive(client, rawPrivateKey, restoreVolCmd, volPath)

		}

		// containers are not running. we cannot restore the dbs.

		// for _, db := range backupMetadata.Databases {
		// 	dbBackupPath := backupDir + db.ArchiveName
		// 	restoreDatabase(client, rawPrivateKey, db, dbBackupPath)
		// }

		coreDBPath := filepath.Join(backupDir, backupMetadata.CoreDB.ArchiveName)
		restoreDatabase(client, rawPrivateKey, backupMetadata.CoreDB, coreDBPath)


		startCoolify := utils.YesNoPrompt("Start coolify?", true)

		if startCoolify {
			fmt.Println("Starting Coolify services...")
			startSession, err := client.NewSession()
			utils.HandleErr("Failed to create ssh session to start coolify", err)

			startSession.Stderr = os.Stderr
			startSession.Stdout = os.Stdout

			startSession.Run("curl -fsSL https://cdn.coollabs.io/coolify/install.sh | bash")

			defer startSession.Close()

		}
	},
}

func init() {
	restoreCmd.Flags().BoolVar(&restoreClean, "clean", false, "Wipe existing data before restoration")
	rootCmd.AddCommand(restoreCmd)
}
