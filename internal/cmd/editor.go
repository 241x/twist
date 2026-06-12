package cmd

import (
	"fmt"

	"github.com/241x/twist/internal/editor"
	"github.com/spf13/cobra"
)

var editorPort int

var editorCmd = &cobra.Command{
	Use:   "editor [config file]",
	Short: "Open the rule editor in your browser",
	Long:  `Start a web-based rule editor and open it in your default browser. Optionally load an existing config file.`,
	RunE: runEditor,
}

func init() {
	editorCmd.Flags().IntVarP(&editorPort, "port", "p", 9876, "Editor HTTP server port")
	rootCmd.AddCommand(editorCmd)
}

func runEditor(cmd *cobra.Command, args []string) error {
	srv := editor.NewServer(editorPort)

	fmt.Printf("twist editor started at http://127.0.0.1:%d\n", editorPort)
	fmt.Println("Press Ctrl+C to stop")

	return srv.Start()
}
