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

func TestParseQuery(t *testing.T) {
	tests := []struct {
		url  string
		key  string
		want string
	}{
		{"https://example.com?page=1&size=10", "page", "1"},
		{"https://example.com?page=1&size=10", "size", "10"},
		{"https://example.com?name=hello%20world", "name", "hello world"},
		{"https://example.com?flag", "flag", ""},
		{"https://example.com?a=1&b=2#section", "a", "1"},
		{"https://example.com", "none", ""},
	}

	for _, tt := range tests {
		t.Run(tt.key, func(t *testing.T) {
			q := parseQuery(tt.url)
			got := q[tt.key]
			if got != tt.want {
				t.Errorf("%q = %q, want %q", tt.key, got, tt.want)
			}
		})
	}
}

func TestParseCookies(t *testing.T) {
	hdrs := map[string]string{
		"Cookie": "sessionId=abc123; theme=dark; token=xyz",
	}
	cookies := parseCookies(hdrs)

	if cookies["sessionId"] != "abc123" {
		t.Errorf("sessionId = %q", cookies["sessionId"])
	}
	if cookies["theme"] != "dark" {
		t.Errorf("theme = %q", cookies["theme"])
	}
	if cookies["token"] != "xyz" {
		t.Errorf("token = %q", cookies["token"])
	}
	if _, ok := cookies["nonexistent"]; ok {
		t.Error("should not have nonexistent cookie")
	}
}

func TestParseCookiesNoHeader(t *testing.T) {
	hdrs := map[string]string{}
	cookies := parseCookies(hdrs)
	if len(cookies) != 0 {
		t.Errorf("expected empty cookies, got %d", len(cookies))
	}
}

func TestMatchQueryConditions(t *testing.T) {
	i := &Intercept{}

	ev := &fetch.RequestPausedReply{
		Request: network.Request{
			URL:    "https://example.com/api?page=1&size=10&debug=true",
			Method: "GET",
		},
	}

	t.Run("queryExists", func(t *testing.T) {
		if !i.matchCondition(ev, Condition{Type: "queryExists", Name: "page"}) {
			t.Error("queryExists should match")
		}
		if i.matchCondition(ev, Condition{Type: "queryExists", Name: "none"}) {
			t.Error("queryExists should not match non-existent param")
		}
	})

	t.Run("queryNotExists", func(t *testing.T) {
		if i.matchCondition(ev, Condition{Type: "queryNotExists", Name: "page"}) {
			t.Error("queryNotExists should not match existing param")
		}
		if !i.matchCondition(ev, Condition{Type: "queryNotExists", Name: "none"}) {
			t.Error("queryNotExists should match non-existent param")
		}
	})

	t.Run("queryEquals", func(t *testing.T) {
		if !i.matchCondition(ev, Condition{Type: "queryEquals", Name: "size", Value: "10"}) {
			t.Error("queryEquals should match")
		}
		if i.matchCondition(ev, Condition{Type: "queryEquals", Name: "size", Value: "20"}) {
			t.Error("queryEquals should not match different value")
		}
	})

	t.Run("queryContains", func(t *testing.T) {
		if !i.matchCondition(ev, Condition{Type: "queryContains", Name: "debug", Value: "tru"}) {
			t.Error("queryContains should match")
		}
	})

	t.Run("queryRegex", func(t *testing.T) {
		if !i.matchCondition(ev, Condition{Type: "queryRegex", Name: "page", Pattern: `^\d+$`}) {
			t.Error("queryRegex should match")
		}
		if i.matchCondition(ev, Condition{Type: "queryRegex", Name: "page", Pattern: `^[a-z]+$`}) {
			t.Error("queryRegex should not match")
		}
	})
}

func TestMatchCookieConditions(t *testing.T) {
	hdrs := network.Headers(`{"Cookie": "sessionId=abc123; theme=dark"}`)
	ev := &fetch.RequestPausedReply{
		Request: network.Request{
			URL:     "https://example.com/api",
			Method:  "GET",
			Headers: hdrs,
		},
	}

	i := &Intercept{}

	t.Run("cookieExists", func(t *testing.T) {
		if !i.matchCondition(ev, Condition{Type: "cookieExists", Name: "sessionId"}) {
			t.Error("cookieExists should match")
		}
		if i.matchCondition(ev, Condition{Type: "cookieExists", Name: "nonexistent"}) {
			t.Error("cookieExists should not match")
		}
	})

	t.Run("cookieEquals", func(t *testing.T) {
		if !i.matchCondition(ev, Condition{Type: "cookieEquals", Name: "theme", Value: "dark"}) {
			t.Error("cookieEquals should match")
		}
	})

	t.Run("cookieContains", func(t *testing.T) {
		if !i.matchCondition(ev, Condition{Type: "cookieContains", Name: "sessionId", Value: "abc"}) {
			t.Error("cookieContains should match")
		}
	})
}

func TestSetQueryParamValue(t *testing.T) {
	result := setQueryParamValue("https://example.com?page=1", "size", "10")
	if !strings.Contains(result, "size=10") {
		t.Errorf("missing size param: %s", result)
	}
	if !strings.Contains(result, "page=1") {
		t.Errorf("missing existing param: %s", result)
	}
}

func TestRemoveQueryParamValue(t *testing.T) {
	result := removeQueryParamValue("https://example.com?page=1&debug=true", "debug")
	if strings.Contains(result, "debug") {
		t.Errorf("debug param not removed: %s", result)
	}
	if !strings.Contains(result, "page=1") {
		t.Errorf("page param lost: %s", result)
	}
}

func TestModifyCookieHeader(t *testing.T) {
	hdrs := map[string]string{
		"Cookie": "sessionId=abc; theme=dark",
	}

	entries := modifyCookieHeader(hdrs, "theme", "light")
	found := false
	for _, e := range entries {
		if e.Name == "Cookie" &&
			strings.Contains(e.Value, "sessionId=abc") &&
			strings.Contains(e.Value, "theme=light") {
			found = true
		}
	}
	if !found {
		t.Errorf("cookie not modified: %+v", entries)
	}
}

func TestRemoveCookieFromHeader(t *testing.T) {
	hdrs := map[string]string{
		"Cookie": "sessionId=abc; theme=dark; token=xyz",
	}

	entries := removeCookieFromHeader(hdrs, "theme")
	found := false
	for _, e := range entries {
		if e.Name == "Cookie" {
			if !strings.Contains(e.Value, "theme") && strings.Contains(e.Value, "sessionId") && strings.Contains(e.Value, "token") {
				found = true
			}
		}
	}
	if !found {
		t.Errorf("cookie not removed: %+v", entries)
	}
}

func TestParseCookiePairs(t *testing.T) {
	pairs := parseCookiePairs("a=1; b=2; c=3")
	if pairs["a"] != "1" || pairs["b"] != "2" || pairs["c"] != "3" {
		t.Errorf("pairs = %+v", pairs)
	}
}

func TestBuildCookieString(t *testing.T) {
	pairs := map[string]string{"a": "1", "b": "2"}
	result := buildCookieString(pairs)
	if !strings.Contains(result, "a=1") || !strings.Contains(result, "b=2") {
		t.Errorf("result = %q", result)
	}
}

func TestBuildHeaderEntries(t *testing.T) {
	hdrs := map[string]string{
		"Content-Type": "application/json",
		"Cookie":       "old=value",
	}

	entries := buildHeaderEntries(hdrs, "Cookie", "new=value")
	var cookieEntry *fetch.HeaderEntry
	for i := range entries {
		if entries[i].Name == "Cookie" {
			cookieEntry = &entries[i]
		}
	}
	if cookieEntry == nil || cookieEntry.Value != "new=value" {
		t.Errorf("cookie not replaced: %+v", entries)
	}
}

func TestSetFormFieldValue(t *testing.T) {
	body := []byte("name=old&age=10")
	result := setFormFieldValue(body, "name", "new")
	if !strings.Contains(string(result), "name=new") || !strings.Contains(string(result), "age=10") {
		t.Errorf("result = %s", string(result))
	}
}

func TestRemoveFormFieldValue(t *testing.T) {
	body := []byte("name=test&debug=true&age=10")
	result := removeFormFieldValue(body, "debug")
	if strings.Contains(string(result), "debug") {
		t.Errorf("debug not removed: %s", string(result))
	}
	if !strings.Contains(string(result), "name=test") || !strings.Contains(string(result), "age=10") {
		t.Errorf("other fields lost: %s", string(result))
	}
}

func TestBuildHeaderEntriesNoCookie(t *testing.T) {
	hdrs := map[string]string{
		"Content-Type": "application/json",
	}

	entries := buildHeaderEntries(hdrs, "Cookie", "newcookie=val")
	found := false
	for _, e := range entries {
		if e.Name == "Cookie" && e.Value == "newcookie=val" {
			found = true
		}
	}
	if !found {
		t.Errorf("new cookie not added: %+v", entries)
	}
}

func TestJSONPatchAdd(t *testing.T) {
	var doc any = map[string]any{"name": "old", "age": 10.0}

	err := jsonPatchAdd(&doc, "/email", "test@example.com")
	if err != nil {
		t.Fatal(err)
	}

	m := doc.(map[string]any)
	if m["email"] != "test@example.com" {
		t.Errorf("email = %v", m["email"])
	}
}

func TestJSONPatchReplace(t *testing.T) {
	var doc any = map[string]any{"name": "old", "role": "user"}

	err := jsonPatchReplace(&doc, "/name", "new")
	if err != nil {
		t.Fatal(err)
	}

	m := doc.(map[string]any)
	if m["name"] != "new" {
		t.Errorf("name = %v", m["name"])
	}
}

func TestJSONPatchRemove(t *testing.T) {
	var doc any = map[string]any{"name": "test", "tmp": "delete-me"}

	err := jsonPatchRemove(&doc, "/tmp")
	if err != nil {
		t.Fatal(err)
	}

	m := doc.(map[string]any)
	if _, ok := m["tmp"]; ok {
		t.Error("tmp should be removed")
	}
	if m["name"] != "test" {
		t.Error("name should remain")
	}
}

func TestJSONPatchMove(t *testing.T) {
	var doc any = map[string]any{"old_name": "value"}

	err := jsonPatchMove(&doc, "/old_name", "/new_name")
	if err != nil {
		t.Fatal(err)
	}

	m := doc.(map[string]any)
	if _, ok := m["old_name"]; ok {
		t.Error("old_name should be moved")
	}
	if m["new_name"] != "value" {
		t.Errorf("new_name = %v", m["new_name"])
	}
}

func TestJSONPatchCopy(t *testing.T) {
	var doc any = map[string]any{"name": "original"}

	err := jsonPatchCopy(&doc, "/name", "/display_name")
	if err != nil {
		t.Fatal(err)
	}

	m := doc.(map[string]any)
	if m["name"] != "original" {
		t.Error("name should remain")
	}
	if m["display_name"] != "original" {
		t.Errorf("display_name = %v", m["display_name"])
	}
}

func TestJSONPatchTest(t *testing.T) {
	var doc any = map[string]any{"status": "active"}

	err := jsonPatchTest(doc, "/status", "active")
	if err != nil {
		t.Errorf("test should pass: %v", err)
	}

	err = jsonPatchTest(doc, "/status", "inactive")
	if err == nil {
		t.Error("test should fail")
	}
}

func TestPatchGet(t *testing.T) {
	doc := map[string]any{
		"user": map[string]any{
			"name": "john",
			"age":  25.0,
		},
	}

	val, err := patchGet(doc, "/user/name")
	if err != nil {
		t.Fatal(err)
	}
	if val != "john" {
		t.Errorf("got %v", val)
	}

	val, err = patchGet(doc, "/user/age")
	if err != nil {
		t.Fatal(err)
	}
	if val != 25.0 {
		t.Errorf("got %v", val)
	}

	_, err = patchGet(doc, "/user/nonexistent")
	if err == nil {
		t.Error("expected error for nonexistent path")
	}
}

func TestContentLengthBypass(t *testing.T) {
	i := &Intercept{cdp: &CDP{host: "127.0.0.1", port: 9222}}

	ev := &fetch.RequestPausedReply{
		Request: network.Request{
			URL:    "https://example.com/upload",
			Method: "POST",
			Headers: network.Headers(`{"Content-Length": "10485760"}`),
		},
	}

	if !i.shouldBypass(context.Background(), ev) {
		t.Error("10MB request should be bypassed")
	}

	ev2 := &fetch.RequestPausedReply{
		Request: network.Request{
			URL:    "https://example.com/normal",
			Method: "POST",
			Headers: network.Headers(`{"Content-Length": "1024"}`),
		},
	}

	if i.shouldBypass(context.Background(), ev2) {
		t.Error("1KB request should not be bypassed")
	}
}

func TestMatchBodyContains(t *testing.T) {
	i := &Intercept{}

	postData := `{"username":"admin","password":"secret"}`
	ev := &fetch.RequestPausedReply{
		Request: network.Request{
			URL:      "https://example.com/login",
			Method:   "POST",
			PostData: &postData,
		},
	}

	cond := Condition{Type: "bodyContains", Value: "admin"}
	if !i.matchCondition(ev, cond) {
		t.Error("bodyContains should match 'admin'")
	}

	cond2 := Condition{Type: "bodyContains", Value: "nonexistent"}
	if i.matchCondition(ev, cond2) {
		t.Error("bodyContains should not match 'nonexistent'")
	}

	ev2 := &fetch.RequestPausedReply{
		Request: network.Request{
			URL:    "https://example.com/api",
			Method: "GET",
		},
	}
	if i.matchCondition(ev2, cond) {
		t.Error("bodyContains should not match on empty body")
	}
}

func TestMatchBodyRegex(t *testing.T) {
	i := &Intercept{}

	postData := `{"userId":123,"name":"john"}`
	ev := &fetch.RequestPausedReply{
		Request: network.Request{
			URL:      "https://example.com/api",
			Method:   "POST",
			PostData: &postData,
		},
	}

	cond := Condition{Type: "bodyRegex", Pattern: `"userId":\s*\d+`}
	if !i.matchCondition(ev, cond) {
		t.Error("bodyRegex should match userId pattern")
	}

	cond2 := Condition{Type: "bodyRegex", Pattern: `"role":\s*"admin"`}
	if i.matchCondition(ev, cond2) {
		t.Error("bodyRegex should not match role pattern")
	}
}

func TestMatchBodyJsonPath(t *testing.T) {
	i := &Intercept{}

	postData := `{"user":{"id":123,"role":"admin"}}`
	ev := &fetch.RequestPausedReply{
		Request: network.Request{
			URL:      "https://example.com/api",
			Method:   "POST",
			PostData: &postData,
		},
	}

	cond := Condition{Type: "bodyJsonPath", Path: "/user/role", Value: "admin"}
	if !i.matchCondition(ev, cond) {
		t.Error("bodyJsonPath should match /user/role = admin")
	}

	cond2 := Condition{Type: "bodyJsonPath", Path: "/user/id", Value: "999"}
	if i.matchCondition(ev, cond2) {
		t.Error("bodyJsonPath should not match wrong id")
	}

	cond3 := Condition{Type: "bodyJsonPath", Path: "/user/nonexistent", Value: "x"}
	if i.matchCondition(ev, cond3) {
		t.Error("bodyJsonPath should not match nonexistent path")
	}
}
