package config

import (
	"fmt"
	"os"
	"time"

	"gopkg.in/yaml.v3"
)

// Config represents the full agent configuration.
type Config struct {
	Agent      AgentConfig       `yaml:"agent"`
	Cloud      CloudConfig       `yaml:"cloud"`
	Connectors []ConnectorConfig `yaml:"connectors"`
}

type AgentConfig struct {
	ID   string `yaml:"id"`
	Name string `yaml:"name"`
}

type CloudConfig struct {
	Endpoint string `yaml:"endpoint"`
	APIKey   string `yaml:"api-key"`
}

type ConnectorConfig struct {
	Type         string            `yaml:"type"` // gravitee, kong, openapi, traffic-logs
	Name         string            `yaml:"name"`
	URL          string            `yaml:"url"`
	Auth         AuthConfig        `yaml:"auth"`
	SyncInterval time.Duration     `yaml:"sync-interval"`
	Options      map[string]string `yaml:"options"`
}

type AuthConfig struct {
	Type     string `yaml:"type"` // bearer, basic, apikey
	Token    string `yaml:"token"`
	Username string `yaml:"username"`
	Password string `yaml:"password"`
}

// Load reads and parses the YAML config file.
// Environment variables in values (${VAR}) are expanded.
func Load(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read config: %w", err)
	}

	expanded := os.ExpandEnv(string(data))

	var cfg Config
	if err := yaml.Unmarshal([]byte(expanded), &cfg); err != nil {
		return nil, fmt.Errorf("parse config: %w", err)
	}

	if err := cfg.validate(); err != nil {
		return nil, fmt.Errorf("validate config: %w", err)
	}

	for i := range cfg.Connectors {
		if cfg.Connectors[i].SyncInterval == 0 {
			cfg.Connectors[i].SyncInterval = 5 * time.Minute
		}
	}

	return &cfg, nil
}

func (c *Config) validate() error {
	if c.Agent.ID == "" {
		return fmt.Errorf("agent.id is required")
	}
	if c.Cloud.Endpoint == "" {
		return fmt.Errorf("cloud.endpoint is required")
	}
	if c.Cloud.APIKey == "" {
		return fmt.Errorf("cloud.api-key is required")
	}
	if len(c.Connectors) == 0 {
		return fmt.Errorf("at least one connector is required")
	}
	for i, conn := range c.Connectors {
		if conn.Type == "" {
			return fmt.Errorf("connectors[%d].type is required", i)
		}
	}
	return nil
}
