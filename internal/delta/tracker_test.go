package delta

import (
	"testing"

	"github.com/reqshift-platform/reqshift-agent/pkg/models"
)

func TestFirstSyncIsFullSync(t *testing.T) {
	tracker := NewTracker()
	specs := []models.APISpec{
		{APIID: "api-1", APIName: "API One", SpecContent: "content-1"},
		{APIID: "api-2", APIName: "API Two", SpecContent: "content-2"},
	}

	changed, deleted, fullSync := tracker.Compare("conn-1", specs)

	if !fullSync {
		t.Error("expected fullSync=true on first sync")
	}
	if len(changed) != 2 {
		t.Errorf("expected 2 changed, got %d", len(changed))
	}
	if len(deleted) != 0 {
		t.Errorf("expected 0 deleted, got %d", len(deleted))
	}
}

func TestNoChangesAfterUpdate(t *testing.T) {
	tracker := NewTracker()
	specs := []models.APISpec{
		{APIID: "api-1", APIName: "API One", SpecContent: "content-1"},
	}

	// First sync + update.
	tracker.Compare("conn-1", specs)
	tracker.Update("conn-1", specs)

	// Second sync — same data.
	changed, deleted, fullSync := tracker.Compare("conn-1", specs)

	if fullSync {
		t.Error("expected fullSync=false on second sync")
	}
	if len(changed) != 0 {
		t.Errorf("expected 0 changed, got %d", len(changed))
	}
	if len(deleted) != 0 {
		t.Errorf("expected 0 deleted, got %d", len(deleted))
	}
}

func TestDetectsChangedSpec(t *testing.T) {
	tracker := NewTracker()
	specs := []models.APISpec{
		{APIID: "api-1", APIName: "API One", SpecContent: "content-1"},
	}

	tracker.Compare("conn-1", specs)
	tracker.Update("conn-1", specs)

	// Modify spec content.
	specs[0].SpecContent = "content-1-modified"
	changed, deleted, fullSync := tracker.Compare("conn-1", specs)

	if fullSync {
		t.Error("expected fullSync=false")
	}
	if len(changed) != 1 {
		t.Errorf("expected 1 changed, got %d", len(changed))
	}
	if changed[0].APIID != "api-1" {
		t.Errorf("changed[0].APIID = %q, want %q", changed[0].APIID, "api-1")
	}
	if len(deleted) != 0 {
		t.Errorf("expected 0 deleted, got %d", len(deleted))
	}
}

func TestDetectsDeletedSpec(t *testing.T) {
	tracker := NewTracker()
	specs := []models.APISpec{
		{APIID: "api-1", APIName: "API One", SpecContent: "content-1"},
		{APIID: "api-2", APIName: "API Two", SpecContent: "content-2"},
	}

	tracker.Compare("conn-1", specs)
	tracker.Update("conn-1", specs)

	// Remove one spec.
	remaining := specs[:1]
	changed, deleted, fullSync := tracker.Compare("conn-1", remaining)

	if fullSync {
		t.Error("expected fullSync=false")
	}
	if len(changed) != 0 {
		t.Errorf("expected 0 changed, got %d", len(changed))
	}
	if len(deleted) != 1 {
		t.Fatalf("expected 1 deleted, got %d", len(deleted))
	}
	if deleted[0] != "api-2" {
		t.Errorf("deleted[0] = %q, want %q", deleted[0], "api-2")
	}
}

func TestDetectsNewSpec(t *testing.T) {
	tracker := NewTracker()
	specs := []models.APISpec{
		{APIID: "api-1", APIName: "API One", SpecContent: "content-1"},
	}

	tracker.Compare("conn-1", specs)
	tracker.Update("conn-1", specs)

	// Add a new spec.
	specs = append(specs, models.APISpec{
		APIID: "api-3", APIName: "API Three", SpecContent: "content-3",
	})
	changed, deleted, fullSync := tracker.Compare("conn-1", specs)

	if fullSync {
		t.Error("expected fullSync=false")
	}
	if len(changed) != 1 {
		t.Errorf("expected 1 changed (new), got %d", len(changed))
	}
	if changed[0].APIID != "api-3" {
		t.Errorf("changed[0].APIID = %q, want %q", changed[0].APIID, "api-3")
	}
	if len(deleted) != 0 {
		t.Errorf("expected 0 deleted, got %d", len(deleted))
	}
}

func TestSeparateConnectors(t *testing.T) {
	tracker := NewTracker()

	tracker.Compare("conn-a", []models.APISpec{{APIID: "a1", SpecContent: "c1"}})
	tracker.Update("conn-a", []models.APISpec{{APIID: "a1", SpecContent: "c1"}})

	// conn-b is independent — should be fullSync.
	_, _, fullSync := tracker.Compare("conn-b", []models.APISpec{{APIID: "b1", SpecContent: "c2"}})
	if !fullSync {
		t.Error("expected fullSync=true for new connector")
	}

	// conn-a unchanged.
	changed, _, _ := tracker.Compare("conn-a", []models.APISpec{{APIID: "a1", SpecContent: "c1"}})
	if len(changed) != 0 {
		t.Errorf("expected 0 changed for unchanged conn-a, got %d", len(changed))
	}
}

func TestHashIncludesVersionAndName(t *testing.T) {
	tracker := NewTracker()
	specs := []models.APISpec{
		{APIID: "api-1", APIName: "API", Version: "1.0", SpecContent: "content"},
	}

	tracker.Compare("conn-1", specs)
	tracker.Update("conn-1", specs)

	// Change version only.
	specs[0].Version = "2.0"
	changed, _, _ := tracker.Compare("conn-1", specs)
	if len(changed) != 1 {
		t.Error("expected version change to be detected")
	}
}
