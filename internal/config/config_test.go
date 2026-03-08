package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func writeConfigFile(t *testing.T, content string) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "agent.yaml")
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestLoadValidConfig(t *testing.T) {
	yaml := `
agent:
  id: agent-test-01
  name: Test Agent
cloud:
  endpoint: https://api.example.com
  api-key: secret-key
connectors:
  - type: openapi
    name: local-specs
    url: /tmp/specs
    sync-interval: 10m
`
	cfg, err := Load(writeConfigFile(t, yaml))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.Agent.ID != "agent-test-01" {
		t.Errorf("agent.id = %q, want %q", cfg.Agent.ID, "agent-test-01")
	}
	if cfg.Agent.Name != "Test Agent" {
		t.Errorf("agent.name = %q, want %q", cfg.Agent.Name, "Test Agent")
	}
	if cfg.Cloud.Endpoint != "https://api.example.com" {
		t.Errorf("cloud.endpoint = %q", cfg.Cloud.Endpoint)
	}
	if cfg.Cloud.APIKey != "secret-key" {
		t.Errorf("cloud.api-key = %q", cfg.Cloud.APIKey)
	}
	if len(cfg.Connectors) != 1 {
		t.Fatalf("expected 1 connector, got %d", len(cfg.Connectors))
	}
	if cfg.Connectors[0].SyncInterval != 10*time.Minute {
		t.Errorf("sync-interval = %v, want 10m", cfg.Connectors[0].SyncInterval)
	}
}

func TestLoadDefaultSyncInterval(t *testing.T) {
	yaml := `
agent:
  id: agent-01
cloud:
  endpoint: https://api.example.com
  api-key: key
connectors:
  - type: kong
    name: kong-prod
    url: http://kong:8001
`
	cfg, err := Load(writeConfigFile(t, yaml))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.Connectors[0].SyncInterval != 5*time.Minute {
		t.Errorf("expected default 5m sync interval, got %v", cfg.Connectors[0].SyncInterval)
	}
}

func TestLoadEnvExpansion(t *testing.T) {
	t.Setenv("TEST_AGENT_ID", "from-env")
	t.Setenv("TEST_API_KEY", "env-secret")

	yaml := `
agent:
  id: ${TEST_AGENT_ID}
cloud:
  endpoint: https://api.example.com
  api-key: ${TEST_API_KEY}
connectors:
  - type: openapi
    name: specs
    url: /tmp
`
	cfg, err := Load(writeConfigFile(t, yaml))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.Agent.ID != "from-env" {
		t.Errorf("agent.id = %q, want %q", cfg.Agent.ID, "from-env")
	}
	if cfg.Cloud.APIKey != "env-secret" {
		t.Errorf("cloud.api-key = %q, want %q", cfg.Cloud.APIKey, "env-secret")
	}
}

func TestLoadMissingAgentID(t *testing.T) {
	yaml := `
agent:
  name: Test
cloud:
  endpoint: https://api.example.com
  api-key: key
connectors:
  - type: openapi
    name: specs
    url: /tmp
`
	_, err := Load(writeConfigFile(t, yaml))
	if err == nil || !strings.Contains(err.Error(), "agent.id is required") {
		t.Errorf("expected agent.id validation error, got: %v", err)
	}
}

func TestLoadMissingCloudEndpoint(t *testing.T) {
	yaml := `
agent:
  id: agent-01
cloud:
  api-key: key
connectors:
  - type: openapi
    name: specs
    url: /tmp
`
	_, err := Load(writeConfigFile(t, yaml))
	if err == nil || !strings.Contains(err.Error(), "cloud.endpoint is required") {
		t.Errorf("expected cloud.endpoint validation error, got: %v", err)
	}
}

func TestLoadMissingCloudAPIKey(t *testing.T) {
	yaml := `
agent:
  id: agent-01
cloud:
  endpoint: https://api.example.com
connectors:
  - type: openapi
    name: specs
    url: /tmp
`
	_, err := Load(writeConfigFile(t, yaml))
	if err == nil || !strings.Contains(err.Error(), "cloud.api-key is required") {
		t.Errorf("expected cloud.api-key validation error, got: %v", err)
	}
}

func TestLoadNoConnectors(t *testing.T) {
	yaml := `
agent:
  id: agent-01
cloud:
  endpoint: https://api.example.com
  api-key: key
`
	_, err := Load(writeConfigFile(t, yaml))
	if err == nil || !strings.Contains(err.Error(), "at least one connector") {
		t.Errorf("expected connector validation error, got: %v", err)
	}
}

func TestLoadConnectorMissingType(t *testing.T) {
	yaml := `
agent:
  id: agent-01
cloud:
  endpoint: https://api.example.com
  api-key: key
connectors:
  - name: specs
    url: /tmp
`
	_, err := Load(writeConfigFile(t, yaml))
	if err == nil || !strings.Contains(err.Error(), "type is required") {
		t.Errorf("expected connector type validation error, got: %v", err)
	}
}

func TestLoadFileNotFound(t *testing.T) {
	_, err := Load("/nonexistent/path/agent.yaml")
	if err == nil {
		t.Error("expected error for nonexistent file")
	}
}

func TestLoadMalformedYAML(t *testing.T) {
	yaml := `{{{invalid yaml`
	_, err := Load(writeConfigFile(t, yaml))
	if err == nil || !strings.Contains(err.Error(), "parse config") {
		t.Errorf("expected parse error, got: %v", err)
	}
}

func TestLoadAuthConfig(t *testing.T) {
	yaml := `
agent:
  id: agent-01
cloud:
  endpoint: https://api.example.com
  api-key: key
connectors:
  - type: gravitee
    name: gravitee-prod
    url: http://gravitee:8083
    auth:
      type: bearer
      token: my-token
`
	cfg, err := Load(writeConfigFile(t, yaml))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	auth := cfg.Connectors[0].Auth
	if auth.Type != "bearer" {
		t.Errorf("auth.type = %q, want %q", auth.Type, "bearer")
	}
	if auth.Token != "my-token" {
		t.Errorf("auth.token = %q, want %q", auth.Token, "my-token")
	}
}
