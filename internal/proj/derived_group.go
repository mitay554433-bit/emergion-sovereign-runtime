package proj

import (
	"regexp"
	"sort"
	"strings"

	"emergion-sovereign-runtime/internal/core"
)

// Derived display groups are a read-time grouping, never a REG facet.
//
// A REG-accepted record with no REG facet is placed in a topic group derived
// from what it already carries, in this order:
//  1. a safe REL source_topic on a SOURCE_UPLOAD record (facet_source
//     "source_topic")
//  2. keywords in its Archonym, summary, source title/path/name, or target
//     (facet_source "derived_keyword")
//  3. a known source_kind / class (facet_source "derived_kind")
//  4. the catch-all topic "other" (facet_source "derived_fallback")
//
// The grouping is rebuilt on every read, writes nothing, and changes no
// authority: REG facets stay the only canonical facets, and a record with any
// REG facet is never given a derived one. Derived keys reuse the
// SOURCE_TOPIC: prefix so they share the existing topic tiles and can never
// collide with a canonical core.Facet.

const (
	// FacetSourceKind marks a facet derived from REL source_kind.
	FacetSourceKind = "derived_kind"
	// FacetSourceKeyword marks a facet derived from title-like keywords.
	FacetSourceKeyword = "derived_keyword"
	// FacetSourceFallback marks a record that could not be grouped any
	// other way and was placed in the Other topic.
	FacetSourceFallback = "derived_fallback"
	// FacetSourceMixed marks a topic group whose members were derived in
	// more than one way.
	FacetSourceMixed = "derived"
)

// derivedKindTopics maps unambiguous system source_kind values onto the
// existing topic set. Ambiguous kinds (SOURCE, TARGET, TEST, empty, and
// SOURCE_UPLOAD, which is handled by source_topic first) are left to
// keywords, then Other.
var derivedKindTopics = map[string]string{
	"PROGRAM":                            "runtime-code-docs",
	"PROGRAM_PATCH":                      "runtime-code-docs",
	"IMPLEMENTATION":                     "runtime-code-docs",
	"IMPLEMENTATION_EVIDENCE":            "runtime-code-docs",
	"EXECUTION_OBSERVATION":              "runtime-code-docs",
	"EXECUTION_RESULT":                   "runtime-code-docs",
	"REALIZATION_SELECTION":              "runtime-code-docs",
	"REALIZATION_REORDER":                "runtime-code-docs",
	"IDLE_STATE_OBSERVATION":             "runtime-code-docs",
	"GOVERNED_EVIDENCE":                  "runtime-code-docs",
	"CAPABILITY_PROVIDER":                "runtime-code-docs",
	"DETERMINISTIC_CAPABILITY_PROVIDERS": "runtime-code-docs",
	"PROJECTION":                         "runtime-code-docs",
	"SOURCE_FILE":                        "runtime-code-docs",
	"COMPARISON":                         "runtime-code-docs",
	"TARGET_COMPARISON":                  "runtime-code-docs",
}

// derivedKeywordRules is the first-match keyword table. Order is the
// specificity used when a title could fit more than one topic.
var derivedKeywordRules = []struct {
	topic string
	re    *regexp.Regexp
}{
	{topic: "investor", re: regexp.MustCompile(`investor|buyer brief|one-?page (overview|teaser)|diligence package|acquisition package|deal.?room`)},
	{topic: "grants-funding", re: regexp.MustCompile(`\b(irap|mitacs|nserc|sbir|nrc)\b|nist sbir|\bbdc\b|funding (proposal|package)|grant application|\bgrants?\b`)},
	{topic: "business-legal", re: regexp.MustCompile(`\b(incorporation|term sheet|contract|gst|hst|registr(?:y|ies)|business (?:number|plan)|legal|employment|royalty|company summary)\b`)},
	{topic: "data", re: regexp.MustCompile(`\b(dataset|spreadsheet|inventory|csv|xlsx|jsonl|parts list|bom)\b`)},
	{topic: "inventions-ip", re: regexp.MustCompile(`\b(patent|cipo|invention|harvestskin|helix|piezo|voluntary amendment|small entity|trademark|intellectual property|prototype)\b`)},
	{topic: "math-formulas", re: regexp.MustCompile(`\b(formula|equation|simulation|mathematical|probabilistic|field equation)\b`)},
	{topic: "design-visuals", re: regexp.MustCompile(`living universe|infographic|\b(palette|render|mockup|concept art|graphic|visual|logo|figma)\b`)},
	{topic: "runtime-code-docs", re: regexp.MustCompile(`\b(fieldd|cosl|emergion|archonym|runtime|bridgegap|ecolock|golang|architecture|source code)\b|projection\.json|capsule runner`)},
	{topic: "archives", re: regexp.MustCompile(`\b(archive|backup|bundle|tarball)\b|\.zip\b|\.gz\b|\.tar\b`)},
}

func derivedKindTopic(em core.EmergION) string {
	k := strings.TrimSpace(em.REL["source_kind"])
	if k == "" {
		k = strings.TrimSpace(em.REL["class"])
	}
	return derivedKindTopics[k]
}

func derivedHaystack(em core.EmergION) string {
	parts := []string{
		em.MEM.Summary,
		em.REL["source_title"],
		em.REL["source_path"],
		em.REL["source_name"],
		em.REL["target_thing"],
		em.REL["target_state"],
	}
	if em.EVO.Metadata != nil {
		parts = append(parts, em.EVO.Metadata.Archonym)
	}
	return strings.ToLower(strings.Join(parts, "\n"))
}

func extIs(path string, exts ...string) bool {
	p := strings.ToLower(path)
	for _, e := range exts {
		if strings.HasSuffix(p, e) {
			return true
		}
	}
	return false
}

func derivedKeywordTopic(em core.EmergION) string {
	path := strings.ToLower(em.REL["source_path"] + " " + em.REL["source_title"])
	mime := strings.ToLower(em.REL["source_mime"])
	if extIs(path, ".zip", ".gz", ".tgz", ".tar", ".bundle") || strings.Contains(mime, "zip") || strings.Contains(mime, "gzip") {
		return "archives"
	}
	if extIs(path, ".csv", ".xlsx", ".xls", ".jsonl") || strings.Contains(mime, "csv") || strings.Contains(mime, "spreadsheet") || strings.Contains(mime, "excel") {
		return "data"
	}
	text := derivedHaystack(em)
	for _, rule := range derivedKeywordRules {
		if rule.re.MatchString(text) {
			return rule.topic
		}
	}
	return ""
}

// derivedDisplayGroup returns the topic slug, derived facet key and
// facet_source for an accepted record, or empty strings when the record
// already has a REG facet.
func derivedDisplayGroup(em core.EmergION, regFacets []core.Facet) (topic, facet, source string) {
	if len(regFacets) > 0 {
		return "", "", ""
	}
	if t := sourceTopicOf(em); t != "" {
		return t, SourceTopicFacetPrefix + t, FacetSourceTopic
	}
	if t := derivedKeywordTopic(em); t != "" {
		return t, SourceTopicFacetPrefix + t, FacetSourceKeyword
	}
	if t := derivedKindTopic(em); t != "" {
		return t, SourceTopicFacetPrefix + t, FacetSourceKind
	}
	return "other", SourceTopicFacetPrefix + "other", FacetSourceFallback
}

func uniqueGroupSource(srcs []string) string {
	seen := map[string]bool{}
	var uniq []string
	for _, s := range srcs {
		if s == "" || seen[s] {
			continue
		}
		seen[s] = true
		uniq = append(uniq, s)
	}
	sort.Strings(uniq)
	if len(uniq) == 1 {
		return uniq[0]
	}
	return FacetSourceMixed
}

// derivedTopicGroups groups derived members by topic, largest first, then
// by topic name, so the order is deterministic. A group of mixed origin is
// marked facet_source "derived".
func derivedTopicGroups(byTopic map[string][]string, srcByID map[string]string) []LinkedDerivedFacetGroup {
	out := make([]LinkedDerivedFacetGroup, 0, len(byTopic))
	for t, ms := range byTopic {
		ms = append([]string(nil), ms...)
		sort.Strings(ms)
		srcs := make([]string, 0, len(ms))
		for _, id := range ms {
			srcs = append(srcs, srcByID[id])
		}
		src := uniqueGroupSource(srcs)
		uploads := 0
		for _, s := range srcs {
			if s == FacetSourceTopic {
				uploads++
			}
		}
		label := SourceTopicLabel(t)
		if t == "other" && src != FacetSourceTopic {
			label = "Other"
		}
		out = append(out, LinkedDerivedFacetGroup{
			Facet:       SourceTopicFacetPrefix + t,
			Label:       label,
			Topic:       t,
			FacetSource: src,
			Members:     ms,

			SourceUploads: uploads,
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
