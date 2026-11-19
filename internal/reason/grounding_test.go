package reason

import "testing"

func TestExportedGroundingWrappersMatchOriginals(t *testing.T) {
	source := "TARGET_COMPARISON/1\nTARGET_EMERGION:E-T\nTARGET_STATE:\ninvoice ledger reconciled\nREALITY:\nE-A\nZ\n"
	for _, difference := range []string{
		"TARGET_STATE!=ACCEPTED_REALITY",
		"payments automatically transfer to customers",
		"",
	} {
		if SourceSupportsDifference(source, difference) != sourceSupportsDifference(source, difference) {
			t.Fatalf("SourceSupportsDifference diverged for %q", difference)
		}
	}
	if !SourceSupportsDifference(source, "TARGET_STATE!=ACCEPTED_REALITY") {
		t.Fatal("canonical symbolic Difference not grounded")
	}
	if SourceSupportsFact("invoice ledger exported", "invoice ledger reconciled") != sourceSupportsFact("invoice ledger exported", "invoice ledger reconciled") {
		t.Fatal("SourceSupportsFact diverged")
	}
}
