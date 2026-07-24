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
	observe = false
	observeCount = 0
	observeDuration = ""
	observeFullBody = false
	observeFilter = nil
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

func TestObserveFlagsDefault(t *testing.T) {
	resetFlags()
	rootCmd.SetArgs([]string{"--help"})
	rootCmd.Execute()

	if observe {
		t.Error("observe should be false by default")
	}
	if observeCount != 0 {
		t.Errorf("observeCount should be 0, got %d", observeCount)
	}
	if observeDuration != "" {
		t.Errorf("observeDuration should be empty, got %q", observeDuration)
	}
	if observeFullBody {
		t.Error("observeFullBody should be false by default")
	}
}

func TestObserveFlagsParsing(t *testing.T) {
	resetFlags()
	rootCmd.SetArgs([]string{"--observe", "--observe-count", "50", "--observe-duration", "30s", "--observe-full-body", "--help"})
	rootCmd.Execute()

	if !observe {
		t.Error("observe should be true")
	}
	if observeCount != 50 {
		t.Errorf("observeCount = %d", observeCount)
	}
	if observeDuration != "30s" {
		t.Errorf("observeDuration = %q", observeDuration)
	}
	if !observeFullBody {
		t.Error("observeFullBody should be true")
	}
}

func TestObserveAndConfigMutuallyExclusive(t *testing.T) {
	resetFlags()
	observe = true
	configFile = "rules.json"
	rootCmd.SetArgs([]string{})
	err := runRoot(rootCmd, nil)
	if err == nil {
		t.Error("expected error when --observe and --config used together")
	} else {
		t.Logf("got expected error: %v", err)
	}
}

func TestObserveWithInvalidDuration(t *testing.T) {
	resetFlags()
	observe = true
	observeDuration = "xyz"
	rootCmd.SetArgs([]string{})
	err := runRoot(rootCmd, nil)
	if err == nil {
		t.Error("expected error for invalid duration")
	} else {
		t.Logf("got expected error: %v", err)
	}
}

func TestObserveFilterParsing(t *testing.T) {
	resetFlags()
	rootCmd.SetArgs([]string{"--observe", "--observe-filter", "url=api,analytics", "--observe-filter", "type=xhr,fetch", "--help"})
	rootCmd.Execute()

	if len(observeFilter) != 2 {
		t.Fatalf("expected 2 filter flags, got %d: %v", len(observeFilter), observeFilter)
	}
	if observeFilter[0] != "url=api,analytics" {
		t.Errorf("filter[0] = %q", observeFilter[0])
	}
	if observeFilter[1] != "type=xhr,fetch" {
		t.Errorf("filter[1] = %q", observeFilter[1])
	}
}

func TestParseObserveFilter(t *testing.T) {
	f := parseObserveFilter([]string{"url=api,analytics", "type=xhr"})
	if len(f.URLs) != 2 {
		t.Errorf("expected 2 URLs, got %d: %v", len(f.URLs), f.URLs)
	}
	if f.URLs[0] != "api" {
		t.Errorf("URLs[0] = %q", f.URLs[0])
	}
	if f.URLs[1] != "analytics" {
		t.Errorf("URLs[1] = %q", f.URLs[1])
	}
	if len(f.Types) != 1 {
		t.Errorf("expected 1 type, got %d: %v", len(f.Types), f.Types)
	}
	if f.Types[0] != "xhr" {
		t.Errorf("Types[0] = %q", f.Types[0])
	}
}

func TestParseObserveFilterEmpty(t *testing.T) {
	f := parseObserveFilter(nil)
	if !f.IsEmpty() {
		t.Error("filter should be empty")
	}
}

func TestParseObserveFilterInvalid(t *testing.T) {
	f := parseObserveFilter([]string{"invalid", "=novalue", "unknown=foo"})
	if !f.IsEmpty() {
		t.Error("filter should be empty for invalid inputs")
	}
}
