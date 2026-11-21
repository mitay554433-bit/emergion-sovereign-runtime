package proj

import (
	"strconv"
	"strings"
	"time"

	"emergion-sovereign-runtime/internal/core"
)

// LinkedTemporal is the live temporal view of the ledger: how many distinct
// record states the hash-chained events have produced and when the ledger
// last changed. Derived from the events on every call, read-only, never
// authority. It replaces nothing: the recorded wave file
// (outputs/real_temporal_wave.json) is still served as before.
type LinkedTemporal struct {
	Source string `json:"source"` // always "ledger"
	Events int    `json:"events"`
	// States counts distinct (record, state) pairs the ledger has produced:
	// a capture, each interpretation revision, each HUMAN_FINAL / policy
	// decision value and the REG acceptance of each record.
	States int `json:"states"`
	// Records is the number of distinct records with at least one event.
	Records int `json:"records"`
	// Revisions counts INTERPRETATION_REVISED events (record versions).
	Revisions int `json:"revisions"`
	// Days is the number of distinct UTC days with at least one event.
	Days int `json:"days"`
	// Last24h counts events in the 24 hours up to the ledger tip.
	Last24h int `json:"last_24h"`
	// TipAt is the time of the newest ledger event (any kind), TipKind its
	// kind. This is the "last update" of the FIELD.
	TipAt          string         `json:"tip_at,omitempty"`
	TipKind        string         `json:"tip_kind,omitempty"`
	TipHash        string         `json:"tip_hash,omitempty"`
	FirstAt        string         `json:"first_at,omitempty"`
	LastCaptureAt  string         `json:"last_capture_at,omitempty"`
	LastDecisionAt string         `json:"last_decision_at,omitempty"`
	LastREGAt      string         `json:"last_reg_at,omitempty"`
	ByKind         map[string]int `json:"by_kind"`
}

var temporalKinds = map[string]string{
	"C": "CANDIDATE_CAPTURED",
	"I": "INTERPRETATION_REVISED",
	"D": "DECISION",
	"R": "REG_ACCEPTED",
	"Q": "ACTION_AUTHORIZATION",
}

func eventRecordID(ev core.Event) string {
	switch {
	case ev.EmergION != nil:
		return ev.EmergION.IDN
	case ev.Decision != nil:
		return ev.Decision.EmergIONID
	case ev.REG != nil:
		return ev.REG.EmergIONID
	case ev.ActionAuthorization != nil:
		return ev.ActionAuthorization.EmergIONID
	}
	return ""
}

func rfc(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	return t.UTC().Format(time.RFC3339)
}

// Temporal derives the live temporal view from the ledger events.
func Temporal(st core.State, events []core.Event) LinkedTemporal {
	out := LinkedTemporal{Source: "ledger", Events: len(events), TipHash: st.TipHash, ByKind: map[string]int{}}
	states := map[string]bool{}
	records := map[string]bool{}
	revs := map[string]int{}
	days := map[string]bool{}
	var first, tip, lastC, lastD, lastR time.Time
	tipKind := ""
	for _, ev := range events {
		kind := temporalKinds[ev.Type]
		if kind == "" {
			kind = "EVENT_" + ev.Type
		}
		out.ByKind[kind]++
		at := ev.At
		if !at.IsZero() {
			if first.IsZero() || at.Before(first) {
				first = at
			}
			if !at.Before(tip) {
				tip, tipKind = at, kind
			}
			days[at.UTC().Format("2006-01-02")] = true
		}
		id := eventRecordID(ev)
		if id == "" {
			continue
		}
		records[id] = true
		switch ev.Type {
		case "C":
			states[id+"|C"] = true
			if at.After(lastC) {
				lastC = at
			}
		case "I":
			revs[id]++
			out.Revisions++
			states[id+"|I"+strconv.Itoa(revs[id])] = true
		case "D":
			d := ""
			if ev.Decision != nil {
				d = strings.ToUpper(strings.TrimSpace(ev.Decision.Decision))
			}
			states[id+"|D"+d] = true
			if at.After(lastD) {
				lastD = at
			}
		case "R":
			states[id+"|R"] = true
			if at.After(lastR) {
				lastR = at
			}
		}
	}
	if !tip.IsZero() {
		from := tip.Add(-24 * time.Hour)
		for _, ev := range events {
			if !ev.At.IsZero() && ev.At.After(from) {
				out.Last24h++
			}
		}
	}
	out.States, out.Records, out.Days = len(states), len(records), len(days)
	out.TipAt, out.TipKind, out.FirstAt = rfc(tip), tipKind, rfc(first)
	out.LastCaptureAt, out.LastDecisionAt, out.LastREGAt = rfc(lastC), rfc(lastD), rfc(lastR)
	return out
}
