package proj

import (
	"sort"
	"strings"

	"emergion-sovereign-runtime/internal/core"
	"emergion-sovereign-runtime/policy"
)

// LinkedGapGroup is one gap class with its SOURCE_GAP records. Derived,
// read-only, rebuilt on every call; never authority.
type LinkedGapGroup struct {
	Class    string          `json:"class"`
	Pivot    string          `json:"pivot,omitempty"`
	Accepted int             `json:"accepted"`
	AtGOV    int             `json:"at_gov"`
	Held     int             `json:"held"`
	Items    []LinkedGapItem `json:"items"`
}

// LinkedGapItem is one SOURCE_GAP that is REG-accepted or AtGOV.
type LinkedGapItem struct {
	ID          string `json:"id"`
	State       string `json:"state"`
	Of          string `json:"of"`
	SourceTitle string `json:"source_title,omitempty"`
	Basis       string `json:"basis,omitempty"`
	Pivot       string `json:"pivot,omitempty"`
	Summary     string `json:"summary,omitempty"`
}

func sourceTitle(st core.State, id string) string {
	for _, set := range []map[string]core.EmergION{st.Accepted, st.AtGOV, st.Approved, st.Held, st.Returned} {
		if em, ok := set[id]; ok {
			return strings.TrimSpace(em.REL["source_title"])
		}
	}
	return ""
}

// gapGroups groups SOURCE_GAP records by REL gap_class in policy.GapClasses
// order (any other class after, by name). Accepted and AtGOV records are
// listed; held ones are counted only.
func gapGroups(st core.State) []LinkedGapGroup {
	byClass := map[string]*LinkedGapGroup{}
	get := func(class string) *LinkedGapGroup {
		g := byClass[class]
		if g == nil {
			g = &LinkedGapGroup{Class: class, Pivot: policy.GapPivot[class], Items: []LinkedGapItem{}}
			byClass[class] = g
		}
		return g
	}
	add := func(set map[string]core.EmergION, state string) {
		for id, em := range set {
			if strings.TrimSpace(em.REL["source_kind"]) != policy.KindGap {
				continue
			}
			class := strings.TrimSpace(em.REL["gap_class"])
			if class == "" {
				class = "unclassified"
			}
			g := get(class)
			switch state {
			case "REG_ACCEPTED":
				g.Accepted++
			case "AtGOV":
				g.AtGOV++
			case "HELD":
				g.Held++
				continue
			}
			of := strings.TrimSpace(em.REL["gap_of"])
			g.Items = append(g.Items, LinkedGapItem{
				ID: id, State: state, Of: of, SourceTitle: sourceTitle(st, of),
				Basis: strings.TrimSpace(em.REL["gap_basis"]), Pivot: strings.TrimSpace(em.REL["gap_pivot"]),
				Summary: strings.TrimSpace(em.MEM.Summary),
			})
		}
	}
	add(st.AtGOV, "AtGOV")
	add(st.Accepted, "REG_ACCEPTED")
	add(st.Held, "HELD")

	order := map[string]int{}
	for i, c := range policy.GapClasses {
		order[c] = i
	}
	out := make([]LinkedGapGroup, 0, len(byClass))
	for _, g := range byClass {
		sort.Slice(g.Items, func(i, j int) bool {
			if g.Items[i].State != g.Items[j].State {
				return g.Items[i].State == "AtGOV"
			}
			return g.Items[i].ID < g.Items[j].ID
		})
		out = append(out, *g)
	}
	sort.Slice(out, func(i, j int) bool {
		oi, iok := order[out[i].Class]
		oj, jok := order[out[j].Class]
		if iok != jok {
			return iok
		}
		if iok && oi != oj {
			return oi < oj
		}
		return out[i].Class < out[j].Class
	})
	return out
}

// LinkedGapSummary is the real SOURCE_GAP totals in every GOV state plus the
// accepted worker insights the gap pass turns into SOURCE_GAP candidates.
type LinkedGapSummary struct {
	Records          int `json:"records"`
	Accepted         int `json:"accepted"`
	AtGOV            int `json:"at_gov"`
	Held             int `json:"held"`
	Other            int `json:"other"` // approved (not yet REG), rejected, returned
	Classes          int `json:"classes"`
	Sources          int `json:"sources"` // distinct sources with a gap record
	InsightsAccepted int `json:"insights_accepted"`
}

func gapSummary(st core.State) LinkedGapSummary {
	var s LinkedGapSummary
	classes, sources := map[string]bool{}, map[string]bool{}
	count := func(set map[string]core.EmergION, n *int) {
		for _, em := range set {
			switch strings.TrimSpace(em.REL["source_kind"]) {
			case policy.KindGap:
				s.Records++
				*n++
				c := strings.TrimSpace(em.REL["gap_class"])
				if c == "" {
					c = "unclassified"
				}
				classes[c] = true
				if of := strings.TrimSpace(em.REL["gap_of"]); of != "" {
					sources[of] = true
				}
			}
		}
	}
	count(st.Accepted, &s.Accepted)
	count(st.AtGOV, &s.AtGOV)
	count(st.Held, &s.Held)
	count(st.Approved, &s.Other)
	count(st.Rejected, &s.Other)
	count(st.Returned, &s.Other)
	for _, em := range st.Accepted {
		if strings.TrimSpace(em.REL["source_kind"]) == policy.KindInsight {
			s.InsightsAccepted++
		}
	}
	s.Classes, s.Sources = len(classes), len(sources)
	return s
}
