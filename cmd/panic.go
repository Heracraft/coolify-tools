package cmd

import (
	"fmt"

	"github.com/spf13/cobra"
)

var panicCmd = &cobra.Command{
	Use:   "panic",
	Short: "Restore on autopilot, no questions just try all defaults to restore an instance. useful when you are loosing your shit",
	Long: ``,
	Run: func(cmd *cobra.Command, args []string) {
		fmt.Println("panic called")
	},
}

func init() {
	rootCmd.AddCommand(panicCmd)
}
