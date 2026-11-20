package proj

import (
	"sort"
	"strings"

	"emergion-sovereign-runtime/internal/core"
)

// SOURCE topic facets are a read-time display grouping, never a REG facet.
//
// A REG-accepted SOURCE_UPLOAD record that carries no REG facet is grouped by
// the topic its manifest line recorded (REL source_topic). The grouping is
// derived on every read, is marked facet_source "source_topic", writes nothing
// and changes no authority: REG facets (EVO metadata) stay the only canonical
// facets, and a record with any REG facet is never given a derived one.

// FacetSourceTopic marks a facet derived from REL source_topic.
const FacetSourceTopic = "source_topic"

// SourceTopicFacetPrefix prefixes derived facet keys so they can never collide
// with a canonical core.Facet.
const SourceTopicFacetPrefix = "SOURCE_TOPIC:"

const sourceUploadKind = "SOURCE_UPLOAD"

var sourceTopicLabels = map[string]string{
	"inventions-ip":     "Inventions & IP",
	"runtime-code-docs": "Runtime Code & Docs",
	"grants-funding":    "Grants & Funding",
	"archives":          "Archives",
	"other":             "Other Sources",
	"design-visuals":    "Design & Visuals",
	"business-legal":    "Business & Legal",
	"math-formulas":     "Math & Formulas",
	"investor":          "Investor",
	"data":              "Data",
}

// LinkedDerivedFacetGroup is one SOURCE topic group (derived, not REG).
type LinkedDerivedFacetGroup struct {
	Facet       string   `json:"facet"`
	Label       string   `json:"label"`
	Topic       string   `json:"topic"`
	FacetSource string   `json:"facet_source"`
	Members     []string `json:"members"`
}

// LinkedSourceRefGroup lists accepted SOURCE records whose manifest lines
// name the same related id. Related ids are not EmergION ids, so this is a
// shared reference, never a KIN claim.
type LinkedSourceRefGroup struct {
	Ref     string   `json:"ref"`
	Members []string `json:"members"`
}

// normalizeSourceTopic returns a safe slug for a recorded topic, or "".
func normalizeSourceTopic(raw string) string {
	t := strings.ToLower(strings.TrimSpace(raw))
	if t == "" || len(t) > 64 {
		return ""
	}
	for _, r := range t {
		if !(r >= 'a' && r <= 'z' || r >= '0' && r <= '9' || r == '-' || r == '_') {
			return ""
		}
	}
	return t
}

// SourceTopicLabel is the readable name of a topic ("inventions-ip" ->
// "Inventions & IP"); unknown topics are title-cased.
func SourceTopicLabel(topic string) string {
	if l, ok := sourceTopicLabels[topic]; ok {
		return l
	}
	parts := strings.FieldsFunc(topic, func(r rune) bool { return r == '-' || r == '_' })
	for i, p := range parts {
		parts[i] = strings.ToUpper(p[:1]) + p[1:]
	}
	return strings.Join(parts, " ")
}

// sourceTopicOf returns the topic slug of a SOURCE_UPLOAD record, or "".
func sourceTopicOf(em core.EmergION) string {
	if strings.TrimSpace(em.REL["source_kind"]) != sourceUploadKind {
		return ""
	}
	return normalizeSourceTopic(em.REL["source_topic"])
}

// derivedSourceTopicFacet returns the derived facet key for an accepted
// record, or "" when it has a REG facet or is not a topic-bearing SOURCE.
func derivedSourceTopicFacet(em core.EmergION, regFacets []core.Facet) string {
	if len(regFacets) > 0 {
		return ""
	}
	t := sourceTopicOf(em)
	if t == "" {
		return ""
	}
	return SourceTopicFacetPrefix + t
}

// derivedSourceTopicGroups groups derived members by topic, largest first,
// then by topic name, so the order is deterministic.
func derivedSourceTopicGroups(byTopic map[string][]string) []LinkedDerivedFacetGroup {
	out := make([]LinkedDerivedFacetGroup, 0, len(byTopic))
	for t, ms := range byTopic {
		ms = append([]string(nil), ms...)
		sort.Strings(ms)
		out = append(out, LinkedDerivedFacetGroup{
			Facet:       SourceTopicFacetPrefix + t,
			Label:       SourceTopicLabel(t),
			Topic:       t,
			FacetSource: FacetSourceTopic,
			Members:     ms,
		})
	}
	sort.Slice(out, func(i, j int) bool {
		if len(out[i].Members) != len(out[j].Members) {
			return len(out[i].Members) > len(out[j].Members)
		}
		return out[i].Topic < out[j].Topic
	})
	return out
}

// sourceRefGroups lists related ids shared by at least two accepted SOURCE
// records (REL source_related), sorted by ref.
func sourceRefGroups(st core.State) []LinkedSourceRefGroup {
	by := map[string]map[string]bool{}
	for id, em := range st.Accepted {
		if strings.TrimSpace(em.REL["source_kind"]) != sourceUploadKind {
			continue
		}
		for _, r := range strings.Split(em.REL["source_related"], ",") {
			r = strings.TrimSpace(r)
			if r == "" {
				continue
			}
			if by[r] == nil {
				by[r] = map[string]bool{}
			}
			by[r][id] = true
		}
	}
	out := []LinkedSourceRefGroup{}
	for r, set := range by {
		if len(set) < 2 {
			continue
		}
		ms := make([]string, 0, len(set))
		for id := range set {
			ms = append(ms, id)
		}
		sort.Strings(ms)
		out = append(out, LinkedSourceRefGroup{Ref: r, Members: ms})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Ref < out[j].Ref })
	return out
}
