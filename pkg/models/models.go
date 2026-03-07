package models

import (
	"strings"
	"time"
)

// HealthStatus represents the agent or connector health state.
type HealthStatus string

const (
	StatusHealthy  HealthStatus = "healthy"
	StatusDegraded HealthStatus = "degraded"
	StatusError    HealthStatus = "error"
)

// SpecFormat represents the format of an API specification.
type SpecFormat string

const (
	SpecOpenAPI3 SpecFormat = "openapi3"
	SpecOpenAPI2 SpecFormat = "openapi2"
	SpecAsyncAPI SpecFormat = "asyncapi"
	SpecUnknown  SpecFormat = "unknown"
)

// ConnectorType identifies a supported connector.
const (
	ConnectorGravitee    = "gravitee"
	ConnectorKong        = "kong"
	ConnectorOpenAPI     = "openapi"
	ConnectorTrafficLogs = "traffic-logs"
)

// DetectSpecFormat inspects the beginning of a spec file to determine its format.
func DetectSpecFormat(content string) SpecFormat {
	// Only inspect the first 2KB for performance on large specs.
	if len(content) > 2048 {
		content = content[:2048]
	}
	if strings.Contains(content, `"asyncapi"`) || strings.Contains(content, "asyncapi:") {
		return SpecAsyncAPI
	}
	if strings.Contains(content, `"swagger"`) || strings.Contains(content, "swagger:") {
		return SpecOpenAPI2
	}
	if strings.Contains(content, `"openapi"`) || strings.Contains(content, "openapi:") {
		return SpecOpenAPI3
	}
	return SpecUnknown
}

// SyncPayload is the top-level payload sent to the Reqshift Ingestion API.
type SyncPayload struct {
	AgentID       string       `json:"agentId"`
	AgentVersion  string       `json:"agentVersion"`
	Timestamp     time.Time    `json:"timestamp"`
	ConnectorType string       `json:"connectorType"`
	ConnectorName string       `json:"connectorName"`
	Specs         []APISpec    `json:"specs,omitempty"`
	Metrics       []APIMetrics `json:"metrics,omitempty"`
	Health        *AgentHealth `json:"health,omitempty"`
}

// APISpec represents a discovered API specification.
type APISpec struct {
	APIID        string            `json:"apiId"`
	APIName      string            `json:"apiName"`
	Version      string            `json:"version,omitempty"`
	BasePath     string            `json:"basePath,omitempty"`
	SpecFormat   SpecFormat        `json:"specFormat,omitempty"`
	SpecContent  string            `json:"specContent,omitempty"`
	Metadata     map[string]string `json:"metadata,omitempty"`
	Tags         []string          `json:"tags,omitempty"`
	LastModified time.Time         `json:"lastModified,omitempty"`
}

// APIMetrics holds runtime performance data for an API.
type APIMetrics struct {
	APIID         string    `json:"apiId"`
	RequestCount  int64     `json:"requestCount"`
	LatencyP50Ms  float64   `json:"latencyP50Ms"`
	LatencyP95Ms  float64   `json:"latencyP95Ms"`
	LatencyP99Ms  float64   `json:"latencyP99Ms"`
	ErrorRate     float64   `json:"errorRate"`
	UptimePercent float64   `json:"uptimePercent"`
	PeriodStart   time.Time `json:"periodStart"`
	PeriodEnd     time.Time `json:"periodEnd"`
}

// AgentHealth reports the agent's own status.
type AgentHealth struct {
	Status          HealthStatus      `json:"status"`
	UptimeSeconds   int64             `json:"uptimeSeconds"`
	MemoryUsedMB    int64             `json:"memoryUsedMb"`
	LastSyncAt      time.Time         `json:"lastSyncAt"`
	LastError       string            `json:"lastError,omitempty"`
	ConnectorStatus map[string]string `json:"connectorStatus,omitempty"`
}

// SyncResponse is what the Ingestion API returns.
type SyncResponse struct {
	Status          string `json:"status"`
	SpecsIngested   int    `json:"specsIngested"`
	MetricsIngested int    `json:"metricsIngested"`
	NextSyncIn      int    `json:"nextSyncIn"` // seconds
}
