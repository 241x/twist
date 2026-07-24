// Package cmd 提供 twist CLI 的命令定义、参数解析和配置加载。
package cmd

import (
	"context"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"github.com/241x/twist/internal/app"
	"github.com/241x/twist/internal/log"
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
	observe        bool
	observeCount   int
	observeDuration string
	observeFullBody bool
	observeFilter  []string
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
	rootCmd.PersistentFlags().IntVar(&timeout, "timeout", 15, "CDP connection timeout in seconds")

	rootCmd.PersistentFlags().BoolVar(&observe, "observe", false, "Observe network requests and responses, output JSONL to stdout")
	rootCmd.PersistentFlags().IntVar(&observeCount, "observe-count", 0, "Exit after observing N events (0 = unlimited)")
	rootCmd.PersistentFlags().StringVar(&observeDuration, "observe-duration", "", "Exit after observing for a duration (e.g. 30s, 5m)")
	rootCmd.PersistentFlags().BoolVar(&observeFullBody, "observe-full-body", false, "Include full response body without truncation")
	rootCmd.PersistentFlags().StringArrayVar(&observeFilter, "observe-filter", nil, "Filter observed events (repeatable, format: key=value1,value2)")
}

func runRoot(cmd *cobra.Command, args []string) error {
	log.Init(verbose)

	if observe && configFile != "" {
		return &ExitError{Code: 1, Msg: "--observe and --config are mutually exclusive"}
	}
	if observe && listTargets {
		return &ExitError{Code: 1, Msg: "--observe and --list-targets are mutually exclusive"}
	}
	if !observe && (observeCount > 0 || observeDuration != "" || observeFullBody || len(observeFilter) > 0) {
		return &ExitError{Code: 1, Msg: "--observe-count, --observe-duration, --observe-full-body, and --observe-filter require --observe"}
	}

	var dur time.Duration
	if observeDuration != "" {
		var err error
		dur, err = time.ParseDuration(observeDuration)
		if err != nil {
			return &ExitError{Code: 1, Msg: fmt.Sprintf("invalid --observe-duration: %v", err)}
		}
	}

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
		Observe: app.ObserveOptions{
			Enabled:  observe,
			Count:    observeCount,
			Duration: dur,
			FullBody: observeFullBody,
			Filter:   parseObserveFilter(observeFilter),
		},
	}

	if !listTargets && !observe {
		configData, err := resolveConfig()
		if err != nil {
			return err
		}
		opts.ConfigData = configData
	}

	ctx := log.WithContext(context.Background())

	a := app.New(opts)
	defer a.Shutdown()

	return a.Run(ctx)
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

	return nil, &ExitError{Code: 1, Msg: "no mode specified: use --observe to watch network requests, --config to modify them, or --list-targets to list tabs"}
}

func isStdinPiped() bool {
	stat, err := os.Stdin.Stat()
	if err != nil {
		return false
	}
	return (stat.Mode() & os.ModeCharDevice) == 0
}

// parseObserveFilter 将原始 filter 标志值解析为 ObserveFilter 结构。
// 格式：key=value1,value2，多次重复合并。
func parseObserveFilter(raw []string) app.ObserveFilter {
	var f app.ObserveFilter
	for _, item := range raw {
		key, vals, ok := strings.Cut(item, "=")
		if !ok || vals == "" {
			continue
		}
		parts := strings.Split(vals, ",")
		switch key {
		case "url":
			f.URLs = append(f.URLs, parts...)
		case "type":
			f.Types = append(f.Types, parts...)
		}
	}
	return f
}

func Execute() error {
	return rootCmd.Execute()
}
