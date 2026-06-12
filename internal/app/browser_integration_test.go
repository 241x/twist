package app

import (
	"context"
	"encoding/base64"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/mafredri/cdp/protocol/fetch"
	"github.com/mafredri/cdp/protocol/page"
)

func headlessArgs() []string {
	return []string{"--headless=new"}
}

func TestRealBrowserLaunch(t *testing.T) {
	port := 19222
	browser := NewBrowser(true, port)
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	if err := browser.Start(ctx, "chrome", headlessArgs(), "about:blank"); err != nil {
		t.Skipf("Chrome not available: %v", err)
	}
	defer browser.Stop()
	time.Sleep(2 * time.Second)

	cdp := NewCDP("127.0.0.1", port, 10, true)
	if err := cdp.Connect(ctx); err != nil {
		t.Fatalf("CDP connect failed: %v", err)
	}
	defer cdp.Close()

	targets, _ := cdp.ListTargets(ctx)
	target := NewTarget(cdp)
	selected, _ := target.Select(ctx, "", "")
	cdp.AttachToTarget(ctx, selected.ID)
	cdp.NavigateTo(ctx, "about:blank")

	t.Logf("Browser OK: %d targets", len(targets))
}

func TestRealBrowserNewTab(t *testing.T) {
	port := 19223
	browser := NewBrowser(true, port)
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	if err := browser.Start(ctx, "chrome", headlessArgs(), "about:blank"); err != nil {
		t.Skipf("Chrome not available: %v", err)
	}
	defer browser.Stop()
	time.Sleep(2 * time.Second)

	cdp := NewCDP("127.0.0.1", port, 10, false)
	cdp.Connect(ctx)
	defer cdp.Close()

	tab, _ := cdp.NewTab(ctx, "https://example.com")
	t.Logf("New tab: id=%s url=%s", tab.ID, tab.URL)
}

func TestRealBrowserInterceptBlock(t *testing.T) {
	port := 19224
	var blocked atomic.Bool

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(200)
		w.Write([]byte("OK"))
	}))
	defer ts.Close()

	browser := NewBrowser(true, port)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	if err := browser.Start(ctx, "chrome", headlessArgs(), "about:blank"); err != nil {
		t.Skipf("Chrome not available: %v", err)
	}
	defer browser.Stop()
	time.Sleep(2 * time.Second)

	cdp := NewCDP("127.0.0.1", port, 10, true)
	cdp.Connect(ctx)
	defer cdp.Close()

	tab, _ := cdp.NewTab(ctx, "about:blank")
	cdp.AttachToTarget(ctx, tab.ID)
	cdp.EnableNetwork(ctx)
	paused, _ := cdp.EnableFetch(ctx)
	defer paused.Close()

	go func() {
		for {
			ev, _ := paused.Recv()
			if strings.Contains(ev.Request.URL, "analytics") {
				blocked.Store(true)
				cdp.TargetClient().Fetch.FulfillRequest(ctx,
					fetch.NewFulfillRequestArgs(ev.RequestID, 204))
				return
			}
			cdp.TargetClient().Fetch.ContinueRequest(ctx,
				fetch.NewContinueRequestArgs(ev.RequestID))
		}
	}()

	time.Sleep(500 * time.Millisecond)
	cdp.TargetClient().Page.Navigate(ctx,
		page.NewNavigateArgs(ts.URL+"/api/analytics"))
	time.Sleep(3 * time.Second)

	if !blocked.Load() {
		t.Error("expected analytics request to be blocked")
	}
	t.Logf("block OK")
}

func TestRealBrowserSetHeader(t *testing.T) {
	port := 19227
	var receivedHeader string

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedHeader = r.Header.Get("X-Twist-Test")
		w.WriteHeader(200)
		w.Write([]byte("OK"))
	}))
	defer ts.Close()

	browser := NewBrowser(true, port)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	if err := browser.Start(ctx, "chrome", headlessArgs(), "about:blank"); err != nil {
		t.Skipf("Chrome not available: %v", err)
	}
	defer browser.Stop()
	time.Sleep(2 * time.Second)

	cdp := NewCDP("127.0.0.1", port, 10, true)
	cdp.Connect(ctx)
	defer cdp.Close()

	tab, _ := cdp.NewTab(ctx, "about:blank")
	cdp.AttachToTarget(ctx, tab.ID)
	cdp.EnableNetwork(ctx)
	paused, _ := cdp.EnableFetch(ctx)
	defer paused.Close()

	done := make(chan struct{})
	go func() {
		for {
			ev, err := paused.Recv()
			if err != nil {
				return
			}
			if strings.Contains(ev.Request.URL, "/api/header-test") {
				args := fetch.NewContinueRequestArgs(ev.RequestID)
				args.SetHeaders([]fetch.HeaderEntry{
					{Name: "X-Twist-Test", Value: "injected-value"},
				})
				cdp.TargetClient().Fetch.ContinueRequest(ctx, args)
				close(done)
				return
			}
			cdp.TargetClient().Fetch.ContinueRequest(ctx,
				fetch.NewContinueRequestArgs(ev.RequestID))
		}
	}()

	time.Sleep(500 * time.Millisecond)
	cdp.TargetClient().Page.Navigate(ctx,
		page.NewNavigateArgs(ts.URL+"/api/header-test"))

	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("timeout waiting for request interception")
	}
	time.Sleep(500 * time.Millisecond)

	if receivedHeader != "injected-value" {
		t.Errorf("X-Twist-Test=%q, want injected-value", receivedHeader)
	}
	t.Logf("setHeader OK")
}

func TestRealBrowserSetBodyMockResponse(t *testing.T) {
	port := 19226
	mockBody := `{"mocked":true}`

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"real":true}`))
	}))
	defer ts.Close()

	browser := NewBrowser(true, port)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	if err := browser.Start(ctx, "chrome", headlessArgs(), "about:blank"); err != nil {
		t.Skipf("Chrome not available: %v", err)
	}
	defer browser.Stop()
	time.Sleep(2 * time.Second)

	cdp := NewCDP("127.0.0.1", port, 10, true)
	cdp.Connect(ctx)
	defer cdp.Close()

	tab, _ := cdp.NewTab(ctx, "about:blank")
	cdp.AttachToTarget(ctx, tab.ID)
	cdp.EnableNetwork(ctx)
	paused, _ := cdp.EnableFetch(ctx)
	defer paused.Close()

	done := make(chan struct{})
	go func() {
		for {
			ev, err := paused.Recv()
			if err != nil {
				return
			}
			if strings.Contains(ev.Request.URL, "/api/users") &&
				ev.ResponseStatusCode != nil {
				args := fetch.NewFulfillRequestArgs(ev.RequestID, 200)
				args.SetBody([]byte(mockBody))
				cdp.TargetClient().Fetch.FulfillRequest(ctx, args)
				close(done)
				return
			}
			if ev.ResponseStatusCode != nil {
				cdp.TargetClient().Fetch.ContinueResponse(ctx,
					fetch.NewContinueResponseArgs(ev.RequestID))
			} else {
				cdp.TargetClient().Fetch.ContinueRequest(ctx,
					fetch.NewContinueRequestArgs(ev.RequestID))
			}
		}
	}()

	time.Sleep(500 * time.Millisecond)
	cdp.TargetClient().Page.Navigate(ctx,
		page.NewNavigateArgs(ts.URL+"/api/users"))

	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("timeout waiting for response interception")
	}
	t.Logf("setBody mock OK")
}

func TestRealBrowserReplaceBodyText(t *testing.T) {
	port := 19228

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"role":"user","status":"active"}`))
	}))
	defer ts.Close()

	browser := NewBrowser(true, port)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	if err := browser.Start(ctx, "chrome", headlessArgs(), "about:blank"); err != nil {
		t.Skipf("Chrome not available: %v", err)
	}
	defer browser.Stop()
	time.Sleep(2 * time.Second)

	cdp := NewCDP("127.0.0.1", port, 10, true)
	cdp.Connect(ctx)
	defer cdp.Close()

	tab, _ := cdp.NewTab(ctx, "about:blank")
	cdp.AttachToTarget(ctx, tab.ID)
	cdp.EnableNetwork(ctx)
	paused, _ := cdp.EnableFetch(ctx)
	defer paused.Close()

	var finalBody string
	done := make(chan struct{})
	go func() {
		for {
			ev, err := paused.Recv()
			if err != nil {
				return
			}
			if strings.Contains(ev.Request.URL, "/api/profile") &&
				ev.ResponseStatusCode != nil {
				reply, err := cdp.TargetClient().Fetch.GetResponseBody(ctx,
					fetch.NewGetResponseBodyArgs(ev.RequestID))
				if err != nil {
					cdp.TargetClient().Fetch.ContinueResponse(ctx,
						fetch.NewContinueResponseArgs(ev.RequestID))
					continue
				}
				body := reply.Body
				if reply.Base64Encoded {
					if b, e := base64.StdEncoding.DecodeString(body); e == nil {
						body = string(b)
					}
				}
				newBody := strings.ReplaceAll(body, "user", "admin")
				finalBody = newBody
				args := fetch.NewFulfillRequestArgs(ev.RequestID, 200)
				args.SetBody([]byte(newBody))
				cdp.TargetClient().Fetch.FulfillRequest(ctx, args)
				close(done)
				return
			}
			if ev.ResponseStatusCode != nil {
				cdp.TargetClient().Fetch.ContinueResponse(ctx,
					fetch.NewContinueResponseArgs(ev.RequestID))
			} else {
				cdp.TargetClient().Fetch.ContinueRequest(ctx,
					fetch.NewContinueRequestArgs(ev.RequestID))
			}
		}
	}()

	time.Sleep(500 * time.Millisecond)
	cdp.TargetClient().Page.Navigate(ctx,
		page.NewNavigateArgs(ts.URL+"/api/profile"))

	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("timeout waiting for response interception")
	}

	if !strings.Contains(finalBody, "admin") {
		t.Errorf("expected 'admin' in body, got %q", finalBody)
	}
	t.Logf("replaceBodyText OK: %s", finalBody)
}

var _ atomic.Bool
var _ = os.DevNull
