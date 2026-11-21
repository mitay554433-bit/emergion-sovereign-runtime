package policy

// Gap classes a SOURCE_GAP carries in REL gap_class, in display order, and
// the PIVOT_* action each one suggests in REL gap_pivot. The suggestion is
// text only: nothing runs from it.
const (
	GapOperational = "operational"
	GapCommercial  = "commercial"
	GapIntegration = "integration"
	GapContinuity  = "continuity"
	GapProof       = "proof"
)

// GapClasses is the fixed class order.
var GapClasses = []string{GapOperational, GapCommercial, GapIntegration, GapContinuity, GapProof}

// GapPivot maps a gap class to its suggested PIVOT_* action.
var GapPivot = map[string]string{
	GapOperational: "PIVOT_FAIL_CLOSED_REPAIR",
	GapCommercial:  "PIVOT_PRODUCT_SURVIVAL",
	GapIntegration: "PIVOT_ADAPTER_LAYER",
	GapContinuity:  "PIVOT_CONTINUITY_RECOVERY",
	GapProof:       "PIVOT_EVIDENCE_BINDING",
}
