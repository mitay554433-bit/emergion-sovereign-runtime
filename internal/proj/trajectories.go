package proj

import (
	"sort"
	"strings"
	"time"

	"emergion-sovereign-runtime/internal/core"
)

// LinkedTrajectory is one capability provider trajectory as a single derived
// row: every AtGOV provider_population candidate with the same
// (required_capability, capability_composition, first provider). Read-only;
// the ledger keeps every member record unchanged.
type LinkedTrajectory struct {
	Key                   string    `json:"key"`
	RequiredCapability    string    `json:"required_capability"`
	CapabilityComposition string    `json:"capability_composition"`
	FirstProvider         string    `json:"first_provider"`
	FirstProviderTitle    string    `json:"first_provider_title,omitempty"`
	Count                 int       `json:"count"`
	FirstAt               time.Time `json:"first_at"`
	LastAt                time.Time `json:"last_at"`
	NewestID              string    `json:"newest_id"`
	Members               []string  `json:"members"`
}

// IsProviderTrajectory reports whether em was proposed by the capability
// provider population (runtime materializeCapabilityProviderPopulation).
func IsProviderTrajectory(em core.EmergION) bool {
	return strings.TrimSpace(em.REL["source_name"]) == "provider_population"
}

// FirstCapabilityProvider returns the provider ID of the first recipe part of
// a "PART:ID,PART:ID" capability_providers path.
func FirstCapabilityProvider(providers string) string {
	first, _, _ := strings.Cut(strings.TrimSpace(providers), ",")
	_, id, ok := strings.Cut(first, ":")
	if !ok {
		return ""
	}
	return strings.TrimSpace(id)
}

// TrajectoryKey is the (required_capability, capability_composition, first
// provider) key shared by the projection and the runtime emitter.
func TrajectoryKey(em core.EmergION) string {
	return strings.TrimSpace(em.REL["required_capability"]) + "|" +
		strings.TrimSpace(em.REL["capability_composition"]) + "|" +
		FirstCapabilityProvider(em.REL["capability_providers"])
}

// Trajectories groups the AtGOV provider trajectories in st. Capture times
// come from each member's first C event in events. Largest first.
func Trajectories(st core.State, events []core.Event) []LinkedTrajectory {
	at := map[string]time.Time{}
	for _, ev := range events {
		if ev.Type != "C" || ev.EmergION == nil {
			continue
		}
		if _, seen := at[ev.EmergION.IDN]; !seen {
			at[ev.EmergION.IDN] = ev.At
		}
	}
	by := map[string]*LinkedTrajectory{}
	for id, em := range st.AtGOV {
		if !IsProviderTrajectory(em) {
			continue
		}
		key := TrajectoryKey(em)
		t := by[key]
		if t == nil {
			first := FirstCapabilityProvider(em.REL["capability_providers"])
			t = &LinkedTrajectory{
				Key:                   key,
				RequiredCapability:    strings.TrimSpace(em.REL["required_capability"]),
				CapabilityComposition: strings.TrimSpace(em.REL["capability_composition"]),
				FirstProvider:         first,
				FirstProviderTitle:    recordTitle(st, first),
				Members:               []string{},
			}
			by[key] = t
		}
		t.Count++
		t.Members = append(t.Members, id)
		when := at[id]
		if t.FirstAt.IsZero() || when.Before(t.FirstAt) {
			t.FirstAt = when
		}
		if t.NewestID == "" || when.After(t.LastAt) || (when.Equal(t.LastAt) && id > t.NewestID) {
			t.LastAt, t.NewestID = when, id
		}
	}
	out := make([]LinkedTrajectory, 0, len(by))
	for _, t := range by {
		sort.Strings(t.Members)
		out = append(out, *t)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Count != out[j].Count {
			return out[i].Count > out[j].Count
		}
		return out[i].Key < out[j].Key
	})
	return out
}

func recordTitle(st core.State, id string) string {
	for _, m := range []map[string]core.EmergION{st.Accepted, st.AtGOV} {
		if em, ok := m[id]; ok {
			if em.EVO.Metadata != nil && strings.TrimSpace(em.EVO.Metadata.Archonym) != "" {
				return em.EVO.Metadata.Archonym
			}
			return strings.TrimSpace(em.MEM.Summary)
		}
	}
	return ""
}
