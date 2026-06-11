package app

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"text/tabwriter"

	"github.com/241x/twist/internal/log"
)

type Options struct {
	Host          string
	Port          int
	Launch        bool
	LaunchBrowser string
	LaunchArgs    []string
	URL           string
	ConfigFile    string
	ConfigData    []byte
	ListTargets   bool
	Target        string
	Verbose       bool
	Timeout       int
}

type App struct {
	opts      Options
	signals   chan os.Signal
	browser   *Browser
	cdp       *CDP
	config    *Config
	target    *Target
	intercept *Intercept
}

func New(opts Options) *App {
	return &App{
		opts:    opts,
		signals: make(chan os.Signal, 1),
	}
}

func (a *App) Run(ctx context.Context) error {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	signal.Notify(a.signals, os.Interrupt)
	go func() {
		select {
		case <-a.signals:
			cancel()
		case <-ctx.Done():
		}
	}()

	if a.opts.ListTargets {
		return a.runListTargets(ctx)
	}

	return a.runIntercept(ctx)
}

func (a *App) Shutdown() {
	signal.Stop(a.signals)
	close(a.signals)
}

func (a *App) runListTargets(ctx context.Context) error {
	logger := log.FromContext(ctx)

	a.cdp = NewCDP(a.opts.Host, a.opts.Port, a.opts.Timeout, a.opts.Verbose)

	if err := a.cdp.Connect(ctx); err != nil {
		return fmt.Errorf("failed to connect to CDP: %w", err)
	}
	defer a.cdp.Close()

	targets, err := a.cdp.ListTargets(ctx)
	if err != nil {
		return fmt.Errorf("failed to list targets: %w", err)
	}

	logger.Info().Int("count", len(targets)).Msg("targets listed")
	printTargets(targets)
	return nil
}

func (a *App) runIntercept(ctx context.Context) error {
	logger := log.FromContext(ctx)

	cfg, err := LoadConfig(a.opts.ConfigFile, a.opts.ConfigData)
	if err != nil {
		return err
	}
	a.config = cfg
	logger.Info().Str("name", cfg.Name).Int("rules", len(cfg.Rules)).Msg("config loaded")

	host := a.opts.Host
	if a.opts.Launch {
		host = "127.0.0.1"
	}

	browser := NewBrowser(a.opts.Launch, a.opts.Port)
	if err := browser.Start(ctx, a.opts.LaunchBrowser, a.opts.LaunchArgs, a.opts.URL); err != nil {
		return fmt.Errorf("failed to start browser: %w", err)
	}
	a.browser = browser

	if browser.IsLaunched() {
		defer browser.Stop()
		logger.Info().Str("browser", a.opts.LaunchBrowser).Int("port", a.opts.Port).Msg("browser launched")
	}

	a.cdp = NewCDP(host, a.opts.Port, a.opts.Timeout, a.opts.Verbose)
	if err := a.cdp.Connect(ctx); err != nil {
		return fmt.Errorf("failed to connect to CDP: %w", err)
	}
	defer a.cdp.Close()

	a.target = NewTarget(a.cdp)

	selectURL := a.opts.URL
	if a.opts.Launch {
		selectURL = ""
	}

	selected, err := a.target.Select(ctx, a.opts.Target, selectURL)
	if err != nil {
		return fmt.Errorf("failed to select target: %w", err)
	}
	logger.Info().Str("id", selected.ID).Str("url", selected.URL).Msg("target selected")

	if err := a.cdp.AttachToTarget(ctx, selected.ID); err != nil {
		return fmt.Errorf("failed to attach to target: %w", err)
	}

	a.cdp.CloseBrowser()

	if a.opts.Target != "" && a.opts.URL != "" && !a.opts.Launch {
		if err := a.cdp.NavigateTo(ctx, a.opts.URL); err != nil {
			return fmt.Errorf("failed to navigate to URL: %w", err)
		}
		logger.Info().Str("url", a.opts.URL).Msg("navigated")
	}

	a.intercept = NewIntercept(a.cdp, a.config)
	logger.Info().Msg("interception started")

	return a.intercept.Start(ctx)
}

func printTargets(targets []CDPTarget) {
	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "ID\tTYPE\tURL\tTITLE")
	for _, t := range targets {
		u := t.URL
		if len(u) > 80 {
			u = u[:77] + "..."
		}
		fmt.Fprintf(w, "%s\t%s\t%s\t%s\n", t.ID, t.Type, u, t.Title)
	}
	w.Flush()
}
