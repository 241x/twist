package app

import (
	"encoding/json"
	"fmt"
	"os"
)

type Config struct {
	ID          string                 `json:"id"`
	Name        string                 `json:"name"`
	Version     string                 `json:"version"`
	Description string                 `json:"description,omitempty"`
	Settings    map[string]any         `json:"settings,omitempty"`
	Rules       []Rule                 `json:"rules"`
}

type Rule struct {
	ID       string  `json:"id"`
	Name     string  `json:"name"`
	Enabled  bool    `json:"enabled"`
	Priority int     `json:"priority"`
	Stage    string  `json:"stage"`
	Match    Match   `json:"match"`
	Actions  []Action `json:"actions"`
}

type Match struct {
	AllOf []Condition `json:"allOf,omitempty"`
	AnyOf []Condition `json:"anyOf,omitempty"`
}

type Condition struct {
	Type    string   `json:"type"`
	Name    string   `json:"name,omitempty"`
	Value   string   `json:"value,omitempty"`
	Values  []string `json:"values,omitempty"`
	Pattern string   `json:"pattern,omitempty"`
	Path    string   `json:"path,omitempty"`
}

func (c *Condition) UnmarshalJSON(data []byte) error {
	type rawCondition Condition
	var r rawCondition
	if err := json.Unmarshal(data, &r); err != nil {
		return err
	}

	if err := validateCondition(Condition(r)); err != nil {
		return err
	}

	*c = Condition(r)
	return nil
}

func validateCondition(c Condition) error {
	switch c.Type {
	case "urlEquals", "urlPrefix", "urlSuffix", "urlContains":
		if c.Value == "" {
			return fmt.Errorf("condition %q requires field 'value'", c.Type)
		}
	case "urlRegex", "bodyRegex":
		if c.Pattern == "" {
			return fmt.Errorf("condition %q requires field 'pattern'", c.Type)
		}
	case "method", "resourceType":
		if len(c.Values) == 0 {
			return fmt.Errorf("condition %q requires field 'values'", c.Type)
		}
	case "headerExists", "headerNotExists", "queryExists", "queryNotExists", "cookieExists", "cookieNotExists":
		if c.Name == "" {
			return fmt.Errorf("condition %q requires field 'name'", c.Type)
		}
	case "headerEquals", "headerContains", "queryEquals", "queryContains", "cookieEquals", "cookieContains":
		if c.Name == "" {
			return fmt.Errorf("condition %q requires field 'name'", c.Type)
		}
		if c.Value == "" {
			return fmt.Errorf("condition %q requires field 'value'", c.Type)
		}
	case "headerRegex", "queryRegex", "cookieRegex":
		if c.Name == "" {
			return fmt.Errorf("condition %q requires field 'name'", c.Type)
		}
		if c.Pattern == "" {
			return fmt.Errorf("condition %q requires field 'pattern'", c.Type)
		}
	case "bodyContains":
		if c.Value == "" {
			return fmt.Errorf("condition %q requires field 'value'", c.Type)
		}
	case "bodyJsonPath":
		if c.Path == "" {
			return fmt.Errorf("condition %q requires field 'path'", c.Type)
		}
		if c.Value == "" {
			return fmt.Errorf("condition %q requires field 'value'", c.Type)
		}
	default:
		return fmt.Errorf("unknown condition type: %q", c.Type)
	}
	return nil
}

type Action struct {
	Type         string            `json:"type"`
	Name         string            `json:"name,omitempty"`
	Value        any               `json:"value,omitempty"`
	Search       string            `json:"search,omitempty"`
	Replace      string            `json:"replace,omitempty"`
	ReplaceAll   bool              `json:"replaceAll,omitempty"`
	StatusCode   int               `json:"statusCode,omitempty"`
	Headers      map[string]string `json:"headers,omitempty"`
	Body         string            `json:"body,omitempty"`
	BodyEncoding string            `json:"bodyEncoding,omitempty"`
	Encoding     string            `json:"encoding,omitempty"`
	Patches      []JSONPatch       `json:"patches,omitempty"`
}

type JSONPatch struct {
	Op    string `json:"op"`
	Path  string `json:"path"`
	From  string `json:"from,omitempty"`
	Value any    `json:"value,omitempty"`
}

func (a *Action) UnmarshalJSON(data []byte) error {
	type rawAction Action
	var r rawAction
	if err := json.Unmarshal(data, &r); err != nil {
		return err
	}

	act := Action(r)
	applyActionDefaults(&act)

	if err := validateAction(act); err != nil {
		return err
	}

	*a = act
	return nil
}

func applyActionDefaults(a *Action) {
	if a.Type == "block" {
		if a.StatusCode == 0 {
			a.StatusCode = 200
		}
		if a.BodyEncoding == "" {
			a.BodyEncoding = "text"
		}
	}
	if a.Type == "setBody" {
		if a.Encoding == "" {
			a.Encoding = "text"
		}
	}
}

func validateAction(a Action) error {
	switch a.Type {
	case "setUrl", "setMethod":
		if a.Value == nil {
			return fmt.Errorf("action %q requires field 'value'", a.Type)
		}
	case "setQueryParam", "removeQueryParam", "setCookie", "removeCookie", "setFormField", "removeFormField", "setHeader", "removeHeader":
		if a.Name == "" {
			return fmt.Errorf("action %q requires field 'name'", a.Type)
		}
		if a.Type == "setQueryParam" || a.Type == "setCookie" || a.Type == "setFormField" || a.Type == "setHeader" {
			if a.Value == nil {
				return fmt.Errorf("action %q requires field 'value'", a.Type)
			}
		}
	case "block":
	case "setStatus":
		if a.Value == nil {
			return fmt.Errorf("action %q requires field 'value'", a.Type)
		}
	case "setBody":
		if a.Value == nil {
			return fmt.Errorf("action %q requires field 'value'", a.Type)
		}
	case "replaceBodyText":
		if a.Search == "" {
			return fmt.Errorf("action %q requires field 'search'", a.Type)
		}
		if a.Replace == "" {
			return fmt.Errorf("action %q requires field 'replace'", a.Type)
		}
	case "patchBodyJson":
		if len(a.Patches) == 0 {
			return fmt.Errorf("action %q requires field 'patches'", a.Type)
		}
	default:
		return fmt.Errorf("unknown action type: %q", a.Type)
	}
	return nil
}

func LoadConfig(configFile string, configData []byte) (*Config, error) {
	data, err := resolveConfigData(configFile, configData)
	if err != nil {
		return nil, err
	}

	cfg, err := parseConfig(data)
	if err != nil {
		return nil, fmt.Errorf("failed to parse config: %w", err)
	}

	if err := validateConfig(cfg); err != nil {
		return nil, fmt.Errorf("invalid config: %w", err)
	}

	return cfg, nil
}

func resolveConfigData(configFile string, configData []byte) ([]byte, error) {
	if configFile != "" {
		return os.ReadFile(configFile)
	}

	if len(configData) > 0 {
		return configData, nil
	}

	return nil, fmt.Errorf("no config available")
}

func parseConfig(data []byte) (*Config, error) {
	var cfg Config
	if err := json.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("failed to parse JSON: %w", err)
	}
	return &cfg, nil
}

func validateConfig(cfg *Config) error {
	if cfg.ID == "" {
		return fmt.Errorf("field 'id' is required")
	}
	if cfg.Name == "" {
		return fmt.Errorf("field 'name' is required")
	}
	if cfg.Version == "" {
		return fmt.Errorf("field 'version' is required")
	}
	if len(cfg.Rules) == 0 {
		return fmt.Errorf("field 'rules' must not be empty")
	}

	for i, rule := range cfg.Rules {
		if rule.ID == "" {
			return fmt.Errorf("rules[%d]: field 'id' is required", i)
		}
		if rule.Name == "" {
			return fmt.Errorf("rules[%d]: field 'name' is required", i)
		}
		if rule.Stage != "request" && rule.Stage != "response" {
			return fmt.Errorf("rules[%d]: field 'stage' must be 'request' or 'response'", i)
		}
		if len(rule.Actions) == 0 {
			return fmt.Errorf("rules[%d]: field 'actions' must not be empty", i)
		}
	}

	return nil
}
