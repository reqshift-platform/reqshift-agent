package delta

import (
	"crypto/sha256"
	"encoding/hex"
	"sync"

	"github.com/reqshift-platform/reqshift-agent/pkg/models"
)

// Tracker maintains per-connector content hashes to compute deltas between syncs.
type Tracker struct {
	mu     sync.Mutex
	hashes map[string]map[string]string // connector → apiID → sha256(specContent)
}

// NewTracker creates a new delta tracker.
func NewTracker() *Tracker {
	return &Tracker{
		hashes: make(map[string]map[string]string),
	}
}

// Compare computes the delta between the current specs and the previously known state.
// Returns changed specs, deleted IDs, and whether this is a full sync (first time for this connector).
func (t *Tracker) Compare(connector string, specs []models.APISpec) (changed []models.APISpec, deleted []string, fullSync bool) {
	t.mu.Lock()
	defer t.mu.Unlock()

	prev, exists := t.hashes[connector]
	if !exists {
		// First sync for this connector — full sync.
		return specs, nil, true
	}

	// Build current hash map.
	current := make(map[string]string, len(specs))
	for _, spec := range specs {
		current[spec.APIID] = hashSpec(spec)
	}

	// Find changed or new specs.
	for _, spec := range specs {
		h := current[spec.APIID]
		if prevHash, ok := prev[spec.APIID]; !ok || prevHash != h {
			changed = append(changed, spec)
		}
	}

	// Find deleted specs.
	for id := range prev {
		if _, ok := current[id]; !ok {
			deleted = append(deleted, id)
		}
	}

	return changed, deleted, false
}

// Update records the current state of specs for the given connector.
// Call this after a successful push.
func (t *Tracker) Update(connector string, specs []models.APISpec) {
	t.mu.Lock()
	defer t.mu.Unlock()

	m := make(map[string]string, len(specs))
	for _, spec := range specs {
		m[spec.APIID] = hashSpec(spec)
	}
	t.hashes[connector] = m
}

func hashSpec(spec models.APISpec) string {
	h := sha256.New()
	h.Write([]byte(spec.SpecContent))
	h.Write([]byte(spec.Version))
	h.Write([]byte(spec.APIName))
	return hex.EncodeToString(h.Sum(nil))
}
