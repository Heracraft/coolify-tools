package cmd

import (
	"os"

	"github.com/spf13/cobra"
)

var username string
var sshPort string
var passphrase string

var rootCmd = &cobra.Command{
	Use:   "coolify-cli",
	Short: "tooling for using coolify in prod. Starting with instance wide backups",
	Long:  `Tools for those deploying coolify in production. For now that's instance wide backups encrypted with an SSH key using age`,
}

func Execute() {
	err := rootCmd.Execute()
	if err != nil {
		os.Exit(1)
	}
}

func init() {
	rootCmd.PersistentFlags().StringVarP(&username, "username", "u", "root", "Username(apart from root)")
	rootCmd.PersistentFlags().StringVarP(&sshPort, "port", "p", "22", "custom ssh port")
	rootCmd.PersistentFlags().StringVar(&passphrase, "passphrase", "", "private key passphrase")

	rootCmd.Flags().BoolP("toggle", "t", false, "Help message for toggle")
}
