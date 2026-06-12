package cmd

import (
	"testing"
)

func resetFlags() {
	cdpHost = "127.0.0.1"
	cdpPort = 9222
	timeout = 30
	verbose = false
	launch = false
	launchBrowser = "chrome"
	targetURL = ""
	target = ""
	configFile = ""
	listTargets = false
	launchArgs = nil
}

func TestExecuteHelp(t *testing.T) {
	resetFlags()
	rootCmd.SetArgs([]string{"--help"})
	if err := rootCmd.Execute(); err != nil {
		t.Errorf("Execute: %v", err)
	}
}

func TestExecuteVersion(t *testing.T) {
	resetFlags()
	rootCmd.SetArgs([]string{"version"})
	if err := rootCmd.Execute(); err != nil {
		t.Errorf("Execute version: %v", err)
	}
}

func TestExecuteListTargets(t *testing.T) {
	resetFlags()
	rootCmd.SetArgs([]string{"--list-targets", "--timeout", "1", "--port", "19999"})
	if err := rootCmd.Execute(); err != nil {
		t.Logf("Expected error (no browser): %v", err)
	}
}

func TestFlagsDefault(t *testing.T) {
	resetFlags()
	rootCmd.SetArgs([]string{"--help"})
	rootCmd.Execute()

	if cdpHost != "127.0.0.1" {
		t.Errorf("default host = %q", cdpHost)
	}
	if cdpPort != 9222 {
		t.Errorf("default port = %d", cdpPort)
	}
	if timeout != 30 {
		t.Errorf("default timeout = %d", timeout)
	}
}

func TestFlagsParsing(t *testing.T) {
	resetFlags()
	rootCmd.SetArgs([]string{"--host", "192.168.1.1", "--port", "9333", "--timeout", "10", "--verbose", "--launch", "--launch-browser", "edge", "--url", "https://example.com", "--target", "abc123", "--config", "rules.json", "--help"})
	rootCmd.Execute()

	if cdpHost != "192.168.1.1" {
		t.Errorf("host = %q", cdpHost)
	}
	if cdpPort != 9333 {
		t.Errorf("port = %d", cdpPort)
	}
	if timeout != 10 {
		t.Errorf("timeout = %d", timeout)
	}
	if !verbose {
		t.Error("verbose should be true")
	}
	if !launch {
		t.Error("launch should be true")
	}
	if launchBrowser != "edge" {
		t.Errorf("launchBrowser = %q", launchBrowser)
	}
	if targetURL != "https://example.com" {
		t.Errorf("url = %q", targetURL)
	}
	if target != "abc123" {
		t.Errorf("target = %q", target)
	}
	if configFile != "rules.json" {
		t.Errorf("config = %q", configFile)
	}
}
