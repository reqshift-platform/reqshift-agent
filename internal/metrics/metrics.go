package metrics

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

var (
	// SyncsTotal counts sync operations by connector and status.
	SyncsTotal = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "reqshift_syncs_total",
		Help: "Total number of sync operations.",
	}, []string{"connector", "status"})

	// SyncDuration observes sync duration in seconds by connector.
	SyncDuration = promauto.NewHistogramVec(prometheus.HistogramOpts{
		Name:    "reqshift_sync_duration_seconds",
		Help:    "Duration of sync operations in seconds.",
		Buckets: prometheus.DefBuckets,
	}, []string{"connector"})

	// SpecsDiscovered tracks the number of specs discovered per connector.
	SpecsDiscovered = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Name: "reqshift_specs_discovered",
		Help: "Number of API specs discovered per connector.",
	}, []string{"connector"})

	// PushErrorsTotal counts push failures.
	PushErrorsTotal = promauto.NewCounter(prometheus.CounterOpts{
		Name: "reqshift_push_errors_total",
		Help: "Total number of push errors to cloud.",
	})

	// AgentUp is always 1 when the agent is running.
	AgentUp = promauto.NewGauge(prometheus.GaugeOpts{
		Name: "reqshift_agent_up",
		Help: "Whether the agent is up (always 1).",
	})
)

// Init sets initial metric values.
func Init() {
	AgentUp.Set(1)
}
