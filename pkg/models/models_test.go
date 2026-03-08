package models

import "testing"

func TestHealthStatusConstants(t *testing.T) {
	if StatusHealthy != "healthy" {
		t.Errorf("expected healthy, got %s", StatusHealthy)
	}
	if StatusDegraded != "degraded" {
		t.Errorf("expected degraded, got %s", StatusDegraded)
	}
	if StatusError != "error" {
		t.Errorf("expected error, got %s", StatusError)
	}
}

func TestSpecFormatConstants(t *testing.T) {
	if SpecOpenAPI3 != "openapi3" {
		t.Errorf("expected openapi3, got %s", SpecOpenAPI3)
	}
	if SpecOpenAPI2 != "openapi2" {
		t.Errorf("expected openapi2, got %s", SpecOpenAPI2)
	}
	if SpecAsyncAPI != "asyncapi" {
		t.Errorf("expected asyncapi, got %s", SpecAsyncAPI)
	}
	if SpecUnknown != "unknown" {
		t.Errorf("expected unknown, got %s", SpecUnknown)
	}
}

func TestConnectorTypeConstants(t *testing.T) {
	if ConnectorGravitee != "gravitee" {
		t.Errorf("expected gravitee, got %s", ConnectorGravitee)
	}
	if ConnectorKong != "kong" {
		t.Errorf("expected kong, got %s", ConnectorKong)
	}
	if ConnectorOpenAPI != "openapi" {
		t.Errorf("expected openapi, got %s", ConnectorOpenAPI)
	}
	if ConnectorTrafficLogs != "traffic-logs" {
		t.Errorf("expected traffic-logs, got %s", ConnectorTrafficLogs)
	}
}

func TestDetectSpecFormat(t *testing.T) {
	tests := []struct {
		name     string
		content  string
		expected SpecFormat
	}{
		{
			name:     "OpenAPI 3 JSON",
			content:  `{"openapi": "3.0.1", "info": {"title": "My API"}}`,
			expected: SpecOpenAPI3,
		},
		{
			name:     "OpenAPI 3 YAML",
			content:  "openapi: 3.0.0\ninfo:\n  title: My API",
			expected: SpecOpenAPI3,
		},
		{
			name:     "OpenAPI 2 / Swagger JSON",
			content:  `{"swagger": "2.0", "info": {"title": "Old API"}}`,
			expected: SpecOpenAPI2,
		},
		{
			name:     "OpenAPI 2 / Swagger YAML",
			content:  "swagger: \"2.0\"\ninfo:\n  title: Old API",
			expected: SpecOpenAPI2,
		},
		{
			name:     "AsyncAPI JSON",
			content:  `{"asyncapi": "2.6.0", "info": {"title": "Events"}}`,
			expected: SpecAsyncAPI,
		},
		{
			name:     "AsyncAPI YAML",
			content:  "asyncapi: 2.6.0\ninfo:\n  title: Events",
			expected: SpecAsyncAPI,
		},
		{
			name:     "Unknown format",
			content:  `{"type": "something else entirely"}`,
			expected: SpecUnknown,
		},
		{
			name:     "Empty string",
			content:  "",
			expected: SpecUnknown,
		},
		{
			name:     "AsyncAPI takes priority over OpenAPI",
			content:  `{"asyncapi": "2.0", "openapi": "3.0"}`,
			expected: SpecAsyncAPI,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := DetectSpecFormat(tt.content)
			if got != tt.expected {
				t.Errorf("DetectSpecFormat() = %s, want %s", got, tt.expected)
			}
		})
	}
}

func TestDetectSpecFormatTruncatesAt2KB(t *testing.T) {
	// Put the keyword beyond the 2KB mark — should not be detected.
	padding := make([]byte, 2100)
	for i := range padding {
		padding[i] = 'x'
	}
	content := string(padding) + `"openapi": "3.0.0"`
	got := DetectSpecFormat(content)
	if got != SpecUnknown {
		t.Errorf("expected unknown for keyword beyond 2KB, got %s", got)
	}

	// Put the keyword within the 2KB mark — should be detected.
	content2 := `{"openapi": "3.0.0"}` + string(padding)
	got2 := DetectSpecFormat(content2)
	if got2 != SpecOpenAPI3 {
		t.Errorf("expected openapi3 for keyword within 2KB, got %s", got2)
	}
}
