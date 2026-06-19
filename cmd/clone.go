/*
Copyright © 2026 Nehemia
*/
package cmd

import (
	"fmt"
	"log"
	"os"

	"github.com/spf13/cobra"
)

var keepBackup bool

var cloneCmd = &cobra.Command{
	Use:   "clone",
	Short: "Clone a Coolify instance or specific container from a source to a target host",
	Long: `Creates an encrypted backup of a Coolify instance (or specific container) from a source host,
then immediately restores that backup onto a target host, utilizing a single SSH key for both hosts.

Example:
  coolify-tools clone source.example.com target.example.com id_ed25519 [target-container]`,
	Args: cobra.MinimumNArgs(3),
	Run: func(cmd *cobra.Command, args []string) {
		sourceHost := args[0]
		targetHost := args[1]
		sshKey := args[2]
		var targetContainer string

		if len(args) == 4 {
			targetContainer = args[3]
		}

		fmt.Printf("Starting clone from %s to %s...\n", sourceHost, targetHost)

		backupDir, err := runBackup(sourceHost, sshKey, targetContainer)
		if err != nil {
			log.Fatalf("Clone failed during backup phase: %v", err)
		}

		if !keepBackup {
			defer func() {
				fmt.Printf("Cleaning up temporary backup directory: %s\n", backupDir)
				if err := os.RemoveAll(backupDir); err != nil {
					log.Printf("Warning: failed to clean up backup directory %s: %v", backupDir, err)
				}
			}()
		}

		err = runRestore(targetHost, sshKey, backupDir, targetContainer)
		if err != nil {
			log.Fatalf("Clone failed during restore phase: %v", err)
		}

		fmt.Println("Clone operation completed successfully!")
	},
}

func init() {
	cloneCmd.Flags().BoolVar(&keepBackup, "keep", true, "Retain the local backup archive after the clone completes")
	cloneCmd.Flags().BoolVar(&restoreClean, "clean", false, "Wipe existing data on target before restoration")
	rootCmd.AddCommand(cloneCmd)
}
