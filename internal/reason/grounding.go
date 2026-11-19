package reason

// SourceSupportsDifference exposes the existing target-comparison grounding
// gate so the runtime can re-check Differences proposed by any Reasoner, not
// only the ones validated inside the Gemma path. It adds no new rule.
func SourceSupportsDifference(source, difference string) bool {
	return sourceSupportsDifference(source, difference)
}

// SourceSupportsFact exposes the existing source-grounding check for facts.
// It adds no new rule.
func SourceSupportsFact(source, fact string) bool {
	return sourceSupportsFact(source, fact)
}
