package app

import (
	"context"
	"fmt"
	"time"

	"github.com/mafredri/cdp"
	"github.com/mafredri/cdp/devtool"
	"github.com/mafredri/cdp/protocol/fetch"
	"github.com/mafredri/cdp/protocol/network"
	"github.com/mafredri/cdp/protocol/page"
	"github.com/mafredri/cdp/rpcc"
)

// CDPTarget 浏览器标签页目标信息。
type CDPTarget struct {
	ID    string `json:"id"`
	Title string `json:"title"`
	URL   string `json:"url"`
	Type  string `json:"type"`
}

// CDP 管理浏览器和页面级 WebSocket 连接，封装 CDP 协议操作。
// 浏览器级连接用于管理标签页，页面级连接用于拦截具体页面。
type CDP struct {
	host    string
	port    int
	timeout int
	verbose bool

	devt   *devtool.DevTools
	conn   *rpcc.Conn
	client *cdp.Client

	targetConn   *rpcc.Conn
	targetClient *cdp.Client
}

func NewCDP(host string, port int, timeout int, verbose bool) *CDP {
	return &CDP{
		host:    host,
		port:    port,
		timeout: timeout,
		verbose: verbose,
	}
}

// Connect 建立浏览器级 CDP 连接，等待浏览器就绪后 WebSocket 握手。
func (c *CDP) Connect(ctx context.Context) error {
	c.devt = devtool.New(fmt.Sprintf("http://%s:%d", c.host, c.port))

	if err := c.waitForBrowser(ctx); err != nil {
		return fmt.Errorf("browser not ready: %w", err)
	}

	ver, err := c.devt.Version(ctx)
	if err != nil {
		return fmt.Errorf("failed to get browser version: %w", err)
	}

	conn, err := rpcc.DialContext(ctx, ver.WebSocketDebuggerURL,
		rpcc.WithWriteBufferSize(32*1024*1024),
		rpcc.WithCompression(),
	)
	if err != nil {
		return fmt.Errorf("failed to dial browser: %w", err)
	}
	c.conn = conn
	c.client = cdp.NewClient(conn)

	return nil
}

func (c *CDP) Close() error {
	if c.targetConn != nil {
		c.targetConn.Close()
	}
	if c.conn != nil {
		c.conn.Close()
	}
	return nil
}

func (c *CDP) CloseBrowser() error {
	if c.conn != nil {
		if err := c.conn.Close(); err != nil {
			return err
		}
		c.conn = nil
		c.client = nil
	}
	return nil
}

// ListTargets 通过 HTTP /json 端点获取所有可调试目标。
func (c *CDP) ListTargets(ctx context.Context) ([]CDPTarget, error) {
	targets, err := c.devt.List(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to list targets: %w", err)
	}

	result := make([]CDPTarget, len(targets))
	for i, t := range targets {
		result[i] = CDPTarget{
			ID:    t.ID,
			Title: t.Title,
			URL:   t.URL,
			Type:  string(t.Type),
		}
	}
	return result, nil
}

func (c *CDP) NewTab(ctx context.Context, targetURL string) (*CDPTarget, error) {
	t, err := c.devt.CreateURL(ctx, targetURL)
	if err != nil {
		return nil, fmt.Errorf("failed to create tab: %w", err)
	}

	return &CDPTarget{
		ID:    t.ID,
		Title: t.Title,
		URL:   t.URL,
		Type:  string(t.Type),
	}, nil
}

func (c *CDP) NavigateTo(ctx context.Context, targetURL string) error {
	if c.targetClient == nil {
		return fmt.Errorf("target not attached")
	}

	_, err := c.targetClient.Page.Navigate(ctx, page.NewNavigateArgs(targetURL))
	if err != nil {
		return fmt.Errorf("failed to navigate: %w", err)
	}

	return nil
}

func (c *CDP) EnableNetwork(ctx context.Context) error {
	if c.targetClient == nil {
		return fmt.Errorf("target not attached")
	}

	if err := c.targetClient.Network.Enable(ctx, network.NewEnableArgs()); err != nil {
		return fmt.Errorf("failed to enable network: %w", err)
	}

	return nil
}

func (c *CDP) EnableFetch(ctx context.Context) (fetch.RequestPausedClient, error) {
	if c.targetClient == nil {
		return nil, fmt.Errorf("target not attached")
	}

	paused, err := c.targetClient.Fetch.RequestPaused(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to subscribe to RequestPaused: %w", err)
	}

	patterns := []fetch.RequestPattern{
		{URLPattern: strPtr("http://*/*"), RequestStage: fetch.RequestStageRequest},
		{URLPattern: strPtr("https://*/*"), RequestStage: fetch.RequestStageRequest},
		{URLPattern: strPtr("http://*/*"), RequestStage: fetch.RequestStageResponse},
		{URLPattern: strPtr("https://*/*"), RequestStage: fetch.RequestStageResponse},
	}

	enableArgs := fetch.NewEnableArgs().SetPatterns(patterns)
	if err := c.targetClient.Fetch.Enable(ctx, enableArgs); err != nil {
		paused.Close()
		return nil, fmt.Errorf("failed to enable fetch: %w", err)
	}

	return paused, nil
}

// AttachToTarget 连接到指定标签页的 WebSocket，建立页面级连接。
func (c *CDP) AttachToTarget(ctx context.Context, targetID string) error {
	targets, err := c.ListTargets(ctx)
	if err != nil {
		return err
	}

	var wsURL string
	for _, t := range targets {
		if t.ID == targetID {
			devtTargets, err := c.devt.List(ctx)
			if err != nil {
				return fmt.Errorf("failed to get target WS URL: %w", err)
			}
			for _, dt := range devtTargets {
				if dt.ID == targetID {
					wsURL = dt.WebSocketDebuggerURL
					break
				}
			}
			break
		}
	}

	if wsURL == "" {
		return fmt.Errorf("target %q not found or has no WebSocket URL", targetID)
	}

	conn, err := rpcc.DialContext(ctx, wsURL,
		rpcc.WithWriteBufferSize(32*1024*1024),
		rpcc.WithCompression(),
	)
	if err != nil {
		return fmt.Errorf("failed to dial target: %w", err)
	}

	c.targetConn = conn
	c.targetClient = cdp.NewClient(conn)

	return nil
}

func (c *CDP) DisableFetch(ctx context.Context) error {
	if c.targetClient == nil {
		return nil
	}
	return c.targetClient.Fetch.Disable(ctx)
}

func (c *CDP) TargetClient() *cdp.Client {
	return c.targetClient
}

func (c *CDP) waitForBrowser(ctx context.Context) error {
	deadline := time.Now().Add(time.Duration(c.timeout) * time.Second)
	for {
		if time.Now().After(deadline) {
			return fmt.Errorf("timed out after %ds: unable to connect to browser at %s:%d. Ensure the browser is running with remote debugging enabled (--remote-debugging-port=%d), or use --launch to start one automatically", c.timeout, c.host, c.port, c.port)
		}

		ver, err := c.devt.Version(ctx)
		if err == nil && ver != nil && ver.WebSocketDebuggerURL != "" {
			return nil
		}

		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(500 * time.Millisecond):
		}
	}
}

func strPtr(s string) *string {
	return &s
}
