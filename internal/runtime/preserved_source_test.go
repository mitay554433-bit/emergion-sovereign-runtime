package runtime

import (
	"strings"
	"testing"

	"emergion-sovereign-runtime/internal/core"
	"emergion-sovereign-runtime/internal/store"
)

// attachPreservedSource makes a GOV fixture's source hash correspond to a
// real preserved evidence object. If the claimed hash is already preserved,
// the EmergION is left unchanged so Capture-built fixtures stay as they are.
func attachPreservedSource(t *testing.T, s *store.Store, em *core.EmergION) {
	t.Helper()
	hash := strings.TrimSpace(em.MEM.SourceHash)
	if hash != "" {
		if _, err := s.ReadEvidence(hash); err == nil {
			return
		}
	}
	raw := []byte(em.IDN)
	if hash != "" {
		raw = []byte(em.IDN + " " + hash + "\n")
	}
	ev, err := s.Preserve(raw)
	if err != nil {
		t.Fatal(err)
	}
	em.MEM.SourceHash = ev.Hash
	em.MEM.Codec = ev.Codec
	em.MEM.Bytes = ev.Bytes
	em.MEM.Stored = ev.Stored
}
