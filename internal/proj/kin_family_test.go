package proj

import (
	"testing"

	"emergion-sovereign-runtime/internal/core"
)

func kinUpload(id string) core.EmergION {
	return core.EmergION{IDN: id, STA: core.StateAccepted, REL: map[string]string{"source_kind": "SOURCE_UPLOAD", "source_title": "t " + id}}
}

func kinLink(id, from, to string) core.EmergION {
	return core.EmergION{IDN: id, STA: core.StateAccepted, REL: map[string]string{
		"source_kind": "SOURCE_KIN_LINK", "kin_link_from": from, "kin_link_to": to, "kin_link_basis": "reasoner",
	}}
}

func familySizes(lp LinkedProjection) (fam map[string]string, multi int, total int) {
	fam = map[string]string{}
	count := map[string]int{}
	for _, r := range lp.Records {
		fam[r.ID] = r.KinFamily
		count[r.KinFamily]++
	}
	for _, n := range count {
		if n > 1 {
			multi++
		}
	}
	return fam, multi, len(count)
}

// Reproduction: an accepted worker SOURCE_KIN_LINK between two accepted
// uploads leaves every record in its own engine kin root, because
// AcceptedKinRoot follows only EVO.Supersedes.
func TestKinRootIgnoresWorkerKinLinks(t *testing.T) {
	st := core.EmptyState()
	st.Accepted["E-u1"] = kinUpload("E-u1")
	st.Accepted["E-u2"] = kinUpload("E-u2")
	st.Accepted["E-l1"] = kinLink("E-l1", "E-u1", "E-u2")
	lp := Linked(st)
	roots := map[string]int{}
	for _, r := range lp.Records {
		roots[r.KinRoot]++
	}
	if len(roots) != 3 {
		t.Fatalf("engine kin roots = %v, want 3 singletons (Supersedes lineage only)", roots)
	}
}

func TestKinFamilyJoinsAcceptedWorkerLinks(t *testing.T) {
	st := core.EmptyState()
	for _, id := range []string{"E-u1", "E-u2", "E-u3", "E-u4", "E-u5"} {
		st.Accepted[id] = kinUpload(id)
	}
	// Chain u1–u2–u3 through two accepted links.
	st.Accepted["E-l1"] = kinLink("E-l1", "E-u1", "E-u2")
	st.Accepted["E-l2"] = kinLink("E-l2", "E-u2", "E-u3")
	// A link that is only AtGOV / Held never joins a family.
	st.AtGOV["E-l3"] = kinLink("E-l3", "E-u4", "E-u5")
	st.Held["E-l4"] = kinLink("E-l4", "E-u4", "E-u5")
	// An accepted link with an end that is not accepted joins nothing.
	st.Accepted["E-l5"] = kinLink("E-l5", "E-u4", "E-missing")
	// Supersedes lineage still joins: u6 supersedes u5.
	u6 := kinUpload("E-u6")
	u6.EVO.Supersedes = "E-u5"
	st.Accepted["E-u6"] = u6

	lp := Linked(st)
	fam, multi, total := familySizes(lp)
	want := fam["E-u1"]
	for _, id := range []string{"E-u2", "E-u3", "E-l1", "E-l2"} {
		if fam[id] != want {
			t.Fatalf("%s family %q, want %q (%v)", id, fam[id], want, fam)
		}
	}
	if want != "E-l1" {
		t.Fatalf("family id %q, want smallest engine kin root E-l1", want)
	}
	if fam["E-u4"] != "E-u4" || fam["E-l5"] != "E-l5" {
		t.Fatalf("unaccepted links joined: %v", fam)
	}
	if fam["E-u6"] != "E-u5" || fam["E-u5"] != "E-u5" {
		t.Fatalf("Supersedes lineage lost: %v", fam)
	}
	// Families: {u1,u2,u3,l1,l2}, {u5,u6}, {u4}, {l5}.
	if total != 4 || multi != 2 {
		t.Fatalf("families total %d multi %d, want 4 and 2: %v", total, multi, fam)
	}
	// Engine kin roots stay Supersedes-only.
	for _, r := range lp.Records {
		if r.ID == "E-u2" && r.KinRoot != "E-u2" {
			t.Fatalf("engine kin root changed: %+v", r)
		}
	}
	if len(st.Accepted) != 9 {
		t.Fatal("state mutated")
	}
}
