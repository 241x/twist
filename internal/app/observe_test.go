package app

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/mafredri/cdp/protocol/fetch"
	"github.com/mafredri/cdp/protocol/network"
)

// ---------------------------------------------------------------------------
// ObserveEvent JSON
// ---------------------------------------------------------------------------

func TestObserveEventRequestJSON(t *testing.T) {
	ev := ObserveEvent{
		Type:         "request",
		RequestID:    "req-1",
		URL:          "https://example.com/api",
		Method:       "POST",
		ResourceType: "XHR",
		RequestHeaders: map[string]string{
			"Content-Type": "application/json",
		},
		PostData: `{"key":"value"}`,
	}

	data, err := json.Marshal(ev)
	if err != nil {
		t.Fatal(err)
	}

	var parsed map[string]any
	if err := json.Unmarshal(data, &parsed); err != nil {
		t.Fatal(err)
	}

	if parsed["type"] != "request" {
		t.Errorf("expected type=request, got %v", parsed["type"])
	}
	if parsed["requestId"] != "req-1" {
		t.Errorf("expected requestId=req-1, got %v", parsed["requestId"])
	}
	if parsed["url"] != "https://example.com/api" {
		t.Errorf("unexpected url: %v", parsed["url"])
	}
	if parsed["method"] != "POST" {
		t.Errorf("unexpected method: %v", parsed["method"])
	}
	if parsed["postData"] != `{"key":"value"}` {
		t.Errorf("unexpected postData: %v", parsed["postData"])
	}

	if reqHeaders, ok := parsed["requestHeaders"].(map[string]any); ok {
		if reqHeaders["Content-Type"] != "application/json" {
			t.Errorf("unexpected request header: %v", reqHeaders["Content-Type"])
		}
	}
}

func TestObserveEventResponseJSON(t *testing.T) {
	ev := ObserveEvent{
		Type:        "response",
		RequestID:   "req-2",
		URL:         "https://example.com/data",
		StatusCode:  200,
		StatusText:  "OK",
		Body:        `{"ok":true}`,
		BodySize:    12,
	}

	data, err := json.Marshal(ev)
	if err != nil {
		t.Fatal(err)
	}

	var parsed map[string]any
	if err := json.Unmarshal(data, &parsed); err != nil {
		t.Fatal(err)
	}

	if parsed["type"] != "response" {
		t.Errorf("expected type=response, got %v", parsed["type"])
	}
	if parsed["statusCode"].(float64) != 200 {
		t.Errorf("expected statusCode=200, got %v", parsed["statusCode"])
	}
	if parsed["statusText"] != "OK" {
		t.Errorf("expected statusText=OK, got %v", parsed["statusText"])
	}
	if parsed["body"] != `{"ok":true}` {
		t.Errorf("unexpected body: %v", parsed["body"])
	}
}

func TestObserveEventOmitEmpty(t *testing.T) {
	ev := ObserveEvent{
		Type:      "request",
		RequestID: "req-1",
		URL:       "https://x.com",
	}

	data, _ := json.Marshal(ev)

	if strings.Contains(string(data), `"method"`) {
		t.Error("empty method should be omitted")
	}
	if strings.Contains(string(data), `"postData"`) {
		t.Error("empty postData should be omitted")
	}
	if strings.Contains(string(data), `"body"`) {
		t.Error("empty body should be omitted")
	}
}

func TestObserveEventBodyTruncated(t *testing.T) {
	ev := ObserveEvent{
		Type:          "response",
		RequestID:     "req-1",
		Body:          "ab",
		BodyTruncated: true,
		BodySize:      100,
	}

	data, _ := json.Marshal(ev)
	var parsed map[string]any
	json.Unmarshal(data, &parsed)

	if parsed["bodyTruncated"] != true {
		t.Error("bodyTruncated should be true")
	}
	if parsed["bodySize"].(float64) != 100 {
		t.Errorf("expected bodySize=100, got %v", parsed["bodySize"])
	}
}

func TestObserveEventFailedRequest(t *testing.T) {
	ev := ObserveEvent{
		Type:        "response",
		RequestID:   "req-3",
		URL:         "https://x.com/api",
		ErrorReason: "NameNotResolved",
	}

	data, err := json.Marshal(ev)
	if err != nil {
		t.Fatal(err)
	}

	var parsed map[string]any
	if err := json.Unmarshal(data, &parsed); err != nil {
		t.Fatal(err)
	}

	if parsed["type"] != "response" {
		t.Errorf("expected type=response, got %v", parsed["type"])
	}
	if parsed["errorReason"] != "NameNotResolved" {
		t.Errorf("expected errorReason=NameNotResolved, got %v", parsed["errorReason"])
	}

	// errorReason should exist, statusCode/body should not
	if _, ok := parsed["statusCode"]; ok {
		t.Error("statusCode should be omitted for failed requests")
	}
	if _, ok := parsed["body"]; ok {
		t.Error("body should be omitted for failed requests")
	}
}

// ---------------------------------------------------------------------------
// shouldBypass
// ---------------------------------------------------------------------------

func TestObserveShouldBypass(t *testing.T) {
	o := NewObserve(nil, ObserveOptions{})

	tests := []struct {
		name     string
		url      string
		resType  string
		expected bool
	}{
		{"http", "https://example.com/api", "XHR", false},
		{"data URL", "data:text/html,<p>hi</p>", "Document", true},
		{"blob URL", "blob:https://x.com/uuid", "XHR", true},
		{"WebSocket", "https://example.com/ws", "WebSocket", true},
		{"file URL", "file:///tmp/test.html", "Document", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ev := &fetch.RequestPausedReply{
				Request: network.Request{
					URL: tt.url,
				},
				ResourceType: network.ResourceType(tt.resType),
			}
			result := o.shouldBypass(ev)
			if result != tt.expected {
				t.Errorf("shouldBypass(%q, %q) = %v, want %v", tt.url, tt.resType, result, tt.expected)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// parseHeadersFromEntries
// ---------------------------------------------------------------------------

func TestParseHeadersFromEntries(t *testing.T) {
	entries := []fetch.HeaderEntry{
		{Name: "Content-Type", Value: "application/json"},
		{Name: "X-Custom", Value: "hello"},
	}

	result := parseHeadersFromEntries(entries)
	if len(result) != 2 {
		t.Errorf("expected 2 headers, got %d", len(result))
	}
	if result["Content-Type"] != "application/json" {
		t.Errorf("unexpected Content-Type: %s", result["Content-Type"])
	}
	if result["X-Custom"] != "hello" {
		t.Errorf("unexpected X-Custom: %s", result["X-Custom"])
	}
}

func TestParseHeadersFromEntriesEmpty(t *testing.T) {
	result := parseHeadersFromEntries(nil)
	if len(result) != 0 {
		t.Errorf("expected empty map, got %d entries", len(result))
	}
}

// ---------------------------------------------------------------------------
// ObserveOptions
// ---------------------------------------------------------------------------

func TestObserveOptionsDefaults(t *testing.T) {
	opts := ObserveOptions{}
	if opts.Enabled {
		t.Error("Enabled should be false by default")
	}
	if opts.Count != 0 {
		t.Error("Count should be 0 by default")
	}
	if opts.Duration != 0 {
		t.Error("Duration should be 0 by default")
	}
	if opts.FullBody {
		t.Error("FullBody should be false by default")
	}
}

func TestObserveOptionsFull(t *testing.T) {
	opts := ObserveOptions{
		Enabled:  true,
		Count:    42,
		FullBody: true,
	}
	if !opts.Enabled {
		t.Error("Enabled should be true")
	}
	if opts.Count != 42 {
		t.Errorf("Count should be 42, got %d", opts.Count)
	}
	if !opts.FullBody {
		t.Error("FullBody should be true")
	}
}

// ---------------------------------------------------------------------------
// Filter
// ---------------------------------------------------------------------------

func TestObserveFilterEmpty(t *testing.T) {
	f := ObserveFilter{}
	if !f.IsEmpty() {
		t.Error("empty filter should be IsEmpty")
	}
	if !f.Match("https://anything.com/api", "XHR") {
		t.Error("empty filter should match everything")
	}
}

func TestObserveFilterURLMatch(t *testing.T) {
	f := ObserveFilter{URLs: []string{"api", "analytics"}}

	if !f.Match("https://example.com/api/users", "XHR") {
		t.Error("should match URL containing 'api'")
	}
	if !f.Match("https://x.com/analytics/event", "XHR") {
		t.Error("should match URL containing 'analytics'")
	}
	if f.Match("https://x.com/other", "XHR") {
		t.Error("should not match URL without filter terms")
	}
}

func TestObserveFilterTypeMatch(t *testing.T) {
	f := ObserveFilter{Types: []string{"xhr", "fetch"}}

	if !f.Match("https://x.com/api", "XHR") {
		t.Error("should match XHR (case-insensitive)")
	}
	if !f.Match("https://x.com/api", "Fetch") {
		t.Error("should match Fetch")
	}
	if !f.Match("https://x.com/api", "xhr") {
		t.Error("should match lowercase xhr")
	}
	if f.Match("https://x.com/img.png", "Image") {
		t.Error("should not match Image")
	}
}

func TestObserveFilterCombined(t *testing.T) {
	f := ObserveFilter{URLs: []string{"api"}, Types: []string{"xhr"}}

	// both match
	if !f.Match("https://x.com/api/users", "XHR") {
		t.Error("should match when both URL and type match")
	}
	// URL matches but type doesn't
	if f.Match("https://x.com/api/img.png", "Image") {
		t.Error("should not match when type doesn't match")
	}
	// type matches but URL doesn't
	if f.Match("https://x.com/other", "XHR") {
		t.Error("should not match when URL doesn't match")
	}
}

func TestObserveFilterURLOnly(t *testing.T) {
	f := ObserveFilter{URLs: []string{"api"}}

	if !f.Match("https://x.com/api", "Image") {
		t.Error("should match any type when only URL filter is set")
	}
}

func TestObserveFilterTypeOnly(t *testing.T) {
	f := ObserveFilter{Types: []string{"xhr"}}

	if !f.Match("https://anything.com", "XHR") {
		t.Error("should match any URL when only type filter is set")
	}
}

// ---------------------------------------------------------------------------
// Context cancellation
// ---------------------------------------------------------------------------

func TestObserveContextCancellation(t *testing.T) {
	t.Skip("requires CDP connection")
}
