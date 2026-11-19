package store

import (
	"fmt"
	"strings"

	"emergion-sovereign-runtime/internal/core"
)

// LedgerTip returns the last event's SelfHash and the event count.
// An empty ledger has an empty tip hash and a count of zero.
func LedgerTip(events []core.Event) (tipHash string, count int) {
	count = len(events)
	if count > 0 {
		tipHash = events[count-1].SelfHash
	}
	return tipHash, count
}

// CheckExpectedTip compares the ledger tip and/or event count to an
// operator-supplied receipt. An empty expectTip and a nil expectCount
// mean "not supplied"; then this is a no-op and today's verify behavior
// is unchanged. The ledger format is not modified.
func CheckExpectedTip(events []core.Event, expectTip string, expectCount *int) error {
	expectTip = strings.TrimSpace(expectTip)
	if expectTip == "" && expectCount == nil {
		return nil
	}

	actualTip, actualCount := LedgerTip(events)
	var parts []string
	if expectTip != "" && expectTip != actualTip {
		actual := actualTip
		if actual == "" {
			actual = "(empty)"
		}
		parts = append(parts, fmt.Sprintf("expected tip %s, actual %s", expectTip, actual))
	}
	if expectCount != nil && *expectCount != actualCount {
		parts = append(parts, fmt.Sprintf("expected count %d, actual %d", *expectCount, actualCount))
	}
	if len(parts) == 0 {
		return nil
	}
	return fmt.Errorf("tip mismatch: %s", strings.Join(parts, "; "))
}
