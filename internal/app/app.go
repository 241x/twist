package app

import (
	"context"
	"fmt"
	"os"
	"os/signal"
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
	a.cdp = NewCDP(a.opts.Host, a.opts.Port, a.opts.Timeout, a.opts.Verbose)

	if err := a.cdp.Connect(ctx); err != nil {
		return fmt.Errorf("failed to connect to CDP: %w", err)
	}
	defer a.cdp.Close()

	targets, err := a.cdp.ListTargets(ctx)
	if err != nil {
		return fmt.Errorf("failed to list targets: %w", err)
	}

	printTargets(targets)
	return nil
}

func (a *App) runIntercept(ctx context.Context) error {
	cfg, err := LoadConfig(a.opts.ConfigFile, a.opts.ConfigData)
	if err != nil {
		return err
	}
	a.config = cfg

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
	}

	a.cdp = NewCDP(host, a.opts.Port, a.opts.Timeout, a.opts.Verbose)
	if err := a.cdp.Connect(ctx); err != nil {
		return fmt.Errorf("failed to connect to CDP: %w", err)
	}
	defer a.cdp.Close()

	a.target = NewTarget(a.cdp)
	selected, err := a.target.Select(ctx, a.opts.Target, a.opts.URL)
	if err != nil {
		return fmt.Errorf("failed to select target: %w", err)
	}

	if a.opts.Target != "" && a.opts.URL != "" && !a.opts.Launch {
		if err := a.cdp.NavigateTo(ctx, selected.ID, a.opts.URL); err != nil {
			return fmt.Errorf("failed to navigate to URL: %w", err)
		}
	}

	a.intercept = NewIntercept(a.cdp, a.config)
	if err := a.intercept.Start(ctx, selected.ID); err != nil {
		return fmt.Errorf("failed to start interception: %w", err)
	}

	return a.intercept.Wait(ctx)
}

func printTargets(targets []CDPTarget) {}
