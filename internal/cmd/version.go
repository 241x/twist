package cmd

import (
	"fmt"

	"github.com/spf13/cobra"
)

var Version = "v1.0.0"

var versionCmd = &cobra.Command{
	Use:   "version",
	Short: "Print the version information",
	Long:  `Print the version information for twist.`,
	Run: func(cmd *cobra.Command, args []string) {
		fmt.Println("twist " + Version)
	},
}

func init() {
	rootCmd.AddCommand(versionCmd)
}
