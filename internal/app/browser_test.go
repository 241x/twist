package app

import (
	"context"
	"net"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestCheckPortFree(t *testing.T) {
	b := NewBrowser(true, 19999)
	if err := b.checkPort(); err != nil {
		t.Errorf("port should be free: %v", err)
	}
}

func TestCheckPortOccupied(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()

	port := ln.Addr().(*net.TCPAddr).Port

	b := NewBrowser(true, port)
	if err := b.checkPort(); err == nil {
		t.Error("expected error for occupied port")
	}
}

func TestBrowserNotLaunched(t *testing.T) {
	b := NewBrowser(false, 9222)
	if b.IsLaunched() {
		t.Error("should not be launched")
	}

	if err := b.Start(context.Background(), "chrome", nil, ""); err != nil {
		t.Errorf("Start should be no-op when not launching: %v", err)
	}

	if err := b.Stop(); err != nil {
		t.Errorf("Stop should be no-op: %v", err)
	}
}

func TestFindExecutableChrome(t *testing.T) {
	paths := getPathsForOS("chrome")
	if len(paths) == 0 {
		t.Fatal("no paths for chrome")
	}

	if runtime.GOOS == "windows" {
		hasChromeExe := false
		for _, p := range paths {
			if filepath.Base(p) == "chrome.exe" {
				hasChromeExe = true
			}
		}
		if !hasChromeExe {
			t.Error("chrome.exe not in paths")
		}
	}
}

func TestFindExecutableEdge(t *testing.T) {
	paths := getPathsForOS("edge")
	if len(paths) == 0 {
		t.Fatal("no paths for edge")
	}

	if runtime.GOOS == "windows" {
		hasMsedgeExe := false
		for _, p := range paths {
			if filepath.Base(p) == "msedge.exe" {
				hasMsedgeExe = true
			}
		}
		if !hasMsedgeExe {
			t.Error("msedge.exe not in paths")
		}
	}
}

func TestFindExecutableUnknown(t *testing.T) {
	_, err := findExecutable("firefox")
	if err == nil {
		t.Error("expected error for unknown browser type")
	}
	if !strings.Contains(err.Error(), "not found") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestBuildCommand(t *testing.T) {
	cmd := buildCommand(context.Background(), "chrome.exe", 9222, nil, "")

	if cmd.Path != "chrome.exe" {
		t.Errorf("Path = %q, want chrome.exe", cmd.Path)
	}

	foundPort := false
	foundNoFirstRun := false
	foundNoDefaultBrowserCheck := false
	for _, a := range cmd.Args {
		if strings.Contains(a, "--remote-debugging-port=9222") {
			foundPort = true
		}
		if a == "--no-first-run" {
			foundNoFirstRun = true
		}
		if a == "--no-default-browser-check" {
			foundNoDefaultBrowserCheck = true
		}
	}

	if !foundPort {
		t.Error("missing --remote-debugging-port")
	}
	if !foundNoFirstRun {
		t.Error("missing --no-first-run")
	}
	if !foundNoDefaultBrowserCheck {
		t.Error("missing --no-default-browser-check")
	}
}

func TestBuildCommandWithArgs(t *testing.T) {
	extra := []string{"--headless", "--disable-gpu"}
	cmd := buildCommand(context.Background(), "chrome", 9222, extra, "https://example.com")

	foundHeadless := false
	foundURL := false
	for _, a := range cmd.Args {
		if a == "--headless" {
			foundHeadless = true
		}
		if a == "https://example.com" {
			foundURL = true
		}
	}

	if !foundHeadless {
		t.Error("missing --headless")
	}
	if !foundURL {
		t.Error("missing URL")
	}

	// URL should be last arg
	last := cmd.Args[len(cmd.Args)-1]
	if last != "https://example.com" {
		t.Errorf("last arg = %q, want URL", last)
	}
}

func TestBuildCommandDir(t *testing.T) {
	if runtime.GOOS == "windows" {
		cmd := buildCommand(context.Background(), `C:\Program Files\Chrome\chrome.exe`, 9222, nil, "")
		if cmd.Dir != `C:\Program Files\Chrome` {
			t.Errorf("Dir = %q", cmd.Dir)
		}
	} else {
		cmd := buildCommand(context.Background(), "/usr/bin/chrome", 9222, nil, "")
		if cmd.Dir != "/usr/bin" {
			t.Errorf("Dir = %q", cmd.Dir)
		}
	}
}

func TestExpandEnvPath(t *testing.T) {
	os.Setenv("TEST_VAR", "testvalue")
	defer os.Unsetenv("TEST_VAR")

	result := expandEnvPath("${TEST_VAR}/path")
	if result != "testvalue/path" {
		t.Errorf("expandEnvPath = %q", result)
	}
}

func getPathsForOS(browserType string) []string {
	switch runtime.GOOS {
	case "windows":
		return browserPathsWindows(browserType)
	case "darwin":
		return browserPathsDarwin(browserType)
	default:
		return browserPathsLinux(browserType)
	}
}
