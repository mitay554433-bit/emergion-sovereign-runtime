package policy

import (
	"path"
	"strings"
	"unicode"

	"emergion-sovereign-runtime/internal/core"
)

// Worker quality rules (the "quality" section of auto-accept.json). The
// worker skips sources that fail them, and the policy HOLDs worker links and
// insights that cite them (class WORKER_NOISE; RESUME is one HUMAN_FINAL
// press). Pure and deterministic, so replay re-runs them.

// ClassWorkerNoise is the HOLD class for worker records that fail quality.
const ClassWorkerNoise = "WORKER_NOISE"

// Quality is the parsed quality section.
type Quality struct {
	Note               string   `json:"note"`
	RawTitleMarkers    []string `json:"raw_title_markers"`
	NonCanonTitleTerms []string `json:"non_canon_title_terms"`
	GenericTopics      []string `json:"generic_topics"`
	GenericTerms       []string `json:"generic_terms"`
}

func lowerSet(xs []string) map[string]bool {
	m := map[string]bool{}
	for _, x := range xs {
		m[strings.ToLower(strings.TrimSpace(x))] = true
	}
	return m
}

func quality() Quality {
	if doc.Quality == nil {
		return Quality{}
	}
	return *doc.Quality
}

// TitleNoise reports why a SOURCE title is not a real canon document ("" when
// it is): empty, an ID or hash, a raw path or carrying a raw marker, or
// non-canon.
func TitleNoise(title string) string {
	t := strings.TrimSpace(title)
	if t == "" {
		return "no title"
	}
	q := quality()
	lt := strings.ToLower(t)
	for _, m := range q.NonCanonTitleTerms {
		if m = strings.ToLower(strings.TrimSpace(m)); m != "" && strings.Contains(lt, m) {
			return "non-canon source (" + strings.TrimSpace(m) + ")"
		}
	}
	for _, m := range q.RawTitleMarkers {
		if m = strings.ToLower(m); m != "" && strings.Contains(lt, m) {
			return "title is a raw path or id (" + strings.TrimSpace(m) + ")"
		}
	}
	if strings.HasPrefix(t, "/") || strings.HasPrefix(t, "~/") || strings.Count(t, "/") >= 2 || strings.Contains(t, `\`) {
		return "title is a raw path"
	}
	if validHex64(lt) || isIDN(t) {
		return "title is an id"
	}
	if len(titleTerms(t)) == 0 {
		return "title has no words"
	}
	return ""
}

func isIDN(t string) bool {
	if !strings.HasPrefix(t, "E-") || len(t) < 8 {
		return false
	}
	for _, c := range t[2:] {
		if !(c >= '0' && c <= '9' || c >= 'A' && c <= 'F' || c >= 'a' && c <= 'f') {
			return false
		}
	}
	return true
}

// titleTerms are the meaningful words of a title: letters/digits, at least 3
// characters, not all digits, not a generic term. The file extension is
// dropped.
func titleTerms(title string) map[string]bool {
	t := strings.ToLower(strings.TrimSpace(title))
	if ext := path.Ext(t); ext != "" && len(ext) <= 6 {
		t = strings.TrimSuffix(t, ext)
	}
	generic := lowerSet(quality().GenericTerms)
	out := map[string]bool{}
	for _, f := range strings.FieldsFunc(t, func(r rune) bool { return !unicode.IsLetter(r) && !unicode.IsDigit(r) }) {
		if len([]rune(f)) < 3 || generic[f] {
			continue
		}
		digits := true
		for _, r := range f {
			if !unicode.IsDigit(r) {
				digits = false
				break
			}
		}
		if !digits {
			out[f] = true
		}
	}
	return out
}

// SharedTopic reports whether two SOURCE records share a real topic: the same
// non-generic source_topic, or at least one meaningful title term.
func SharedTopic(a, b core.EmergION) bool {
	ta := strings.ToLower(strings.TrimSpace(a.REL["source_topic"]))
	tb := strings.ToLower(strings.TrimSpace(b.REL["source_topic"]))
	if ta != "" && ta == tb && !lowerSet(quality().GenericTopics)[ta] {
		return true
	}
	wb := titleTerms(b.REL["source_title"])
	for w := range titleTerms(a.REL["source_title"]) {
		if wb[w] {
			return true
		}
	}
	return false
}

// findRecord returns the record with id in any state.
func findRecord(st core.State, id string) (core.EmergION, string, bool) {
	for _, s := range []struct {
		m   map[string]core.EmergION
		sta string
	}{{st.AtGOV, core.StateAtGOV}, {st.Approved, core.StateApproved}, {st.Accepted, core.StateAccepted}, {st.Held, core.StateHeld}, {st.Rejected, core.StateRejected}, {st.Returned, core.StateReturned}} {
		if em, ok := s.m[id]; ok {
			return em, s.sta, true
		}
	}
	return core.EmergION{}, "", false
}

// SourceNoise reports why the SOURCE record id is not a usable worker source
// ("" when it is): missing, on hold, or a title that fails TitleNoise.
func SourceNoise(st core.State, id string) string {
	em, sta, ok := findRecord(st, id)
	if !ok {
		return "source " + id + " missing"
	}
	if sta == core.StateHeld {
		return "source " + id + " is on hold"
	}
	if why := TitleNoise(em.REL["source_title"]); why != "" {
		return "source " + id + ": " + why
	}
	return ""
}

// LinkNoise reports why a worker link between from and to is noise ("" when
// it is a real link between two real documents sharing a topic).
func LinkNoise(st core.State, from, to string) string {
	for _, id := range []string{from, to} {
		if why := SourceNoise(st, id); why != "" {
			return why
		}
	}
	a, _, _ := findRecord(st, from)
	b, _, _ := findRecord(st, to)
	if !SharedTopic(a, b) {
		return "no shared topic between " + from + " and " + to
	}
	return ""
}
