/*
Copyright © 2026 Nehemia
*/
package cmd

import (

	// "path/filepath"
	"strings"

	// "fmt"
	"log"

	internalssh "coolify-tools/internal/ssh"
	"coolify-tools/internal/utils"

	"github.com/spf13/cobra"
)


var restoreDBCmd = &cobra.Command{
	Use:   "restoredb",
	Short: "restore all or a particular database from backups",
	Long: `Restores the data on all or a particular running db container using a local backup.
Assumes the database containers are already running on the target host.

Example:
  coolify-tools restoredb server.example.com ~/.ssh/id_ed25519 .coolify/20260524_120000`,
	Args: cobra.MinimumNArgs(3),
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

		// assumes containers are running. we cannot restore the dbs.
		// check?

		for _, db := range backupMetadata.Databases {
			dbBackupPath := backupDir + db.ArchiveName
			restoreDatabase(client, rawPrivateKey, db, dbBackupPath)
		}
		
	},
}

func init() {
	rootCmd.AddCommand(restoreDBCmd)
}
