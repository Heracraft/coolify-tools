/*
Copyright © 2026 Nehemia <heracraft@teksafari.org>
*/
package cmd

import (
	"fmt"
	"log"

	"os"
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

var ouputFile string


func downloadEncryptedTarArchive(client *ssh.Client, signer ssh.Signer) {

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

	// ---
	timestamp := time.Now().Format("20060102_150405")
	backupFileName := ouputFile

	if ouputFile == "timestamp" {
		backupFileName = fmt.Sprintf("coolify-backup-%s.tar.gz.age", timestamp)
	}

	localFile, err := os.Create(backupFileName)
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

	cmd := "tar -czf - /data/coolify"

	if err := session.Run(cmd); err != nil {
		log.Fatalf("Backup failed: %v", err)
	}

	log.Println("Transfer complete!")

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

		downloadEncryptedTarArchive(client, signer)
	},
}

func init() {

	rootCmd.PersistentFlags().StringVarP(&username, "username", "u", "root", "Username(apart from root)")
	rootCmd.PersistentFlags().StringVarP(&sshPort, "port", "p", "22", "custom ssh port")
	rootCmd.PersistentFlags().StringVar(&passpharse, "passphrase", "", "private key passphrase")

	backupCmd.PersistentFlags().StringVarP(&ouputFile, "out", "o", "timestamp", "where to save archive")

	rootCmd.AddCommand(backupCmd)
}
