package cmd

import (
	"fmt"

	"github.com/spf13/cobra"
)

// panicCmd represents the panic command
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

	// Here you will define your flags and configuration settings.

	// Cobra supports Persistent Flags which will work for this command
	// and all subcommands, e.g.:
	// panicCmd.PersistentFlags().String("foo", "", "A help for foo")

	// Cobra supports local flags which will only run when this command
	// is called directly, e.g.:
	// panicCmd.Flags().BoolP("toggle", "t", false, "Help message for toggle")
}
