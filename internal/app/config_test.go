package app

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestParseConfigFull(t *testing.T) {
	input := `{
  "id": "twist-20260611-demo01",
  "name": "演示配置",
  "version": "1.0",
  "description": "包含常见规则的演示配置",
  "rules": [
    {
      "id": "rule-001",
      "name": "Mock 用户信息接口",
      "enabled": true,
      "priority": 10,
      "stage": "response",
      "match": {
        "allOf": [
          {"type": "urlContains", "value": "/api/user/info"},
          {"type": "method", "values": ["GET"]}
        ]
      },
      "actions": [
        {
          "type": "setBody",
          "value": "{\"code\":0}",
          "encoding": "text"
        }
      ]
    },
    {
      "id": "rule-002",
      "name": "全局 CORS 处理",
      "enabled": true,
      "priority": 5,
      "stage": "response",
      "match": {
        "allOf": [],
        "anyOf": []
      },
      "actions": [
        {"type": "setHeader", "name": "Access-Control-Allow-Origin", "value": "*"}
      ]
    },
    {
      "id": "rule-003",
      "name": "阻止追踪请求",
      "enabled": true,
      "priority": 0,
      "stage": "request",
      "match": {
        "anyOf": [
          {"type": "urlContains", "value": "google-analytics.com"}
        ]
      },
      "actions": [
        {"type": "block", "statusCode": 204}
      ]
    }
  ]
}`

	cfg, err := parseConfig([]byte(input))
	if err != nil {
		t.Fatalf("parseConfig: %v", err)
	}

	if cfg.ID != "twist-20260611-demo01" {
		t.Errorf("ID = %q, want %q", cfg.ID, "twist-20260611-demo01")
	}
	if len(cfg.Rules) != 3 {
		t.Fatalf("len(Rules) = %d, want 3", len(cfg.Rules))
	}

	if cfg.Rules[0].Priority != 10 {
		t.Errorf("rule[0].Priority = %d, want 10", cfg.Rules[0].Priority)
	}
	if len(cfg.Rules[0].Match.AllOf) != 2 {
		t.Errorf("rule[0].Match.AllOf = %d, want 2", len(cfg.Rules[0].Match.AllOf))
	}
	if len(cfg.Rules[0].Actions) != 1 {
		t.Errorf("rule[0].Actions = %d, want 1", len(cfg.Rules[0].Actions))
	}

	if cfg.Rules[1].Priority != 5 {
		t.Errorf("rule[1].Priority = %d, want 5", cfg.Rules[1].Priority)
	}

	if cfg.Rules[2].Priority != 0 {
		t.Errorf("rule[2].Priority = %d, want 0", cfg.Rules[2].Priority)
	}
}

func TestParseConfigMissingFields(t *testing.T) {
	tests := []struct {
		name  string
		input string
		err   string
	}{
		{
			name:  "missing id",
			input: `{"name":"test","version":"1.0","rules":[]}`,
			err:   "field 'id' is required",
		},
		{
			name:  "missing name",
			input: `{"id":"t-1","version":"1.0","rules":[]}`,
			err:   "field 'name' is required",
		},
		{
			name:  "missing version",
			input: `{"id":"t-1","name":"test","rules":[]}`,
			err:   "field 'version' is required",
		},
		{
			name:  "empty rules",
			input: `{"id":"t-1","name":"test","version":"1.0","rules":[]}`,
			err:   "field 'rules' must not be empty",
		},
		{
			name:  "rule missing id",
			input: `{"id":"t-1","name":"test","version":"1.0","rules":[{"name":"r1","enabled":true,"priority":0,"stage":"request","match":{},"actions":[{"type":"block"}]}]}`,
			err:   "rules[0]: field 'id' is required",
		},
		{
			name:  "rule invalid stage",
			input: `{"id":"t-1","name":"test","version":"1.0","rules":[{"id":"r1","name":"r1","enabled":true,"priority":0,"stage":"invalid","match":{},"actions":[{"type":"block"}]}]}`,
			err:   "rules[0]: field 'stage' must be 'request' or 'response'",
		},
		{
			name:  "rule empty actions",
			input: `{"id":"t-1","name":"test","version":"1.0","rules":[{"id":"r1","name":"r1","enabled":true,"priority":0,"stage":"request","match":{},"actions":[]}]}`,
			err:   "rules[0]: field 'actions' must not be empty",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := LoadConfig("", []byte(tt.input))
			if err == nil {
				t.Fatal("expected error, got nil")
			}
			if !strings.Contains(err.Error(), tt.err) {
				t.Errorf("error = %q, want containing %q", err.Error(), tt.err)
			}
		})
	}
}

func TestConditionValidation(t *testing.T) {
	tests := []struct {
		name    string
		cond    string
		wantErr string
	}{
		{"urlEquals requires value", `{"type":"urlEquals"}`, "requires field 'value'"},
		{"urlRegex requires pattern", `{"type":"urlRegex"}`, "requires field 'pattern'"},
		{"method requires values", `{"type":"method"}`, "requires field 'values'"},
		{"headerExists requires name", `{"type":"headerExists"}`, "requires field 'name'"},
		{"headerEquals requires name+value", `{"type":"headerEquals"}`, "requires field 'name'"},
		{"bodyJsonPath requires path", `{"type":"bodyJsonPath"}`, "requires field 'path'"},
		{"unknown condition type", `{"type":"unknown"}`, "unknown condition type"},
		{"valid urlContains", `{"type":"urlContains","value":"/api"}`, ""},
		{"valid method", `{"type":"method","values":["GET"]}`, ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var c Condition
			err := json.Unmarshal([]byte(tt.cond), &c)
			if tt.wantErr == "" {
				if err != nil {
					t.Errorf("unexpected error: %v", err)
				}
			} else {
				if err == nil {
					t.Fatal("expected error, got nil")
				}
				if !strings.Contains(err.Error(), tt.wantErr) {
					t.Errorf("error = %q, want containing %q", err.Error(), tt.wantErr)
				}
			}
		})
	}
}

func TestActionValidation(t *testing.T) {
	tests := []struct {
		name    string
		action  string
		wantErr string
	}{
		{"setUrl requires value", `{"type":"setUrl"}`, "requires field 'value'"},
		{"setQueryParam requires name", `{"type":"setQueryParam"}`, "requires field 'name'"},
		{"setHeader requires name+value", `{"type":"setHeader"}`, "requires field 'name'"},
		{"block defaults ok", `{"type":"block"}`, ""},
		{"setBody requires value", `{"type":"setBody"}`, "requires field 'value'"},
		{"patchBodyJson requires patches", `{"type":"patchBodyJson"}`, "requires field 'patches'"},
		{"replaceBodyText requires search+replace", `{"type":"replaceBodyText"}`, "requires field 'search'"},
		{"unknown action type", `{"type":"unknown"}`, "unknown action type"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var a Action
			err := json.Unmarshal([]byte(tt.action), &a)
			if tt.wantErr == "" {
				if err != nil {
					t.Errorf("unexpected error: %v", err)
				}
			} else {
				if err == nil {
					t.Fatal("expected error, got nil")
				}
				if !strings.Contains(err.Error(), tt.wantErr) {
					t.Errorf("error = %q, want containing %q", err.Error(), tt.wantErr)
				}
			}
		})
	}
}

func TestMatchAllOfAndAnyOf(t *testing.T) {
	input := `{
		"id": "t-1",
		"name": "test",
		"version": "1.0",
		"rules": [{
			"id": "r1",
			"name": "r1",
			"enabled": true,
			"priority": 0,
			"stage": "request",
			"match": {
				"allOf": [
					{"type": "urlPrefix", "value": "https://example.com/"},
					{"type": "method", "values": ["POST"]}
				],
				"anyOf": [
					{"type": "headerExists", "name": "Authorization"},
					{"type": "cookieExists", "name": "token"}
				]
			},
			"actions": [{"type": "block"}]
		}]
	}`

	cfg, err := parseConfig([]byte(input))
	if err != nil {
		t.Fatalf("parseConfig: %v", err)
	}

	m := cfg.Rules[0].Match
	if len(m.AllOf) != 2 {
		t.Errorf("len(AllOf) = %d, want 2", len(m.AllOf))
	}
	if len(m.AnyOf) != 2 {
		t.Errorf("len(AnyOf) = %d, want 2", len(m.AnyOf))
	}
	if m.AllOf[0].Type != "urlPrefix" || m.AllOf[0].Value != "https://example.com/" {
		t.Errorf("AllOf[0] = %+v", m.AllOf[0])
	}
	if m.AnyOf[1].Type != "cookieExists" || m.AnyOf[1].Name != "token" {
		t.Errorf("AnyOf[1] = %+v", m.AnyOf[1])
	}
}

func TestBlockActionDefaults(t *testing.T) {
	input := `{
		"id": "t-1",
		"name": "test",
		"version": "1.0",
		"rules": [{
			"id": "r1",
			"name": "r1",
			"enabled": true,
			"priority": 0,
			"stage": "request",
			"match": {},
			"actions": [{"type": "block"}]
		}]
	}`

	cfg, err := parseConfig([]byte(input))
	if err != nil {
		t.Fatalf("parseConfig: %v", err)
	}

	a := cfg.Rules[0].Actions[0]
	if a.StatusCode != 200 {
		t.Errorf("block StatusCode default = %d, want 200", a.StatusCode)
	}
	if a.BodyEncoding != "text" {
		t.Errorf("block BodyEncoding default = %q, want text", a.BodyEncoding)
	}
}

func TestValidateActionsCompatibility(t *testing.T) {
	tests := []struct {
		name    string
		actions string
		stage   string
		wantErr bool
	}{
		{
			name:    "single action ok",
			actions: `[{"type": "block"}]`,
			stage:   "request",
		},
		{
			name:    "two modify actions ok",
			actions: `[{"type": "setHeader", "name": "X-Test", "value": "1"}, {"type": "setUrl", "value": "https://example.com"}]`,
			stage:   "request",
		},
		{
			name:    "multiple header actions ok",
			actions: `[{"type": "setHeader", "name": "X-A", "value": "1"}, {"type": "removeHeader", "name": "X-B"}, {"type": "setCookie", "name": "c1", "value": "v1"}]`,
			stage:   "request",
		},
		{
			name:    "block + setHeader conflict",
			actions: `[{"type": "block"}, {"type": "setHeader", "name": "X-Test", "value": "1"}]`,
			stage:   "request",
			wantErr: true,
		},
		{
			name:    "setStatus + setHeader conflict",
			actions: `[{"type": "setStatus", "value": 404}, {"type": "setHeader", "name": "X-Test", "value": "1"}]`,
			stage:   "response",
			wantErr: true,
		},
		{
			name:    "block + setStatus conflict",
			actions: `[{"type": "block"}, {"type": "setStatus", "value": 500}]`,
			stage:   "response",
			wantErr: true,
		},
		{
			name:    "replaceElement + setHeader conflict",
			actions: `[{"type": "replaceElement", "selector": "body", "value": "hi"}, {"type": "setHeader", "name": "X-Test", "value": "1"}]`,
			stage:   "response",
			wantErr: true,
		},
		{
			name:    "formField + bodyText conflict",
			actions: `[{"type": "setFormField", "name": "user", "value": "admin"}, {"type": "setBody", "value": "{\"ok\":true}"}]`,
			stage:   "request",
			wantErr: true,
		},
		{
			name:    "bodyText only ok",
			actions: `[{"type": "setBody", "value": "hello"}, {"type": "appendBody", "value": "world"}]`,
			stage:   "request",
		},
		{
			name:    "formField only ok",
			actions: `[{"type": "setFormField", "name": "a", "value": "1"}, {"type": "removeFormField", "name": "b"}]`,
			stage:   "request",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			input := `{
				"id": "t-1",
				"name": "test",
				"version": "1.0",
				"rules": [{
					"id": "r1",
					"name": "r1",
					"enabled": true,
					"priority": 0,
					"stage": "` + tt.stage + `",
					"match": {},
					"actions": ` + tt.actions + `
				}]
			}`

			cfg := cfgFromParse(t, input)
			err := validateConfig(cfg)
			if tt.wantErr && err == nil {
				t.Errorf("expected error but got nil")
			}
			if !tt.wantErr && err != nil {
				t.Errorf("unexpected error: %v", err)
			}
		})
	}
}

func cfgFromParse(t *testing.T, input string) *Config {
	t.Helper()
	cfg, err := parseConfig([]byte(input))
	if err != nil {
		t.Fatalf("parseConfig: %v", err)
	}
	return cfg
}

func TestApplyBodyTransforms(t *testing.T) {
	i := &Intercept{}

	actions := []Action{
		{Type: "replaceBodyText", Search: "old", Replace: "new"},
		{Type: "patchBodyJson", Patches: []JSONPatch{{Op: "add", Path: "extra", Value: 42}}},
		{Type: "appendBody", Value: "suffix"},
	}

	result := i.applyBodyTransforms(actions, `{"name":"old","count":1}`)
	if !strings.Contains(result, "new") {
		t.Errorf("replaceBodyText not applied: %s", result)
	}
	if !strings.Contains(result, "extra") {
		t.Errorf("patchBodyJson not applied: %s", result)
	}
	if !strings.HasSuffix(result, "suffix") {
		t.Errorf("appendBody not applied: %s", result)
	}
}

func TestApplyBodyTransformsSetBody(t *testing.T) {
	i := &Intercept{}

	actions := []Action{
		{Type: "replaceBodyText", Search: "old", Replace: "new"},
		{Type: "setBody", Value: "final"},
	}

	result := i.applyBodyTransforms(actions, "original")
	if result != "final" {
		t.Errorf("setBody should override earlier transforms, got: %s", result)
	}
}
