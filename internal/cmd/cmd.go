package cmd

import (
	"context"
	"fmt"
	"io"
	"os"

	"github.com/241x/twist/internal/app"
	"github.com/spf13/cobra"
)

var (
	cdpHost        string
	cdpPort        int
	launch         bool
	launchBrowser  string
	launchArgs     []string
	targetURL      string
	configFile     string
	listTargets    bool
	target         string
	verbose        bool
	timeout        int
)

var rootCmd = &cobra.Command{
	Use:   "twist",
	Short: "Intercept and modify browser network requests and responses via CDP",
	Long: `twist connects to a browser's Chrome DevTools Protocol (CDP) endpoint
to intercept, inspect, and modify network requests and responses in real time.`,
	RunE: runRoot,
}

func init() {
	rootCmd.PersistentFlags().StringVarP(&cdpHost, "host", "H", "127.0.0.1", "Browser CDP listening address")
	rootCmd.PersistentFlags().IntVarP(&cdpPort, "port", "p", 9222, "Browser CDP debugging port")
	rootCmd.PersistentFlags().BoolVar(&launch, "launch", false, "Auto-launch a new browser instance with default settings")
	rootCmd.PersistentFlags().StringVar(&launchBrowser, "launch-browser", "chrome", "Browser type to launch (chrome, chromium, edge)")
	rootCmd.PersistentFlags().StringArrayVar(&launchArgs, "launch-args", nil, "Extra arguments passed to the browser on launch")
	rootCmd.PersistentFlags().StringVarP(&targetURL, "url", "u", "", "URL to open in the browser")
	rootCmd.PersistentFlags().StringVarP(&configFile, "config", "c", "", "Path to the rule configuration file")
	rootCmd.PersistentFlags().BoolVar(&listTargets, "list-targets", false, "List all available browser tab targets")
	rootCmd.PersistentFlags().StringVarP(&target, "target", "t", "", "Attach to a specific tab target by ID")
	rootCmd.PersistentFlags().BoolVarP(&verbose, "verbose", "v", false, "Enable verbose logging output")
	rootCmd.PersistentFlags().IntVar(&timeout, "timeout", 30, "CDP connection timeout in seconds")
}

func runRoot(cmd *cobra.Command, args []string) error {
	opts := app.Options{
		Host:          cdpHost,
		Port:          cdpPort,
		Launch:        launch,
		LaunchBrowser: launchBrowser,
		LaunchArgs:    launchArgs,
		URL:           targetURL,
		ConfigFile:    configFile,
		ListTargets:   listTargets,
		Target:        target,
		Verbose:       verbose,
		Timeout:       timeout,
	}

	if !listTargets {
		configData, err := resolveConfig()
		if err != nil {
			return err
		}
		opts.ConfigData = configData
	}

	a := app.New(opts)
	defer a.Shutdown()

	return a.Run(context.Background())
}

func resolveConfig() ([]byte, error) {
	if configFile != "" {
		return os.ReadFile(configFile)
	}

	if isStdinPiped() {
		data, err := io.ReadAll(os.Stdin)
		if err != nil {
			return nil, fmt.Errorf("failed to read config from stdin: %w", err)
		}
		if len(data) > 0 {
			return data, nil
		}
	}

	for _, name := range []string{".twist.json", "twist.json"} {
		if data, err := os.ReadFile(name); err == nil {
			return data, nil
		}
	}

	return nil, &ExitError{Code: 1, Msg: "no config file specified: use --config, pipe config via stdin, or place .twist.json / twist.json in the current directory"}
}

func isStdinPiped() bool {
	stat, err := os.Stdin.Stat()
	if err != nil {
		return false
	}
	return (stat.Mode() & os.ModeCharDevice) == 0
}

func Execute() error {
	return rootCmd.Execute()
}
