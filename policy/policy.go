// Package policy holds the A01 auto-accept amendment (A01-AUTO-1).
//
// Approved by Michael (owner) as HUMAN_FINAL on 2026-11-20: low-risk records
// may be decided by a versioned, hash-identified policy instead of a human
// press. The policy is OFF until a HUMAN_FINAL-approved AUTO_POLICY_SWITCH
// record naming this exact policy hash is REG-accepted. Every auto decision is
// a normal GOV decision receipt with authority AUTO_POLICY (never
// HUMAN_FINAL) and, for APPROVE, a normal REG receipt. Replay re-checks every
// AUTO_POLICY receipt against the switch state and this classifier, so a
// forged or out-of-policy auto decision fails closed.
package policy

import (
	"crypto/sha256"
	_ "embed"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"sort"
	"strconv"
	"strings"

	"emergion-sovereign-runtime/internal/core"
)

//go:embed auto-accept.json
var policyJSON []byte

// Authority is the decision source of every policy decision. It is distinct
// from HUMAN_FINAL on purpose.
const Authority = "AUTO_POLICY"

// Decision kinds a human uses to undo an auto decision.
const (
	// Revert moves an AUTO_POLICY REG-accepted record to HELD. HUMAN_FINAL only.
	Revert = "REVERT"
)

// Record kinds the policy and the worker know.
const (
	KindSourceUpload = "SOURCE_UPLOAD"
	KindSwitch       = "AUTO_POLICY_SWITCH"
	KindKinLink      = "SOURCE_KIN_LINK"
	KindInsight      = "SOURCE_INSIGHT"

	ProvenanceSourceUpload = "governed_source_upload"
	ProvenanceSwitch       = "governed_policy_switch"
	ProvenanceKinLink      = "worker_source_kin_link"
	ProvenanceInsight      = "worker_source_insight"
)

// Classes (must match auto-accept.json).
const (
	ClassNone            = ""
	ClassSourceUpload    = "SOURCE_UPLOAD_VERIFIED"
	ClassDuplicate       = "EXACT_DUPLICATE"
	ClassGenerated       = "GENERATED_PROVIDER_COMBINATION"
	ClassKinLinkAccepted = "SOURCE_KIN_LINK_ACCEPTED_ENDS"
)

// Document is the parsed policy file.
type Document struct {
	Policy    string `json:"policy"`
	Version   string `json:"version"`
	Amends    string `json:"amends"`
	Approved  string `json:"approved"`
	Authority string `json:"authority"`
	Default   string `json:"default"`
	Classes   []struct {
		Class    string `json:"class"`
		Decision string `json:"decision"`
		Rule     string `json:"rule"`
	} `json:"classes"`
	Never []string `json:"never"`
}

var doc Document

func init() {
	if err := json.Unmarshal(policyJSON, &doc); err != nil {
		panic("policy/auto-accept.json: " + err.Error())
	}
}

// Version is the policy version (A01-AUTO-1).
func Version() string { return doc.Version }

// SHA256 is the hex sha256 of the exact embedded policy file.
func SHA256() string {
	h := sha256.Sum256(policyJSON)
	return hex.EncodeToString(h[:])
}

// Doc returns the parsed policy file.
func Doc() Document { return doc }

// Bytes returns the embedded policy file.
func Bytes() []byte { return append([]byte(nil), policyJSON...) }

// Verdict is one classification.
type Verdict struct {
	Class    string `json:"class"`
	Decision string `json:"decision"` // APPROVE, HOLD or "" (needs HUMAN_FINAL)
	Why      string `json:"why"`
}

// Status is the effective switch state derived from the ledger.
type Status struct {
	On       bool   `json:"on"`
	Version  string `json:"version,omitempty"`
	SHA256   string `json:"sha256,omitempty"`
	SwitchID string `json:"switch_id,omitempty"`
}

// SwitchState reads a switch record. ok is false when em is not one.
func SwitchState(em core.EmergION) (on bool, version, hash string, ok bool) {
	if strings.TrimSpace(em.REL["source_kind"]) != KindSwitch || em.MEM.Provenance != ProvenanceSwitch {
		return false, "", "", false
	}
	state := strings.ToLower(strings.TrimSpace(em.REL["policy_state"]))
	if state != "on" && state != "off" {
		return false, "", "", false
	}
	return state == "on", strings.TrimSpace(em.REL["policy_version"]), strings.TrimSpace(em.REL["policy_sha256"]), true
}

// SwitchDescriptor is the preserved evidence of one switch record. at keeps
// two presses of the same state distinct in the content-addressed store.
func SwitchDescriptor(on bool, reason, at string) []byte {
	state := "off"
	if on {
		state = "on"
	}
	return []byte(fmt.Sprintf("AUTO_POLICY_SWITCH/1\nSTATE:%s\nVERSION:%s\nSHA256:%s\nAT:%s\nREASON:%s\nZ\n",
		state, Version(), SHA256(), at, oneLine(reason)))
}

// ReasonText is the reason every AUTO_POLICY receipt carries.
func ReasonText(v Verdict, switchID string) string {
	return fmt.Sprintf("AUTO_POLICY %s sha256=%s class=%s switch=%s: %s",
		Version(), SHA256(), v.Class, switchID, v.Why)
}

func oneLine(s string) string { return strings.Join(strings.Fields(s), " ") }

// Index is the duplicate index: key -> sorted IDNs carrying it, plus the
// state of every known record. Rebuild keeps one incrementally; Build makes
// one from a state.
type Index struct {
	byKey map[string][]string
}

func NewIndex() *Index { return &Index{byKey: map[string][]string{}} }

func dupKeys(em core.EmergION) []string {
	var keys []string
	if strings.TrimSpace(em.REL["source_kind"]) == KindSourceUpload {
		if h := strings.ToLower(strings.TrimSpace(em.REL["source_sha256"])); h != "" {
			keys = append(keys, "upload:"+h)
		}
	}
	if h := strings.TrimSpace(em.MEM.SourceHash); h != "" {
		keys = append(keys, "src:"+h)
	}
	return keys
}

// Add records em (call once per candidate event).
func (x *Index) Add(em core.EmergION) {
	for _, k := range dupKeys(em) {
		ids := x.byKey[k]
		i := sort.SearchStrings(ids, em.IDN)
		if i < len(ids) && ids[i] == em.IDN {
			continue
		}
		ids = append(ids, "")
		copy(ids[i+1:], ids[i:])
		ids[i] = em.IDN
		x.byKey[k] = ids
	}
}

// BuildIndex indexes every record in st.
func BuildIndex(st core.State) *Index {
	x := NewIndex()
	for _, set := range []map[string]core.EmergION{st.AtGOV, st.Approved, st.Accepted, st.Held, st.Rejected, st.Returned} {
		for _, em := range set {
			x.Add(em)
		}
	}
	return x
}

func stateOf(st core.State, id string) string {
	switch {
	case has(st.AtGOV, id):
		return core.StateAtGOV
	case has(st.Accepted, id):
		return core.StateAccepted
	case has(st.Approved, id):
		return core.StateApproved
	case has(st.Held, id):
		return core.StateHeld
	case has(st.Rejected, id):
		return core.StateRejected
	case has(st.Returned, id):
		return core.StateReturned
	}
	return ""
}

func has(m map[string]core.EmergION, id string) bool { _, ok := m[id]; return ok }

// duplicateOf returns the record em duplicates: one sharing a key that is
// already decided, or that is AtGOV with a smaller IDN (the smallest keeps).
func (x *Index) duplicateOf(st core.State, em core.EmergION) string {
	for _, k := range dupKeys(em) {
		for _, id := range x.byKey[k] {
			if id == em.IDN {
				continue
			}
			s := stateOf(st, id)
			if s == "" {
				continue
			}
			if s != core.StateAtGOV || id < em.IDN {
				return id
			}
		}
	}
	return ""
}

var uploadREL = map[string]bool{
	"protector": true, "governed_state": true, "source_kind": true, "source_sha256": true,
	"source_path": true, "source_title": true, "source_bytes": true, "source_topic": true,
	"source_mime": true, "source_captured_at": true, "source_related": true,
}

var linkREL = map[string]bool{
	"protector": true, "governed_state": true, "source_kind": true, "source_topic": true,
	"kin_link_from": true, "kin_link_to": true, "kin_link_basis": true, "kin_link_why": true,
	"worker_version": true,
}

// lowRiskShape is the structural guard every APPROVE class shares.
func lowRiskShape(em core.EmergION, allowed map[string]bool) string {
	if em.STA != core.StateAtGOV {
		return "not AtGOV"
	}
	if !em.VAL.Recoil || !em.VAL.WVC {
		return "RECOIL/WVC not passed"
	}
	if em.VAL.Risk != "L" {
		return "risk is not L"
	}
	if len(em.CAP) != 1 || em.CAP[0] != "OBS" {
		return "capabilities other than OBS"
	}
	if strings.TrimSpace(em.EVO.Supersedes) != "" || len(em.EVO.Delta) > 0 {
		return "lineage change"
	}
	if m := em.EVO.Metadata; m != nil {
		if strings.TrimSpace(m.Archonym) != "" {
			return "claims an Archonym"
		}
		if len(m.Facets) > 0 || len(m.BuildNodes) > 0 || len(m.BuildEdges) > 0 || m.Monetization != nil {
			return "facets, build graph or monetization"
		}
	}
	for k, v := range em.REL {
		if !allowed[k] {
			return "REL key " + k + " is not low-risk"
		}
		if k == "protector" && v != "NO_EXTERNAL_AUTHORITY_CLAIMED" {
			return "protector gate " + v
		}
	}
	return ""
}

func validHex64(v string) bool {
	if len(v) != 64 {
		return false
	}
	for _, c := range v {
		if !(c >= '0' && c <= '9' || c >= 'a' && c <= 'f') {
			return false
		}
	}
	return true
}

// Classify decides which policy class em (AtGOV) falls in, using st and the
// duplicate index x. Decision "" means HUMAN_FINAL is required. It is pure
// and deterministic so replay can re-run it.
func Classify(st core.State, x *Index, em core.EmergION) Verdict {
	kind := strings.TrimSpace(em.REL["source_kind"])
	if em.STA != core.StateAtGOV {
		return Verdict{Why: "not AtGOV"}
	}
	if kind == KindSwitch || em.MEM.Provenance == ProvenanceSwitch {
		return Verdict{Why: "the policy itself needs HUMAN_FINAL"}
	}
	if strings.TrimSpace(em.REL["source_name"]) == "provider_population" {
		return Verdict{Class: ClassGenerated, Decision: "HOLD", Why: "generated provider combination"}
	}
	if x != nil {
		if orig := x.duplicateOf(st, em); orig != "" {
			return Verdict{Class: ClassDuplicate, Decision: "HOLD", Why: "exact duplicate of " + orig}
		}
	}
	switch kind {
	case KindSourceUpload:
		if em.MEM.Provenance != ProvenanceSourceUpload {
			return Verdict{Why: "SOURCE_UPLOAD not from governed_source_upload"}
		}
		if !validHex64(em.REL["source_sha256"]) {
			return Verdict{Why: "source_sha256 invalid"}
		}
		if strings.TrimSpace(em.REL["source_path"]) == "" || strings.TrimSpace(em.REL["source_title"]) == "" {
			return Verdict{Why: "manifest fields missing"}
		}
		if _, err := strconv.ParseInt(em.REL["source_bytes"], 10, 64); err != nil {
			return Verdict{Why: "source_bytes invalid"}
		}
		if why := lowRiskShape(em, uploadREL); why != "" {
			return Verdict{Why: why}
		}
		return Verdict{Class: ClassSourceUpload, Decision: "APPROVE", Why: "verified SOURCE upload " + em.REL["source_sha256"][:12]}
	case KindKinLink:
		if em.MEM.Provenance != ProvenanceKinLink {
			return Verdict{Why: "SOURCE_KIN_LINK not from the worker"}
		}
		from, to := strings.TrimSpace(em.REL["kin_link_from"]), strings.TrimSpace(em.REL["kin_link_to"])
		if from == "" || to == "" || from == to {
			return Verdict{Why: "link ends invalid"}
		}
		for _, end := range []string{from, to} {
			a, ok := st.Accepted[end]
			if !ok || strings.TrimSpace(a.REL["source_kind"]) != KindSourceUpload {
				return Verdict{Why: "link end " + end + " is not a REG-accepted SOURCE_UPLOAD"}
			}
		}
		if why := lowRiskShape(em, linkREL); why != "" {
			return Verdict{Why: why}
		}
		return Verdict{Class: ClassKinLinkAccepted, Decision: "APPROVE", Why: "link between accepted sources " + from + " and " + to}
	}
	return Verdict{Why: "not a low-risk class"}
}

// descriptorLines parses "KEY:value" lines of a worker/ingest descriptor.
func descriptorLines(b []byte, header string) (map[string]string, error) {
	lines := strings.Split(string(b), "\n")
	if len(lines) == 0 || lines[0] != header {
		return nil, fmt.Errorf("descriptor is not %s", header)
	}
	out := map[string]string{}
	for _, l := range lines[1:] {
		k, v, ok := strings.Cut(l, ":")
		if ok {
			out[k] = v
		}
	}
	return out, nil
}

// VerifyEvidence is the decide-time check on the preserved evidence bytes
// (already sha256-verified against the record by the store). fileHash, when
// not empty, is the sha256 of the source file found on this device.
func VerifyEvidence(v Verdict, em core.EmergION, evidence []byte, fileHash string) error {
	switch v.Class {
	case ClassSourceUpload:
		d, err := descriptorLines(evidence, "SOURCE_UPLOAD/1")
		if err != nil {
			return err
		}
		if d["SHA256"] != em.REL["source_sha256"] || d["PATH"] != em.REL["source_path"] ||
			d["TITLE"] != em.REL["source_title"] || d["BYTES"] != em.REL["source_bytes"] {
			return fmt.Errorf("descriptor does not match the record")
		}
		if fileHash != "" && fileHash != em.REL["source_sha256"] {
			return fmt.Errorf("file sha256 does not match the manifest")
		}
	case ClassKinLinkAccepted:
		d, err := descriptorLines(evidence, "SOURCE_KIN_LINK/1")
		if err != nil {
			return err
		}
		if d["FROM"] != em.REL["kin_link_from"] || d["TO"] != em.REL["kin_link_to"] {
			return fmt.Errorf("link descriptor does not match the record")
		}
	}
	return nil
}

// ReplayCheck validates one AUTO_POLICY decision receipt during replay.
func ReplayCheck(st core.State, x *Index, status Status, em core.EmergION, d core.DecisionReceipt) error {
	if d.Authority != Authority {
		return fmt.Errorf("not an AUTO_POLICY decision")
	}
	if !status.On {
		return fmt.Errorf("AUTO_POLICY decision while the policy is off")
	}
	if !strings.Contains(d.Reason, "AUTO_POLICY "+status.Version+" sha256="+status.SHA256+" ") {
		return fmt.Errorf("AUTO_POLICY decision does not name the active policy")
	}
	if status.Version != Version() {
		return fmt.Errorf("AUTO_POLICY version %s has no classifier", status.Version)
	}
	if d.Decision != "APPROVE" && d.Decision != "HOLD" {
		return fmt.Errorf("AUTO_POLICY may only APPROVE or HOLD")
	}
	v := Classify(st, x, em)
	if v.Decision != d.Decision {
		return fmt.Errorf("AUTO_POLICY %s of %s is outside the policy (%s)", d.Decision, em.IDN, v.Why)
	}
	if !strings.Contains(d.Reason, "class="+v.Class+" ") {
		return fmt.Errorf("AUTO_POLICY class mismatch for %s", em.IDN)
	}
	return nil
}
