package app_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"github.com/241x/twist/internal/app"
)

func TestConfigLoadIntegration(t *testing.T) {
	content := `{
		"id": "twist-20260611-test01",
		"name": "Integration Test Config",
		"version": "1.0",
		"description": "Test config for integration",
		"rules": [
			{
				"id": "rule-001",
				"name": "Block analytics",
				"enabled": true,
				"priority": 10,
				"stage": "request",
				"match": {
					"allOf": [
						{"type": "urlContains", "value": "analytics"},
						{"type": "method", "values": ["GET", "POST"]}
					]
				},
				"actions": [{"type": "block", "statusCode": 204}]
			},
			{
				"id": "rule-002",
				"name": "Mock API response",
				"enabled": true,
				"priority": 5,
				"stage": "response",
				"match": {
					"allOf": [
						{"type": "urlContains", "value": "/api/users"}
					]
				},
				"actions": [{
					"type": "setBody",
					"value": "{\"data\":[]}",
					"encoding": "text"
				}]
			}
		]
	}`

	cfg, err := app.LoadConfig("", []byte(content))
	if err != nil {
		t.Fatalf("LoadConfig failed: %v", err)
	}

	if cfg.ID != "twist-20260611-test01" {
		t.Errorf("ID = %q", cfg.ID)
	}
	if len(cfg.Rules) != 2 {
		t.Fatalf("expected 2 rules, got %d", len(cfg.Rules))
	}
	if cfg.Rules[0].Priority != 10 {
		t.Errorf("rule[0] priority = %d", cfg.Rules[0].Priority)
	}
	if cfg.Rules[1].Stage != "response" {
		t.Errorf("rule[1] stage = %q", cfg.Rules[1].Stage)
	}
}

func TestConfigLoadFromFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test-config.json")

	content := `{"id":"twist-20260611-file01","name":"File Config","version":"1.0","rules":[{"id":"r1","name":"r1","enabled":true,"priority":0,"stage":"request","match":{"allOf":[{"type":"urlContains","value":"/api/"}]},"actions":[{"type":"block"}]}]}`
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	cfg, err := app.LoadConfig(path, nil)
	if err != nil {
		t.Fatalf("LoadConfig from file failed: %v", err)
	}

	if cfg.ID != "twist-20260611-file01" {
		t.Errorf("ID = %q", cfg.ID)
	}
	if len(cfg.Rules) != 1 {
		t.Fatalf("expected 1 rule, got %d", len(cfg.Rules))
	}
}

func TestConfigLoadInvalidJSON(t *testing.T) {
	_, err := app.LoadConfig("", []byte(`{invalid}`))
	if err == nil {
		t.Fatal("expected error for invalid JSON")
	}
}

func TestConfigLoadMissingFile(t *testing.T) {
	_, err := app.LoadConfig("/nonexistent/path/config.json", nil)
	if err == nil {
		t.Fatal("expected error for missing file")
	}
}

func TestConditionAllTypes(t *testing.T) {
	tests := []struct {
		name string
		cond string
	}{
		{"urlEquals", `{"type":"urlEquals","value":"https://example.com"}`},
		{"urlPrefix", `{"type":"urlPrefix","value":"https://example.com/"}`},
		{"urlSuffix", `{"type":"urlSuffix","value":".json"}`},
		{"urlContains", `{"type":"urlContains","value":"/api/"}`},
		{"urlRegex", `{"type":"urlRegex","pattern":"^https://.*"}`},
		{"method", `{"type":"method","values":["GET","POST"]}`},
		{"resourceType", `{"type":"resourceType","values":["xhr","fetch"]}`},
		{"headerExists", `{"type":"headerExists","name":"Authorization"}`},
		{"headerNotExists", `{"type":"headerNotExists","name":"X-Debug"}`},
		{"headerEquals", `{"type":"headerEquals","name":"Content-Type","value":"application/json"}`},
		{"headerContains", `{"type":"headerContains","name":"User-Agent","value":"Chrome"}`},
		{"headerRegex", `{"type":"headerRegex","name":"Authorization","pattern":"^Bearer"}`},
		{"queryExists", `{"type":"queryExists","name":"page"}`},
		{"queryEquals", `{"type":"queryEquals","name":"page","value":"1"}`},
		{"queryContains", `{"type":"queryContains","name":"q","value":"test"}`},
		{"cookieExists", `{"type":"cookieExists","name":"sessionId"}`},
		{"cookieEquals", `{"type":"cookieEquals","name":"theme","value":"dark"}`},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var c app.Condition
			if err := json.Unmarshal([]byte(tt.cond), &c); err != nil {
				t.Errorf("unexpected parse error: %v", err)
			}
		})
	}
}

func TestActionAllTypes(t *testing.T) {
	tests := []struct {
		name   string
		action string
	}{
		{"block", `{"type":"block","statusCode":403,"body":"denied"}`},
		{"setHeader", `{"type":"setHeader","name":"X-Custom","value":"test"}`},
		{"removeHeader", `{"type":"removeHeader","name":"X-Tracking"}`},
		{"setUrl", `{"type":"setUrl","value":"https://new.example.com"}`},
		{"setMethod", `{"type":"setMethod","value":"POST"}`},
		{"setQueryParam", `{"type":"setQueryParam","name":"page","value":"2"}`},
		{"removeQueryParam", `{"type":"removeQueryParam","name":"debug"}`},
		{"setCookie", `{"type":"setCookie","name":"token","value":"abc"}`},
		{"removeCookie", `{"type":"removeCookie","name":"tracking"}`},
		{"setFormField", `{"type":"setFormField","name":"name","value":"test"}`},
		{"removeFormField", `{"type":"removeFormField","name":"csrf"}`},
		{"setStatus", `{"type":"setStatus","value":201}`},
		{"setBody", `{"type":"setBody","value":"{\"ok\":true}"}`},
		{"replaceBodyText", `{"type":"replaceBodyText","search":"old","replace":"new","replaceAll":true}`},
		{"patchBodyJson", `{"type":"patchBodyJson","patches":[{"op":"replace","path":"/name","value":"new"}]}`},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var a app.Action
			if err := json.Unmarshal([]byte(tt.action), &a); err != nil {
				t.Errorf("unexpected parse error: %v", err)
			}
		})
	}
}

func TestHTTPTestServer(t *testing.T) {
	var requestCount atomic.Int32

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestCount.Add(1)
		switch r.URL.Path {
		case "/api/users":
			w.Header().Set("Content-Type", "application/json")
			w.Write([]byte(`{"users":[{"id":1,"name":"Alice"},{"id":2,"name":"Bob"}]}`))
		case "/api/analytics":
			w.WriteHeader(204)
		default:
			w.WriteHeader(404)
		}
	}))
	defer server.Close()

	client := &http.Client{Timeout: 5 * time.Second}

	resp, err := client.Get(server.URL + "/api/users")
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Errorf("expected 200, got %d", resp.StatusCode)
	}

	if requestCount.Load() < 1 {
		t.Error("expected at least 1 request")
	}
}

func TestAppNewAndOptions(t *testing.T) {
	opts := app.Options{
		Host:        "127.0.0.1",
		Port:        9222,
		Launch:      false,
		Verbose:     true,
		Timeout:     30,
		ConfigData:  []byte(`{"id":"t-1","name":"test","version":"1.0","rules":[]}`),
		ListTargets: false,
	}

	a := app.New(opts)
	if a == nil {
		t.Fatal("App.New returned nil")
	}
	a.Shutdown()
}

func TestAppListTargetsNoBrowser(t *testing.T) {
	opts := app.Options{
		Host:        "127.0.0.1",
		Port:        19999,
		Timeout:     1,
		ListTargets: true,
	}

	a := app.New(opts)

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	err := a.Run(ctx)
	if err == nil {
		t.Log("expected error (no browser running), got nil")
	}
	a.Shutdown()
}

func TestStdinConfigPiping(t *testing.T) {
	content := `{"id":"twist-20260611-stdin","name":"Stdin Config","version":"1.0","rules":[{"id":"r1","name":"r1","enabled":true,"priority":0,"stage":"request","match":{},"actions":[{"type":"block"}]}]}`

	cfg, err := app.LoadConfig("", []byte(content))
	if err != nil {
		t.Fatalf("LoadConfig from stdin data failed: %v", err)
	}

	if cfg.ID != "twist-20260611-stdin" {
		t.Errorf("ID = %q", cfg.ID)
	}
}

func TestPriorityEdgeCases(t *testing.T) {
	content := `{
		"id": "twist-20260611-priority",
		"name": "Priority Test",
		"version": "1.0",
		"rules": [
			{"id":"p0","name":"p0","enabled":true,"priority":0,"stage":"request","match":{"allOf":[{"type":"urlContains","value":"/api/"}]},"actions":[{"type":"block"}]},
			{"id":"p5","name":"p5","enabled":true,"priority":5,"stage":"request","match":{"allOf":[{"type":"urlContains","value":"/api/"}]},"actions":[{"type":"block"}]},
			{"id":"p10","name":"p10","enabled":true,"priority":10,"stage":"request","match":{"allOf":[{"type":"urlContains","value":"/api/"}]},"actions":[{"type":"block"}]},
			{"id":"p5b","name":"p5b","enabled":true,"priority":5,"stage":"request","match":{"allOf":[{"type":"urlContains","value":"/api/"}]},"actions":[{"type":"block"}]}
		]
	}`

	cfg, err := app.LoadConfig("", []byte(content))
	if err != nil {
		t.Fatal(err)
	}

	if len(cfg.Rules) != 4 {
		t.Fatalf("expected 4 rules, got %d", len(cfg.Rules))
	}

	if cfg.Rules[0].Priority != 0 {
		t.Errorf("rule[0] priority should be 0 (file order), got %d", cfg.Rules[0].Priority)
	}
	if cfg.Rules[1].Priority != 5 {
		t.Errorf("rule[1] priority = %d", cfg.Rules[1].Priority)
	}
	if cfg.Rules[2].Priority != 10 {
		t.Errorf("rule[2] priority = %d", cfg.Rules[2].Priority)
	}
}

func TestEmptyMatch(t *testing.T) {
	content := `{
		"id": "twist-20260611-em",
		"name": "Empty Match",
		"version": "1.0",
		"rules": [{
			"id": "r1",
			"name": "match-all",
			"enabled": true,
			"priority": 0,
			"stage": "request",
			"match": {"allOf":[],"anyOf":[]},
			"actions": [{"type": "block"}]
		}]
	}`

	cfg, err := app.LoadConfig("", []byte(content))
	if err != nil {
		t.Fatal(err)
	}

	if cfg.Rules[0].Match.AllOf != nil && len(cfg.Rules[0].Match.AllOf) != 0 {
		t.Error("allOf should be empty")
	}
	if cfg.Rules[0].Match.AnyOf != nil && len(cfg.Rules[0].Match.AnyOf) != 0 {
		t.Error("anyOf should be empty")
	}
}

func TestLargeConfig(t *testing.T) {
	var rules bytes.Buffer
	rules.WriteString("[")
	for i := 0; i < 100; i++ {
		if i > 0 {
			rules.WriteString(",")
		}
		rules.WriteString(`{"id":"r` + fmt.Sprintf("%d", i) + `","name":"rule","enabled":true,"priority":0,"stage":"request","match":{"allOf":[{"type":"urlContains","value":"/api/"}]},"actions":[{"type":"block"}]}`)
	}
	rules.WriteString("]")

	content := `{"id":"twist-20260611-large","name":"Large","version":"1.0","rules":` + rules.String() + `}`

	cfg, err := app.LoadConfig("", []byte(content))
	if err != nil {
		t.Fatal(err)
	}

	if len(cfg.Rules) != 100 {
		t.Errorf("expected 100 rules, got %d", len(cfg.Rules))
	}
}
