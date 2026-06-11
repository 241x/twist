package app

import (
	"context"
	"fmt"
	"net"
	"os/exec"
	"path/filepath"
)

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

func (b *Browser) Start(ctx context.Context, browserType string, args []string, url string) error {
	if !b.launch {
		return nil
	}

	if err := b.checkPort(); err != nil {
		return err
	}

	execPath, err := b.findExecutable(browserType)
	if err != nil {
		return err
	}

	b.cmd = b.buildCommand(ctx, execPath, args, url)

	if err := b.cmd.Start(); err != nil {
		return fmt.Errorf("failed to start browser: %w", err)
	}

	return nil
}

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

func (b *Browser) findExecutable(browserType string) (string, error) {
	return "", nil
}

func (b *Browser) buildCommand(ctx context.Context, execPath string, args []string, url string) *exec.Cmd {
	defaultArgs := []string{
		fmt.Sprintf("--remote-debugging-port=%d", b.port),
		"--no-first-run",
		"--no-default-browser-check",
	}

	allArgs := append(defaultArgs, args...)

	if url != "" {
		allArgs = append(allArgs, url)
	}

	cmd := exec.CommandContext(ctx, execPath, allArgs...)
	cmd.Dir = filepath.Dir(execPath)

	return cmd
}
