package metrics

import (
	"testing"

	"github.com/prometheus/client_golang/prometheus"
)

func TestInitSetsAgentUp(t *testing.T) {
	Init()

	// Gather metrics to verify AgentUp is set.
	mfs, err := prometheus.DefaultGatherer.Gather()
	if err != nil {
		t.Fatalf("gather: %v", err)
	}

	found := false
	for _, mf := range mfs {
		if mf.GetName() == "reqshift_agent_up" {
			found = true
			if len(mf.GetMetric()) == 0 {
				t.Error("agent_up has no metrics")
				break
			}
			val := mf.GetMetric()[0].GetGauge().GetValue()
			if val != 1 {
				t.Errorf("agent_up = %v, want 1", val)
			}
			break
		}
	}
	if !found {
		t.Error("reqshift_agent_up metric not found")
	}
}

func TestSyncsTotalIncrement(t *testing.T) {
	SyncsTotal.WithLabelValues("test-conn", "success").Inc()
	SyncsTotal.WithLabelValues("test-conn", "error").Inc()

	mfs, err := prometheus.DefaultGatherer.Gather()
	if err != nil {
		t.Fatalf("gather: %v", err)
	}

	for _, mf := range mfs {
		if mf.GetName() == "reqshift_syncs_total" {
			if len(mf.GetMetric()) < 2 {
				t.Error("expected at least 2 metric series")
			}
			return
		}
	}
	t.Error("reqshift_syncs_total not found")
}

func TestSpecsDiscoveredGauge(t *testing.T) {
	SpecsDiscovered.WithLabelValues("gauge-test").Set(42)

	mfs, err := prometheus.DefaultGatherer.Gather()
	if err != nil {
		t.Fatalf("gather: %v", err)
	}

	for _, mf := range mfs {
		if mf.GetName() == "reqshift_specs_discovered" {
			for _, m := range mf.GetMetric() {
				for _, lp := range m.GetLabel() {
					if lp.GetName() == "connector" && lp.GetValue() == "gauge-test" {
						if m.GetGauge().GetValue() != 42 {
							t.Errorf("specs_discovered = %v, want 42", m.GetGauge().GetValue())
						}
						return
					}
				}
			}
		}
	}
	t.Error("reqshift_specs_discovered for gauge-test not found")
}
