package app

import (
	"context"
	"encoding/json"
	"net/http"
	"net/url"
	"strconv"
)

type CDPTarget struct {
	ID    string `json:"id"`
	Title string `json:"title"`
	URL   string `json:"url"`
	Type  string `json:"type"`
}

type CDP struct {
	host    string
	port    int
	timeout int
	verbose bool
}

func NewCDP(host string, port int, timeout int, verbose bool) *CDP {
	return &CDP{
		host:    host,
		port:    port,
		timeout: timeout,
		verbose: verbose,
	}
}

func (c *CDP) Connect(ctx context.Context) error {
	return nil
}

func (c *CDP) Close() error {
	return nil
}

func (c *CDP) ListTargets(ctx context.Context) ([]CDPTarget, error) {
	return nil, nil
}

func (c *CDP) NewTab(ctx context.Context) (*CDPTarget, error) {
	return nil, nil
}

func (c *CDP) NavigateTo(ctx context.Context, targetID string, targetURL string) error {
	return nil
}

func (c *CDP) EnableNetwork(ctx context.Context, targetID string) error {
	return nil
}

func (c *CDP) SetRequestInterceptor(ctx context.Context, targetID string) error {
	return nil
}

func (c *CDP) SetResponseInterceptor(ctx context.Context, targetID string) error {
	return nil
}

func (c *CDP) endpointURL() string {
	return (&url.URL{
		Scheme: "http",
		Host:   buildHostPort(c.host, c.port),
		Path:   "/json",
	}).String()
}

func fetchTargets(httpEndpoint string) ([]CDPTarget, error) {
	resp, err := http.Get(httpEndpoint)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var targets []CDPTarget
	if err := json.NewDecoder(resp.Body).Decode(&targets); err != nil {
		return nil, err
	}

	return targets, nil
}

func buildHostPort(host string, port int) string {
	return host + ":" + strconv.Itoa(port)
}
