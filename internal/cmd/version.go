package cmd

import (
	"fmt"

	"github.com/spf13/cobra"
)

var versionCmd = &cobra.Command{
	Use:   "version",
	Short: "Print the version information",
	Long:  `Print the version information for twist.`,
	Run: func(cmd *cobra.Command, args []string) {
		fmt.Println("twist v0.1.0")
	},
}

func init() {
	rootCmd.AddCommand(versionCmd)
}
