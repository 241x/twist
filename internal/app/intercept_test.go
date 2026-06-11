package app

import (
	"context"
	"strings"
	"testing"

	"github.com/mafredri/cdp/protocol/fetch"
	"github.com/mafredri/cdp/protocol/network"
)

func TestShouldBypass(t *testing.T) {
	cdp := &CDP{host: "127.0.0.1", port: 9222}
	i := &Intercept{cdp: cdp}

	tests := []struct {
		name   string
		url    string
		method string
		rt     network.ResourceType
		bypass bool
	}{
		{"non-http data:", "data:text/html,hello", "GET", network.ResourceTypeDocument, true},
		{"blob URL", "blob:https://example.com/abc", "GET", network.ResourceTypeDocument, true},
		{"websocket", "https://example.com/ws", "GET", network.ResourceTypeWebSocket, true},
		{"OPTIONS preflight", "https://example.com/api", "OPTIONS", network.ResourceTypeXHR, true},
		{"CDP self", "http://127.0.0.1:9222/json", "GET", network.ResourceTypeDocument, true},
		{"normal request", "https://example.com/api/data", "GET", network.ResourceTypeXHR, false},
		{"POST request", "https://example.com/api/data", "POST", network.ResourceTypeFetch, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ev := &fetch.RequestPausedReply{
				Request: network.Request{
					URL:    tt.url,
					Method: tt.method,
				},
				ResourceType: tt.rt,
			}

			if got := i.shouldBypass(context.Background(), ev); got != tt.bypass {
				t.Errorf("shouldBypass = %v, want %v", got, tt.bypass)
			}
		})
	}
}

func TestParseHeaders(t *testing.T) {
	raw := network.Headers(`{"Content-Type": "application/json", "Authorization": "Bearer token123"}`)
	hdrs := parseHeaders(raw)

	if hdrs["Content-Type"] != "application/json" {
		t.Errorf("Content-Type = %q", hdrs["Content-Type"])
	}
	if hdrs["Authorization"] != "Bearer token123" {
		t.Errorf("Authorization = %q", hdrs["Authorization"])
	}
}

func TestParseHeadersEmpty(t *testing.T) {
	hdrs := parseHeaders(nil)
	if len(hdrs) != 0 {
		t.Errorf("expected empty map, got %d entries", len(hdrs))
	}

	hdrs = parseHeaders([]byte{})
	if len(hdrs) != 0 {
		t.Errorf("expected empty map, got %d entries", len(hdrs))
	}
}

func TestHeaderHasKey(t *testing.T) {
	hdrs := map[string]string{
		"Content-Type":   "application/json",
		"Authorization":  "Bearer xyz",
		"X-Custom":       "value",
	}

	if !headerHasKey(hdrs, "content-type") {
		t.Error("case insensitive match failed")
	}
	if !headerHasKey(hdrs, "Authorization") {
		t.Error("exact match failed")
	}
	if headerHasKey(hdrs, "X-Not-Exist") {
		t.Error("should not find non-existent key")
	}
}

func TestHeaderGet(t *testing.T) {
	hdrs := map[string]string{"Content-Type": "text/html"}

	if v := headerGet(hdrs, "content-type"); v != "text/html" {
		t.Errorf("got %q", v)
	}
	if v := headerGet(hdrs, "nonexistent"); v != "" {
		t.Errorf("got %q, want empty", v)
	}
}

func TestMatchRuleURL(t *testing.T) {
	i := &Intercept{}

	ev := &fetch.RequestPausedReply{
		Request: network.Request{
			URL:    "https://example.com/api/users/123",
			Method: "GET",
		},
	}

	rule := &Rule{
		Stage: "request",
		Match: Match{
			AllOf: []Condition{
				{Type: "urlPrefix", Value: "https://example.com/api/"},
				{Type: "urlContains", Value: "users"},
				{Type: "urlSuffix", Value: "/123"},
			},
		},
	}

	if !i.matchRule(ev, rule) {
		t.Error("rule should match")
	}

	rule2 := &Rule{
		Stage: "request",
		Match: Match{
			AllOf: []Condition{
				{Type: "urlEquals", Value: "https://example.com/other"},
			},
		},
	}

	if i.matchRule(ev, rule2) {
		t.Error("rule should not match")
	}
}

func TestMatchRuleMethod(t *testing.T) {
	i := &Intercept{}

	ev := &fetch.RequestPausedReply{
		Request: network.Request{
			URL:    "https://example.com/api",
			Method: "DELETE",
		},
	}

	rule := &Rule{
		Stage: "request",
		Match: Match{
			AllOf: []Condition{
				{Type: "method", Values: []string{"PUT", "DELETE", "PATCH"}},
			},
		},
	}

	if !i.matchRule(ev, rule) {
		t.Error("rule should match DELETE")
	}

	rule2 := &Rule{
		Stage: "request",
		Match: Match{
			AllOf: []Condition{
				{Type: "method", Values: []string{"GET", "POST"}},
			},
		},
	}

	if i.matchRule(ev, rule2) {
		t.Error("rule should not match GET/POST")
	}
}

func TestMatchRuleAnyOf(t *testing.T) {
	i := &Intercept{}

	ev := &fetch.RequestPausedReply{
		Request: network.Request{
			URL:    "https://example.com/api",
			Method: "GET",
		},
	}

	rule := &Rule{
		Stage: "request",
		Match: Match{
			AnyOf: []Condition{
				{Type: "urlContains", Value: "nonexistent"},
				{Type: "urlContains", Value: "api"},
			},
		},
	}

	if !i.matchRule(ev, rule) {
		t.Error("rule should match via anyOf[1]")
	}

	rule2 := &Rule{
		Stage: "request",
		Match: Match{
			AnyOf: []Condition{
				{Type: "urlContains", Value: "nonexistent"},
				{Type: "method", Values: []string{"POST"}},
			},
		},
	}

	if i.matchRule(ev, rule2) {
		t.Error("rule should not match")
	}
}

func TestMatchRuleHeader(t *testing.T) {
	i := &Intercept{}

	hdrs := network.Headers(`{"Content-Type": "application/json", "X-Request-ID": "abc-123"}`)

	ev := &fetch.RequestPausedReply{
		Request: network.Request{
			URL:     "https://example.com/api",
			Method:  "POST",
			Headers: hdrs,
		},
	}

	rule := &Rule{
		Stage: "request",
		Match: Match{
			AllOf: []Condition{
				{Type: "headerExists", Name: "X-Request-ID"},
				{Type: "headerEquals", Name: "Content-Type", Value: "application/json"},
				{Type: "headerContains", Name: "Content-Type", Value: "json"},
			},
		},
	}

	if !i.matchRule(ev, rule) {
		t.Error("rule should match all header conditions")
	}

	rule2 := &Rule{
		Stage: "request",
		Match: Match{
			AllOf: []Condition{
				{Type: "headerNotExists", Name: "Authorization"},
			},
		},
	}

	if !i.matchRule(ev, rule2) {
		t.Error("rule should match headerNotExists")
	}
}

func TestMatchRulesPriority(t *testing.T) {
	i := &Intercept{
		config: &Config{
			Rules: []Rule{
				{
					ID:       "low",
					Enabled:  true,
					Stage:    "request",
					Priority: 0,
					Match:    Match{AllOf: []Condition{{Type: "urlContains", Value: "api"}}},
					Actions:  []Action{{Type: "block"}},
				},
				{
					ID:       "high",
					Enabled:  true,
					Stage:    "request",
					Priority: 10,
					Match:    Match{AllOf: []Condition{{Type: "urlContains", Value: "api"}}},
					Actions:  []Action{{Type: "block"}},
				},
			},
		},
	}

	ev := &fetch.RequestPausedReply{
		Request: network.Request{
			URL:    "https://example.com/api/data",
			Method: "GET",
		},
	}

	rule := i.matchRules(ev, "request")
	if rule == nil {
		t.Fatal("expected match")
	}
	if rule.ID != "high" {
		t.Errorf("expected high priority rule, got %s", rule.ID)
	}
}

func TestMatchRulesDisabled(t *testing.T) {
	i := &Intercept{
		config: &Config{
			Rules: []Rule{
				{
					ID:      "disabled",
					Enabled: false,
					Stage:   "request",
					Match:   Match{AllOf: []Condition{{Type: "urlContains", Value: "api"}}},
					Actions: []Action{{Type: "block"}},
				},
			},
		},
	}

	ev := &fetch.RequestPausedReply{
		Request: network.Request{
			URL:    "https://example.com/api",
			Method: "GET",
		},
	}

	rule := i.matchRules(ev, "request")
	if rule != nil {
		t.Error("disabled rule should not match")
	}
}

func TestMatchRulesStageFilter(t *testing.T) {
	i := &Intercept{
		config: &Config{
			Rules: []Rule{
				{
					ID:      "response-only",
					Enabled: true,
					Stage:   "response",
					Match:   Match{AllOf: []Condition{{Type: "urlContains", Value: "api"}}},
					Actions: []Action{{Type: "block"}},
				},
			},
		},
	}

	ev := &fetch.RequestPausedReply{
		Request: network.Request{
			URL:    "https://example.com/api",
			Method: "GET",
		},
	}

	rule := i.matchRules(ev, "request")
	if rule != nil {
		t.Error("response-stage rule should not match in request stage")
	}
}

func TestSortByPriority(t *testing.T) {
	rules := []Rule{
		{ID: "a", Priority: 0},
		{ID: "b", Priority: 10},
		{ID: "c", Priority: 5},
		{ID: "d", Priority: 10},
	}

	sortByPriority(rules)

	expected := []string{"b", "d", "c", "a"}
	for i, exp := range expected {
		if rules[i].ID != exp {
			t.Errorf("index %d: got %s, want %s", i, rules[i].ID, exp)
		}
	}

	// Both priority 10 should maintain relative order (stable-like)
	if rules[0].ID != "b" || rules[1].ID != "d" {
		t.Errorf("same priority order: [%s, %s]", rules[0].ID, rules[1].ID)
	}
}

func TestMatchConditionCaseInsensitiveMethod(t *testing.T) {
	i := &Intercept{}

	ev := &fetch.RequestPausedReply{
		Request: network.Request{
			URL:    "https://example.com/api",
			Method: "get",
		},
	}

	cond := Condition{Type: "method", Values: []string{"GET"}}
	if !i.matchCondition(ev, cond) {
		t.Error("method matching should be case insensitive")
	}
}

func TestBypassContainsCDP(t *testing.T) {
	i := &Intercept{cdp: &CDP{host: "192.168.1.1", port: 9222}}

	ev := &fetch.RequestPausedReply{
		Request: network.Request{
			URL:    "http://192.168.1.1:9222/devtools/page/abc",
			Method: "GET",
		},
	}

	if !i.shouldBypass(context.Background(), ev) {
		t.Error("CDP self-traffic should be bypassed")
	}

	ev2 := &fetch.RequestPausedReply{
		Request: network.Request{
			URL:    "http://other:9222/api",
			Method: "GET",
		},
	}

	if i.shouldBypass(context.Background(), ev2) {
		t.Error("non-CDP traffic on same port should not be bypassed")
	}
}

func TestWorkerCount(t *testing.T) {
	i := NewIntercept(nil, &Config{})
	if i.workerCount < 4 {
		t.Errorf("workerCount = %d, want at least 4", i.workerCount)
	}
}

func TestRegexMatching(t *testing.T) {
	tests := []struct {
		pattern string
		input   string
		match   bool
	}{
		{`^https://example\.com/api/`, "https://example.com/api/users", true},
		{`^https://example\.com/api/`, "http://other.com/api/users", false},
		{`\d{3,}`, "user/12345", true},
		{`\d{3,}`, "user/12", false},
		{"[invalid", "anything", false},
		{"", "anything", false},
	}

	for _, tt := range tests {
		t.Run(tt.pattern, func(t *testing.T) {
			if got := matchRegex(tt.pattern, tt.input); got != tt.match {
				t.Errorf("matchRegex(%q, %q) = %v, want %v", tt.pattern, tt.input, got, tt.match)
			}
		})
	}
}

func TestMatchRuleURLRegex(t *testing.T) {
	i := &Intercept{}

	ev := &fetch.RequestPausedReply{
		Request: network.Request{
			URL:    "https://example.com/api/users/12345",
			Method: "GET",
		},
	}

	rule := &Rule{
		Stage: "request",
		Match: Match{
			AllOf: []Condition{
				{Type: "urlRegex", Pattern: `^https://example\.com/api/users/\d+$`},
			},
		},
	}

	if !i.matchRule(ev, rule) {
		t.Error("urlRegex should match")
	}

	ev2 := &fetch.RequestPausedReply{
		Request: network.Request{
			URL:    "https://example.com/api/other",
			Method: "GET",
		},
	}

	if i.matchRule(ev2, rule) {
		t.Error("urlRegex should not match")
	}
}

func TestMatchRuleHeaderRegex(t *testing.T) {
	i := &Intercept{}

	hdrs := network.Headers(`{"Authorization": "Bearer eyJhbGciOiJIUzI1NiJ9.eyJzdWIiOiIxMjM0NTY3ODkwIn0.abc123"}`)
	ev := &fetch.RequestPausedReply{
		Request: network.Request{
			URL:     "https://example.com/api",
			Method:  "GET",
			Headers: hdrs,
		},
	}

	rule := &Rule{
		Stage: "request",
		Match: Match{
			AllOf: []Condition{
				{Type: "headerRegex", Name: "Authorization", Pattern: `^Bearer\s+[A-Za-z0-9\-_]+\.[A-Za-z0-9\-_]+\..*$`},
			},
		},
	}

	if !i.matchRule(ev, rule) {
		t.Error("headerRegex should match JWT Bearer token")
	}

	rule2 := &Rule{
		Stage: "request",
		Match: Match{
			AllOf: []Condition{
				{Type: "headerRegex", Name: "X-Custom", Pattern: ".*"},
			},
		},
	}

	if i.matchRule(ev, rule2) {
		t.Error("headerRegex on missing header should not match")
	}
}

func BenchmarkMatchCondition(b *testing.B) {
	i := &Intercept{}
	ev := &fetch.RequestPausedReply{
		Request: network.Request{
			URL:    "https://example.com/api/users/12345/profile",
			Method: "GET",
			Headers: network.Headers(`{"Content-Type":"application/json","Authorization":"Bearer token"}`),
		},
		ResourceType: network.ResourceTypeXHR,
	}

	cond := Condition{Type: "urlContains", Value: "users"}

	b.ResetTimer()
	for n := 0; n < b.N; n++ {
		i.matchCondition(ev, cond)
	}
}

func helperStringContains(s, substr string) bool {
	return strings.Contains(s, substr)
}
