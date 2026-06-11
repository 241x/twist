package app

import (
	"fmt"
	"os"
)

type Config struct {
	Rules []Rule `json:"rules"`
}

type Rule struct {
	Name   string `json:"name"`
	URL    string `json:"url"`
	Action string `json:"action"`
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
	return &Config{}, nil
}
