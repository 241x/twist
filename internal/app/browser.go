package app

import (
	"context"
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
)

// Browser 管理浏览器进程的启动、端口检测和停止。
type Browser struct {
	cmd    *exec.Cmd
	launch bool
	port   int
}

func NewBrowser(launch bool, port int) *Browser {
	return &Browser{
		launch: launch,
		port:   port,
	}
}

// Start 启动浏览器进程，非 launch 模式直接返回。
// 启动前检测端口占用，自动在常见路径查找可执行文件。
func (b *Browser) Start(ctx context.Context, browserType string, args []string, url string) error {
	if !b.launch {
		return nil
	}

	if err := b.checkPort(); err != nil {
		return err
	}

	execPath, err := findExecutable(browserType)
	if err != nil {
		return err
	}

	b.cmd = buildCommand(ctx, execPath, b.port, args, url)

	if err := b.cmd.Start(); err != nil {
		return fmt.Errorf("failed to start browser: %w", err)
	}

	return nil
}

// Stop 终止浏览器进程。
func (b *Browser) Stop() error {
	if b.cmd == nil || b.cmd.Process == nil {
		return nil
	}

	if err := b.cmd.Process.Kill(); err != nil {
		return fmt.Errorf("failed to stop browser: %w", err)
	}

	return nil
}

func (b *Browser) IsLaunched() bool {
	return b.launch
}

func (b *Browser) checkPort() error {
	addr := fmt.Sprintf("127.0.0.1:%d", b.port)
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		return fmt.Errorf("port %d is already in use: %w", b.port, err)
	}
	ln.Close()
	return nil
}

func buildCommand(ctx context.Context, execPath string, port int, args []string, url string) *exec.Cmd {
	defaultArgs := []string{
		fmt.Sprintf("--remote-debugging-port=%d", port),
		"--no-first-run",
		"--no-default-browser-check",
		"--start-maximized",
	}

	tmpDir, _ := os.MkdirTemp("", "twist-browser-*")
	if tmpDir != "" {
		defaultArgs = append(defaultArgs, "--user-data-dir="+tmpDir)
	}

	allArgs := append(defaultArgs, args...)

	if url != "" {
		allArgs = append(allArgs, url)
	}

	cmd := exec.CommandContext(ctx, execPath, allArgs...)
	cmd.Dir = filepath.Dir(execPath)

	return cmd
}

// findExecutable 按浏览器类型和操作系统查找可执行文件路径。
func findExecutable(browserType string) (string, error) {
	var paths []string

	switch runtime.GOOS {
	case "windows":
		paths = browserPathsWindows(browserType)
	case "darwin":
		paths = browserPathsDarwin(browserType)
	default:
		paths = browserPathsLinux(browserType)
	}

	for _, p := range paths {
		if runtime.GOOS == "windows" {
			if _, err := os.Stat(p); err == nil {
				return p, nil
			}
		} else {
			if p, err := exec.LookPath(p); err == nil {
				return p, nil
			}
		}
	}

	return "", fmt.Errorf("browser %q not found, searched: %v", browserType, paths)
}

func expandEnvPath(path string) string {
	return os.ExpandEnv(path)
}

func browserPathsWindows(browserType string) []string {
	var paths []string

	switch browserType {
	case "chrome":
		paths = []string{
			expandEnvPath(`${ProgramFiles}\Google\Chrome\Application\chrome.exe`),
			expandEnvPath(`${LocalAppData}\Google\Chrome\Application\chrome.exe`),
		}
	case "chromium":
		paths = []string{
			expandEnvPath(`${LocalAppData}\Chromium\Application\chrome.exe`),
		}
	case "edge":
		paths = []string{
			expandEnvPath(`${ProgramFiles(x86)}\Microsoft\Edge\Application\msedge.exe`),
			expandEnvPath(`${ProgramFiles}\Microsoft\Edge\Application\msedge.exe`),
		}
	}

	return paths
}

func browserPathsDarwin(browserType string) []string {
	switch browserType {
	case "chrome":
		return []string{"/Applications/Google Chrome.app/Contents/MacOS/Google Chrome"}
	case "chromium":
		return []string{"/Applications/Chromium.app/Contents/MacOS/Chromium"}
	case "edge":
		return []string{"/Applications/Microsoft Edge.app/Contents/MacOS/Microsoft Edge"}
	}
	return nil
}

func browserPathsLinux(browserType string) []string {
	switch browserType {
	case "chrome":
		return []string{"google-chrome", "google-chrome-stable"}
	case "chromium":
		return []string{"chromium", "chromium-browser"}
	case "edge":
		return []string{"microsoft-edge", "microsoft-edge-stable"}
	}
	return nil
}
