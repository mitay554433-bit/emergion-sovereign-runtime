package runtime

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"emergion-sovereign-runtime/internal/adapters"
	"emergion-sovereign-runtime/internal/core"
	"emergion-sovereign-runtime/internal/emerger"
	livefield "emergion-sovereign-runtime/internal/field"
	"emergion-sovereign-runtime/internal/pivot"
	"emergion-sovereign-runtime/internal/reason"
	"emergion-sovereign-runtime/internal/store"
)

type RecaptureError struct {
	Cause    error
	EmergION core.EmergION
}

func (e *RecaptureError) Error() string {
	return fmt.Sprintf(
		"%v; RECAPTURE emerged as %s AT_GOV",
		e.Cause,
		e.EmergION.IDN,
	)
}

func (e *RecaptureError) Unwrap() error {
	return e.Cause
}

type Runtime struct {
	Store               *store.Store
	Reasoner            reason.Reasoner
	ReturnedPredecessor string
}

func (r Runtime) governedStateContext() (core.State, string, error) {
	events, err := r.Store.Events()
	if err != nil {
		return core.State{}, "", err
	}

	st, err := livefield.Rebuild(events)
	if err != nil {
		return core.State{}, "", err
	}

	ids := make([]string, 0, len(st.Accepted))
	for id := range st.Accepted {
		ids = append(ids, id)
	}
	sort.Strings(ids)

	var b strings.Builder

	write := func(key, value string) bool {
		record := fmt.Sprintf("%s=%d:%s\n", key, len(value), value)
		if b.Len()+len(record) > 12000 {
			return false
		}
		b.WriteString(record)
		return true
	}

	for _, id := range ids {
		em := st.Accepted[id]

		if !write("I", em.IDN) || !write("S", em.MEM.Summary) {
			break
		}

		for _, capability := range em.CAP {
			if !write("C", capability) {
				return st, b.String(), nil
			}
		}

		keys := make([]string, 0, len(em.REL))
		for key := range em.REL {
			keys = append(keys, key)
		}
		sort.Strings(keys)

		for _, key := range keys {
			if !write("K", key) || !write("V", em.REL[key]) {
				return st, b.String(), nil
			}
		}

		if em.EVO.Metadata != nil {
			if !write("T", string(em.EVO.Metadata.Topology)) {
				return st, b.String(), nil
			}
		}

		if !write("Z", em.IDN) {
			break
		}
	}

	return st, b.String(), nil
}

func coverage(em *core.EmergION, governedState string) error {
	var bridgegaps []string

	if strings.TrimSpace(em.MEM.SourceHash) == "" {
		bridgegaps = append(bridgegaps, "BRIDGEGAP:source_hash")
	}
	if em.MEM.Bytes <= 0 || em.MEM.Stored <= 0 {
		bridgegaps = append(bridgegaps, "BRIDGEGAP:evidence")
	}

	if strings.TrimSpace(em.MEM.Summary) == "" {
		bridgegaps = append(bridgegaps, "BRIDGEGAP:summary")
	}
	if len(em.VAL.Facts) == 0 {
		bridgegaps = append(bridgegaps, "BRIDGEGAP:facts")
	}
	if len(em.CAP) == 0 {
		bridgegaps = append(bridgegaps, "BRIDGEGAP:capabilities")
	}
	if len(em.REL) == 0 {
		bridgegaps = append(bridgegaps, "BRIDGEGAP:relationships")
	}

	if strings.TrimSpace(governedState) != "" {
		if em.REL["governed_state"] == "" {
			bridgegaps = append(bridgegaps, "BRIDGEGAP:living_state_relationship")
		}
		projected := false
		for _, fact := range em.VAL.Facts {
			if fact == "living_state_projected" {
				projected = true
				break
			}
		}
		if !projected {
			bridgegaps = append(bridgegaps, "BRIDGEGAP:living_state_projection")
		}
	}

	if len(bridgegaps) != 0 {
		em.VAL.Gaps = append(em.VAL.Gaps, bridgegaps...)
		return fmt.Errorf("COVERAGE failed: %s", strings.Join(bridgegaps, ", "))
	}

	return nil
}

func requiredCapabilityFromDivergence(result pivot.Result) string {
	if result.Name != "COVERAGE" {
		return ""
	}

	switch {
	case strings.Contains(result.Divergence, "BRIDGEGAP:capabilities"):
		return "DERIVE_CAPABILITY"
	case strings.Contains(result.Divergence, "BRIDGEGAP:facts"):
		return "ESTABLISH_FACT"
	case strings.Contains(result.Divergence, "BRIDGEGAP:relationships"):
		return "DERIVE_RELATIONSHIP"
	default:
		return ""
	}
}

func requiredCapabilityRecipe(required string) []string {
	switch strings.TrimSpace(required) {
	case "DERIVE_CAPABILITY":
		return []string{"ANALYZE", "CMP", "RLT"}
	case "ESTABLISH_FACT":
		return []string{"OBS", "VLD"}
	case "DERIVE_RELATIONSHIP":
		return []string{"CMP", "RLT"}
	default:
		return nil
	}
}

func acceptedCapabilityProviders(
	required string,
	st core.State,
) (string, bool) {
	recipe := requiredCapabilityRecipe(required)
	if len(recipe) == 0 {
		return "", false
	}

	providers := make([]string, 0, len(recipe))

	for _, requiredPart := range recipe {
		var matches []string

		for id, em := range st.Accepted {
			if em.STA != core.StateAccepted {
				continue
			}

			for _, capability := range em.CAP {
				if strings.ToUpper(strings.TrimSpace(capability)) != requiredPart {
					continue
				}

				matches = append(matches, id)
				break
			}
		}

		if len(matches) == 0 {
			return "", false
		}

		sort.Strings(matches)

		providers = append(
			providers,
			requiredPart+":"+matches[0],
		)
	}

	return strings.Join(providers, ","), true
}

type capabilityProviderEdge struct {
	From string
	To   string
}

func capabilityProviderEdges(providers string) ([]capabilityProviderEdge, error) {
	parts := strings.Split(strings.TrimSpace(providers), ",")
	if len(parts) < 2 {
		return nil, nil
	}

	ids := make([]string, 0, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		_, id, ok := strings.Cut(part, ":")
		if !ok {
			return nil, fmt.Errorf("invalid capability provider %q", part)
		}

		id = strings.TrimSpace(id)
		if id == "" {
			return nil, fmt.Errorf("empty capability provider identity in %q", part)
		}

		ids = append(ids, id)
	}

	edges := make([]capabilityProviderEdge, 0, len(ids)-1)
	for i := 0; i+1 < len(ids); i++ {
		if ids[i] == ids[i+1] {
			return nil, fmt.Errorf("capability provider edge cannot self-reference %s", ids[i])
		}

		edges = append(edges, capabilityProviderEdge{
			From: ids[i],
			To:   ids[i+1],
		})
	}

	return edges, nil
}

func (r Runtime) resolveRequiredCapability(
	em *core.EmergION,
	st core.State,
) {
	if em == nil || em.REL == nil {
		return
	}

	required := strings.TrimSpace(em.REL["required_capability"])
	if required == "" {
		return
	}

	em.REL["capability_resolution"] = "UNRESOLVED"
	delete(em.REL, "capability_composition")
	delete(em.REL, "capability_providers")

	available := map[string]bool{}

	if r.Reasoner != nil && r.Reasoner.Name() == "gemma-llama-cli" {
		validator, ok := r.Reasoner.(interface{ Validate() error })
		if ok && validator.Validate() == nil {
			for _, adapter := range adapters.Catalog(true) {
				if !adapter.Enabled {
					continue
				}
				for _, capability := range adapter.Capabilities {
					capability = strings.ToUpper(strings.TrimSpace(capability))
					if capability != "" {
						available[capability] = true
					}
				}
			}
		}
	}

	for _, capability := range em.CAP {
		capability = strings.ToUpper(strings.TrimSpace(capability))
		if capability != "" {
			available[capability] = true
		}
	}

	for _, accepted := range st.Accepted {
		for _, capability := range accepted.CAP {
			capability = strings.ToUpper(strings.TrimSpace(capability))
			if capability != "" {
				available[capability] = true
			}
		}
	}

	recipe := requiredCapabilityRecipe(required)
	if len(recipe) == 0 {
		return
	}

	for _, requiredPart := range recipe {
		if !available[requiredPart] {
			return
		}
	}

	em.REL["capability_resolution"] = "COMPOSABLE_CANDIDATE"
	em.REL["capability_composition"] = strings.Join(recipe, "+")

	if providers, ok := acceptedCapabilityProviders(required, st); ok {
		em.REL["capability_providers"] = providers
	}
}

func expectedProtectorEnvelope(capabilities []string) (string, string) {
	authority := map[string]bool{}
	catalog := adapters.Catalog(false)

	for _, capability := range capabilities {
		capability = strings.ToUpper(strings.TrimSpace(capability))
		for _, adapter := range catalog {
			for _, allowed := range adapter.Capabilities {
				if capability == allowed {
					authority[adapter.Authority] = true
				}
			}
		}
	}

	if len(authority) == 0 {
		return "NO_EXTERNAL_AUTHORITY_CLAIMED", ""
	}

	classes := make([]string, 0, len(authority))
	for class := range authority {
		classes = append(classes, class)
	}
	sort.Strings(classes)

	gate := ""
	if authority["SEND_GATED"] ||
		authority["TRANSFER_GATED"] ||
		authority["DEPLOY_GATED"] {
		gate = "HUMAN_FINAL_BOUND"
	}

	return strings.Join(classes, ","), gate
}

func protector(em *core.EmergION) error {
	if em.REL == nil {
		em.REL = map[string]string{}
	}

	// PROTECTOR owns these runtime-derived relationships. Source/model
	// supplied values cannot survive into the authority envelope.
	delete(em.REL, "protector")
	delete(em.REL, "protector_gate")

	authority := map[string]bool{}
	catalog := adapters.Catalog(false)

	for _, capability := range em.CAP {
		capability = strings.ToUpper(strings.TrimSpace(capability))
		for _, adapter := range catalog {
			for _, allowed := range adapter.Capabilities {
				if capability == allowed {
					authority[adapter.Authority] = true
				}
			}
		}
	}

	if len(authority) == 0 {
		em.REL["protector"] = "NO_EXTERNAL_AUTHORITY_CLAIMED"
	} else {
		classes := make([]string, 0, len(authority))
		for class := range authority {
			classes = append(classes, class)
		}
		sort.Strings(classes)
		em.REL["protector"] = strings.Join(classes, ",")

		if authority["SEND_GATED"] ||
			authority["TRANSFER_GATED"] ||
			authority["DEPLOY_GATED"] {
			em.REL["protector_gate"] = "HUMAN_FINAL_BOUND"
		}
	}

	_, err := pivot.Observe(
		"PROTECTOR",
		"CAPABILITY_AUTHORITY_CLAIM",
		"ADAPTER_AUTHORITY_OBSERVATION",
		"AUTHORITY_ENVELOPE_MATCH",
		func() error {
			expected, expectedGate := expectedProtectorEnvelope(em.CAP)

			if em.REL["protector"] != expected {
				return fmt.Errorf(
					"protector envelope mismatch: got %q want %q",
					em.REL["protector"],
					expected,
				)
			}

			if em.REL["protector_gate"] != expectedGate {
				return fmt.Errorf(
					"protector gate mismatch: got %q want %q",
					em.REL["protector_gate"],
					expectedGate,
				)
			}

			return nil
		},
	)
	return err
}

func stringSet(values []string) map[string]bool {
	out := map[string]bool{}
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value != "" {
			out[value] = true
		}
	}
	return out
}

func wvcEvidenceContinuity(s *store.Store, em core.EmergION) error {
	observed, err := s.InspectEvidence(em.MEM.SourceHash)
	if err != nil {
		return fmt.Errorf("evidence observation failed: %w", err)
	}

	if observed.Hash != em.MEM.SourceHash {
		return fmt.Errorf(
			"source hash mismatch: candidate %q evidence %q",
			em.MEM.SourceHash,
			observed.Hash,
		)
	}

	if observed.Bytes != em.MEM.Bytes {
		return fmt.Errorf(
			"evidence byte count mismatch: candidate %d evidence %d",
			em.MEM.Bytes,
			observed.Bytes,
		)
	}

	if observed.Stored != em.MEM.Stored {
		return fmt.Errorf(
			"stored evidence size mismatch: candidate %d evidence %d",
			em.MEM.Stored,
			observed.Stored,
		)
	}

	if observed.Codec != em.MEM.Codec {
		return fmt.Errorf(
			"evidence codec mismatch: candidate %q evidence %q",
			em.MEM.Codec,
			observed.Codec,
		)
	}

	return nil
}

func recoilIntegrity(em core.EmergION) error {
	if strings.TrimSpace(em.MEM.SourceHash) == "" {
		return fmt.Errorf("source identity missing")
	}
	if em.MEM.Bytes <= 0 || em.MEM.Stored <= 0 {
		return fmt.Errorf("evidence dimensions invalid")
	}
	if strings.TrimSpace(em.MEM.Summary) == "" {
		return fmt.Errorf("summary missing")
	}
	if len(em.VAL.Facts) == 0 {
		return fmt.Errorf("facts missing")
	}
	if len(em.CAP) == 0 {
		return fmt.Errorf("capabilities missing")
	}
	if len(em.REL) == 0 {
		return fmt.Errorf("relationships missing")
	}
	if strings.TrimSpace(em.REL["protector"]) == "" {
		return fmt.Errorf("PROTECTOR envelope missing")
	}
	return nil
}

func runtimeDerivedRelationship(key string) bool {
	return key == "protector" || key == "protector_gate"
}

func deriveFieldDelta(
	accepted map[string]core.EmergION,
	analysis reason.Result,
) []string {
	if len(accepted) == 0 {
		return []string{"F0"}
	}

	knownCaps := map[string]bool{}
	knownRelationships := map[string]map[string]bool{}
	knownFacets := map[string]bool{}

	for _, em := range accepted {
		for capability := range stringSet(em.CAP) {
			knownCaps[capability] = true
		}

		for key, value := range em.REL {
			if runtimeDerivedRelationship(key) {
				continue
			}

			if knownRelationships[key] == nil {
				knownRelationships[key] = map[string]bool{}
			}
			knownRelationships[key][value] = true
		}

		if em.EVO.Metadata != nil {
			for _, facet := range em.EVO.Metadata.Facets {
				value := strings.TrimSpace(string(facet))
				if value != "" {
					knownFacets[value] = true
				}
			}
		}
	}

	var delta []string

	caps := make([]string, 0, len(analysis.Capabilities))
	for capability := range stringSet(analysis.Capabilities) {
		caps = append(caps, capability)
	}
	sort.Strings(caps)

	for _, capability := range caps {
		if knownCaps[capability] {
			delta = append(delta, "FC:K:"+capability)
		} else {
			delta = append(delta, "FC:N:"+capability)
		}
	}

	keys := make([]string, 0, len(analysis.Relationships))
	for key := range analysis.Relationships {
		if runtimeDerivedRelationship(key) {
			continue
		}
		keys = append(keys, key)
	}
	sort.Strings(keys)

	for _, key := range keys {
		value := analysis.Relationships[key]
		knownValues, exists := knownRelationships[key]

		switch {
		case !exists:
			delta = append(delta, "FR:N:"+key)
		case knownValues[value]:
			delta = append(delta, "FR:M:"+key)
		default:
			delta = append(delta, "FR:V:"+key)
		}
	}

	facets := make([]string, 0, len(analysis.Facets))
	for facet := range stringSet(analysis.Facets) {
		facets = append(facets, facet)
	}
	sort.Strings(facets)

	for _, facet := range facets {
		if knownFacets[facet] {
			delta = append(delta, "FF:K:"+facet)
		} else {
			delta = append(delta, "FF:N:"+facet)
		}
	}

	if len(delta) == 0 {
		return []string{"F_"}
	}

	return delta
}
func deriveDelta(previous core.EmergION, analysis reason.Result) []string {
	var delta []string

	if strings.TrimSpace(previous.MEM.Summary) != strings.TrimSpace(analysis.Summary) {
		delta = append(delta, "DS")
	}

	previousCaps := stringSet(previous.CAP)
	currentCaps := stringSet(analysis.Capabilities)

	var addedCaps, removedCaps []string
	for capability := range currentCaps {
		if !previousCaps[capability] {
			addedCaps = append(addedCaps, capability)
		}
	}
	for capability := range previousCaps {
		if !currentCaps[capability] {
			removedCaps = append(removedCaps, capability)
		}
	}
	sort.Strings(addedCaps)
	sort.Strings(removedCaps)
	for _, capability := range addedCaps {
		delta = append(delta, "DC:+:"+capability)
	}
	for _, capability := range removedCaps {
		delta = append(delta, "DC:-:"+capability)
	}

	previousRelationships := previous.REL
	if previousRelationships == nil {
		previousRelationships = map[string]string{}
	}
	currentRelationships := analysis.Relationships
	if currentRelationships == nil {
		currentRelationships = map[string]string{}
	}

	var relationshipKeys []string
	seenRelationships := map[string]bool{}
	for key := range previousRelationships {
		if runtimeDerivedRelationship(key) {
			continue
		}
		seenRelationships[key] = true
		relationshipKeys = append(relationshipKeys, key)
	}
	for key := range currentRelationships {
		if runtimeDerivedRelationship(key) {
			continue
		}
		if !seenRelationships[key] {
			relationshipKeys = append(relationshipKeys, key)
		}
	}
	sort.Strings(relationshipKeys)

	for _, key := range relationshipKeys {
		previousValue, previousOK := previousRelationships[key]
		currentValue, currentOK := currentRelationships[key]
		switch {
		case !previousOK && currentOK:
			delta = append(delta, "DR:+:"+key)
		case previousOK && !currentOK:
			delta = append(delta, "DR:-:"+key)
		case previousOK && currentOK && previousValue != currentValue:
			delta = append(delta, "DR:~:"+key)
		}
	}

	previousFacets := map[string]bool{}
	if previous.EVO.Metadata != nil {
		for _, facet := range previous.EVO.Metadata.Facets {
			value := strings.TrimSpace(string(facet))
			if value != "" {
				previousFacets[value] = true
			}
		}
	}
	currentFacets := stringSet(analysis.Facets)

	var addedFacets, removedFacets []string
	for facet := range currentFacets {
		if !previousFacets[facet] {
			addedFacets = append(addedFacets, facet)
		}
	}
	for facet := range previousFacets {
		if !currentFacets[facet] {
			removedFacets = append(removedFacets, facet)
		}
	}
	sort.Strings(addedFacets)
	sort.Strings(removedFacets)
	for _, facet := range addedFacets {
		delta = append(delta, "DF:+:"+facet)
	}
	for _, facet := range removedFacets {
		delta = append(delta, "DF:-:"+facet)
	}

	previousNodes := map[string]core.BuildNode{}
	if previous.EVO.Metadata != nil {
		for _, node := range previous.EVO.Metadata.BuildNodes {
			previousNodes[node.ID] = node
		}
	}

	currentNodes := map[string]reason.BuildNode{}
	for _, node := range analysis.BuildNodes {
		currentNodes[node.ID] = node
	}

	var addedNodes, removedNodes, changedNodes []string
	for id, node := range currentNodes {
		previousNode, exists := previousNodes[id]
		if !exists {
			addedNodes = append(addedNodes, id)
			continue
		}
		if previousNode.System != node.System || previousNode.State != node.State {
			changedNodes = append(changedNodes, id)
		}
	}
	for id := range previousNodes {
		if _, exists := currentNodes[id]; !exists {
			removedNodes = append(removedNodes, id)
		}
	}

	sort.Strings(addedNodes)
	sort.Strings(removedNodes)
	sort.Strings(changedNodes)

	for _, id := range addedNodes {
		delta = append(delta, "DN:+:"+id)
	}
	for _, id := range removedNodes {
		delta = append(delta, "DN:-:"+id)
	}
	for _, id := range changedNodes {
		delta = append(delta, "DN:~:"+id)
	}

	edgeKey := func(from, to, kind string) string {
		return from + "\x00" + to + "\x00" + kind
	}
	edgeToken := func(prefix, key string) string {
		parts := strings.Split(key, "\x00")
		return prefix + parts[0] + ":" + parts[1] + ":" + parts[2]
	}

	previousEdges := map[string]bool{}
	if previous.EVO.Metadata != nil {
		for _, edge := range previous.EVO.Metadata.BuildEdges {
			previousEdges[edgeKey(edge.From, edge.To, edge.Kind)] = true
		}
	}

	currentEdges := map[string]bool{}
	for _, edge := range analysis.BuildEdges {
		currentEdges[edgeKey(edge.From, edge.To, edge.Kind)] = true
	}

	var addedEdges, removedEdges []string
	for key := range currentEdges {
		if !previousEdges[key] {
			addedEdges = append(addedEdges, key)
		}
	}
	for key := range previousEdges {
		if !currentEdges[key] {
			removedEdges = append(removedEdges, key)
		}
	}

	sort.Strings(addedEdges)
	sort.Strings(removedEdges)

	for _, key := range addedEdges {
		delta = append(delta, edgeToken("DE:+:", key))
	}
	for _, key := range removedEdges {
		delta = append(delta, edgeToken("DE:-:", key))
	}

	if len(delta) == 0 {
		return []string{"D0"}
	}
	return delta
}

func (r Runtime) validateLineage(analysis *reason.Result) error {
	if r.Reasoner.Name() != "execution-signal" {
		for _, key := range []string{
			"parent_emergion",
			"authorization_event",
			"parent",
			"origin",
			"predecessor",
			"ancestor",
			"successor",
			"kin",
			"lineage",
			"capability_provider_edge_proposal",
			"source_hash",
			"provenance",
		} {
			if value := strings.TrimSpace(analysis.Relationships[key]); value != "" {
				return fmt.Errorf(
					"lineage rejected: relationship %q is runtime-owned",
					key,
				)
			}
		}
	}
	returnedID := strings.TrimSpace(r.ReturnedPredecessor)

	events, err := r.Store.Events()
	if err != nil {
		return err
	}
	st, err := livefield.Rebuild(events)
	if err != nil {
		return err
	}

	compositionTarget := strings.TrimSpace(analysis.Relationships["COMPOSITION_KIN"])
	if compositionTarget != "" {
		if _, ok := st.Accepted[compositionTarget]; !ok {
			return fmt.Errorf(
				"composition rejected: COMPOSITION_KIN target %q is not REG-accepted",
				compositionTarget,
			)
		}
		analysis.Relationships["COMPOSITION_KIN"] = compositionTarget
	}

	if returnedID != "" {
		previous, ok := st.Returned[returnedID]
		if !ok {
			return fmt.Errorf("rework rejected: predecessor %q is not HUMAN_FINAL RETURNED", returnedID)
		}

		analysis.Supersedes = returnedID
		analysis.Delta = deriveDelta(previous, *analysis)
		return nil
	}

	if strings.TrimSpace(analysis.Supersedes) == "" {
		analysis.Supersedes = ""
		analysis.Delta = nil
		return nil
	}

	previous, ok := st.Accepted[analysis.Supersedes]
	if !ok {
		return fmt.Errorf("lineage rejected: supersedes %q is not REG-accepted", analysis.Supersedes)
	}

	analysis.Delta = deriveDelta(previous, *analysis)
	return nil
}

func (r Runtime) recapture(
	ctx context.Context,
	governedState string,
	fieldObservation []string,
	cause error,
) (core.EmergION, bool, error) {
	divergence, ok := cause.(*pivot.DivergenceError)
	if !ok {
		return core.EmergION{}, false, cause
	}

	evidenceBytes := []byte(divergence.Result.Evidence())
	evidence, err := r.Store.Preserve(evidenceBytes)
	if err != nil {
		return core.EmergION{}, false, fmt.Errorf(
			"pivot divergence could not preserve evidence: %w",
			err,
		)
	}

	em := divergence.EmergION

	if em.REL == nil {
		em.REL = map[string]string{}
	}

	if required := requiredCapabilityFromDivergence(divergence.Result); required != "" {
		em.REL["required_capability"] = required
	}

	eventsForCapabilities, err := r.Store.Events()
	if err != nil {
		_, _ = r.Store.PruneOrphans()
		return em, false, err
	}

	capabilityState, err := livefield.Rebuild(eventsForCapabilities)
	if err != nil {
		_, _ = r.Store.PruneOrphans()
		return em, false, err
	}

	r.resolveRequiredCapability(&em, capabilityState)

	delete(em.REL, "capability_provider_edge_proposal")

	if providers := strings.TrimSpace(em.REL["capability_providers"]); providers != "" {
		edges, edgeErr := capabilityProviderEdges(providers)
		if edgeErr != nil {
			_, _ = r.Store.PruneOrphans()
			return em, false, edgeErr
		}

		proposal := make([]string, 0, len(edges))
		for _, edge := range edges {
			proposal = append(proposal, edge.From+"->"+edge.To)
		}
		if len(proposal) > 0 {
			em.REL["capability_provider_edge_proposal"] = strings.Join(proposal, ",")
		}
	}

	if em.EVO.Metadata == nil {
		em.EVO.Metadata = &core.Metadata{
			Topology:     core.TopologyDodecahedronV1,
			CapturedAt:   time.Now().UTC(),
			AIIntegrated: r.Reasoner != nil && r.Reasoner.Name() != "heuristic",
			PromptSchema: "MXPD/2",
		}
	}
	em.EVO.Metadata.FieldObservation = append(
		[]string(nil),
		fieldObservation...,
	)
	em.MEM.SourceHash = evidence.Hash
	em.MEM.Codec = evidence.Codec
	em.MEM.Bytes = evidence.Bytes
	em.MEM.Stored = evidence.Stored
	em.MEM.Provenance = "reciprocal_pivot"

	if em.REL == nil {
		em.REL = map[string]string{}
	}
	if strings.TrimSpace(governedState) != "" {
		em.REL["governed_state"] = "accepted_context_present"
		em.VAL.Facts = append(em.VAL.Facts, "living_state_projected")
	}

	if err := coverage(&em, governedState); err != nil {
		_, _ = r.Store.PruneOrphans()
		return em, false, fmt.Errorf(
			"pivot divergence admission coverage failed: %w",
			err,
		)
	}

	if err := protector(&em); err != nil {
		_, _ = r.Store.PruneOrphans()
		return em, false, err
	}

	_, err = pivot.Observe(
		"RECOIL",
		"PIVOT_DIVERGENCE_CLAIM",
		"DIVERGENCE_EVIDENCE_OBSERVATION",
		"POST_RECAPTURE_PROTECTOR_INTEGRITY",
		func() error {
			return recoilIntegrity(em)
		},
	)
	if err != nil {
		_, _ = r.Store.PruneOrphans()
		return em, false, err
	}
	em.VAL.Recoil = true

	_, err = pivot.Observe(
		"WVC",
		"RECAPTURE_EVIDENCE_CLAIM",
		"PRESERVED_DIVERGENCE_EVIDENCE",
		"EVIDENCE_CONTINUITY",
		func() error {
			return wvcEvidenceContinuity(r.Store, em)
		},
	)
	if err != nil {
		_, _ = r.Store.PruneOrphans()
		return em, false, err
	}
	em.VAL.WVC = true
	em.STA = core.StateAtGOV

	if _, err := r.Store.SaveCandidate(em); err != nil {
		_, _ = r.Store.PruneOrphans()
		return em, false, err
	}

	return em, false, &RecaptureError{
		Cause:    cause,
		EmergION: em,
	}
}

func (r Runtime) Capture(ctx context.Context, path string, removeOnSuccess bool) (core.EmergION, bool, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return core.EmergION{}, false, err
	}

	em, duplicate, err := r.captureBytes(
		ctx,
		filepath.Base(path),
		b,
		"local_dropzone",
	)
	if err != nil {
		return em, duplicate, err
	}

	if removeOnSuccess {
		if err := os.Remove(path); err != nil {
			return em, duplicate, fmt.Errorf(
				"captured but could not clear dropzone: %w",
				err,
			)
		}
	}

	return em, duplicate, nil
}

func (r Runtime) CaptureBytes(
	ctx context.Context,
	name string,
	b []byte,
	provenance string,
) (core.EmergION, bool, error) {
	return r.captureBytes(ctx, name, b, provenance)
}

func (r Runtime) captureBytes(
	ctx context.Context,
	name string,
	b []byte,
	provenance string,
) (core.EmergION, bool, error) {
	if r.Store == nil || r.Reasoner == nil {
		return core.EmergION{}, false, fmt.Errorf("runtime not configured")
	}
	if len(b) == 0 {
		return core.EmergION{}, false, fmt.Errorf("empty source")
	}

	h := store.Hash(b)
	if existing, ok, err := r.Store.FindBySourceHash(h); err != nil {
		return core.EmergION{}, false, err
	} else if ok {
		return existing, true, nil
	}

	boundary, governedState, err := r.governedStateContext()
	if err != nil {
		return core.EmergION{}, false, fmt.Errorf(
			"living state projection failed: %w",
			err,
		)
	}

	analysis, err := r.Reasoner.Analyze(ctx, reason.Input{
		Name:          name,
		Content:       b,
		GovernedState: governedState,
	})
	if err != nil {
		return core.EmergION{}, false, err
	}

	analysis = reason.Calibrate(analysis)
	fieldDelta := deriveFieldDelta(boundary.Accepted, analysis)
	if err := r.validateLineage(&analysis); err != nil {
		return core.EmergION{}, false, err
	}

	ev, err := r.Store.Preserve(b)
	if err != nil {
		return core.EmergION{}, false, err
	}

	fr := fixedReasoner{
		name:    r.Reasoner.Name(),
		version: r.Reasoner.Version(ctx),
		result:  analysis,
	}

	em, err := (emerger.Engine{Reasoner: fr}).Emerge(
		ctx,
		reason.Input{Name: name, Content: b},
		emerger.Evidence{
			Hash:       ev.Hash,
			Bytes:      ev.Bytes,
			Stored:     ev.Stored,
			Codec:      ev.Codec,
			Provenance: provenance,
		},
	)
	if err != nil {
		var divergence *pivot.DivergenceError
		if errors.As(err, &divergence) {
			_, _ = r.Store.PruneOrphans()
			return r.recapture(ctx, governedState, fieldDelta, err)
		}

		_, _ = r.Store.PruneOrphans()
		return core.EmergION{}, false, err
	}

	if target := strings.TrimSpace(em.REL["COMPOSITION_KIN"]); target != "" && target == em.IDN {
		_, _ = r.Store.PruneOrphans()
		return core.EmergION{}, false, fmt.Errorf(
			"composition rejected: COMPOSITION_KIN cannot self-reference %s",
			em.IDN,
		)
	}

	if em.EVO.Metadata == nil {
		return core.EmergION{}, false, fmt.Errorf("emergion metadata missing")
	}
	em.EVO.Metadata.FieldObservation = append(
		[]string(nil),
		fieldDelta...,
	)
	if em.REL == nil {
		em.REL = map[string]string{}
	}
	if strings.TrimSpace(governedState) != "" {
		em.REL["governed_state"] = "accepted_context_present"
		em.VAL.Facts = append(em.VAL.Facts, "living_state_projected")
	}

	if coverageErr := coverage(&em, governedState); coverageErr != nil {
		_, pivotErr := pivot.Observe(
			"COVERAGE",
			"CANDIDATE_COVERAGE_CLAIM",
			"BRIDGEGAP_OBSERVATION",
			"NO_UNRESOLVED_BRIDGEGAP",
			func() error { return coverageErr },
		)

		_, _ = r.Store.PruneOrphans()
		if pivotErr != nil {
			return r.recapture(ctx, governedState, fieldDelta, pivotErr)
		}
		return em, false, coverageErr
	}

	r.resolveRequiredCapability(&em, boundary)

	delete(em.REL, "capability_provider_edge_proposal")

	if providers := strings.TrimSpace(em.REL["capability_providers"]); providers != "" {
		edges, edgeErr := capabilityProviderEdges(providers)
		if edgeErr != nil {
			_, _ = r.Store.PruneOrphans()
			return core.EmergION{}, false, edgeErr
		}

		proposal := make([]string, 0, len(edges))
		for _, edge := range edges {
			proposal = append(proposal, edge.From+"->"+edge.To)
		}

		if len(proposal) > 0 {
			em.REL["capability_provider_edge_proposal"] = strings.Join(proposal, ",")
		}
	}

	if err := protector(&em); err != nil {
		_, _ = r.Store.PruneOrphans()
		return r.recapture(ctx, governedState, fieldDelta, err)
	}

	_, err = pivot.Observe(
		"RECOIL",
		"CANDIDATE_CLAIM",
		"CANDIDATE_OBSERVATION",
		"POST_PROTECTOR_CANDIDATE_INTEGRITY",
		func() error { return recoilIntegrity(em) },
	)
	if err != nil {
		return r.recapture(ctx, governedState, fieldDelta, err)
	}
	em.VAL.Recoil = true

	_, err = pivot.Observe(
		"WVC",
		"CANDIDATE_EVIDENCE_CLAIM",
		"PRESERVED_EVIDENCE_OBSERVATION",
		"EVIDENCE_CONTINUITY",
		func() error { return wvcEvidenceContinuity(r.Store, em) },
	)
	if err != nil {
		return r.recapture(ctx, governedState, fieldDelta, err)
	}

	em.VAL.WVC = true
	em.STA = core.StateAtGOV

	if _, err = r.Store.SaveCandidate(em); err != nil {
		_, _ = r.Store.PruneOrphans()
		return core.EmergION{}, false, err
	}

	return em, false, nil
}

type fixedReasoner struct {
	name, version string
	result        reason.Result
}

func (f fixedReasoner) Analyze(context.Context, reason.Input) (reason.Result, error) {
	return f.result, nil
}
func (f fixedReasoner) Name() string                   { return f.name }
func (f fixedReasoner) Version(context.Context) string { return f.version }

func executionAlreadyObserved(
	st core.State,
	parentEmergION string,
	adapter string,
	action string,
) bool {
	groups := []map[string]core.EmergION{
		st.AtGOV,
		st.Approved,
		st.Accepted,
		st.Held,
		st.Rejected,
		st.Returned,
	}

	for _, group := range groups {
		for _, em := range group {
			if em.REL["source_kind"] != "EXECUTION_RESULT" {
				continue
			}
			if em.REL["parent_emergion"] != parentEmergION {
				continue
			}
			if em.REL["adapter"] != adapter {
				continue
			}
			if em.REL["action"] != action {
				continue
			}
			return true
		}
	}

	return false
}

func (r Runtime) AuthorizeAction(
	emergionID string,
	adapter string,
	action string,
	reasonText string,
	localGemma bool,
) (string, error) {
	if r.Store == nil {
		return "", fmt.Errorf("runtime store not configured")
	}

	events, err := r.Store.Events()
	if err != nil {
		return "", err
	}

	st, err := livefield.Rebuild(events)
	if err != nil {
		return "", err
	}

	em, ok := st.Accepted[emergionID]
	if !ok {
		return "", fmt.Errorf(
			"EmergION %q is not REG-accepted",
			emergionID,
		)
	}

	var facets []string
	if em.EVO.Metadata != nil {
		for _, facet := range em.EVO.Metadata.Facets {
			facets = append(facets, string(facet))
		}
	}

	derivable := false
	for _, candidate := range adapters.DeriveActionCandidates(
		facets,
		em.CAP,
		localGemma,
	) {
		if candidate.Adapter == adapter &&
			candidate.Action == action {
			derivable = true
			break
		}
	}

	if !derivable {
		return "", fmt.Errorf(
			"action %s:%s is not derivable from accepted EmergION %s",
			adapter,
			action,
			em.IDN,
		)
	}

	receipt := core.ActionAuthorizationReceipt{
		EmergIONID: em.IDN,
		Adapter:    adapter,
		Action:     action,
		Authority:  "HUMAN_FINAL",
		Authorized: true,
		Reason:     reasonText,
		At:         time.Now().UTC(),
	}

	return r.Store.SaveActionAuthorization(receipt)
}

func (r Runtime) ExecuteAction(
	ctx context.Context,
	emergionID string,
	adapter string,
	action string,
	gemma reason.GemmaCLI,
) (adapters.ExecutionRequest, adapters.ExecutionResult, core.EmergION, bool, error) {
	if r.Store == nil {
		return adapters.ExecutionRequest{}, adapters.ExecutionResult{}, core.EmergION{}, false, fmt.Errorf("runtime store not configured")
	}

	events, err := r.Store.Events()
	if err != nil {
		return adapters.ExecutionRequest{}, adapters.ExecutionResult{}, core.EmergION{}, false, err
	}

	st, err := livefield.Rebuild(events)
	if err != nil {
		return adapters.ExecutionRequest{}, adapters.ExecutionResult{}, core.EmergION{}, false, err
	}

	request, err := adapters.PrepareExecution(
		st,
		emergionID,
		adapter,
		action,
		gemma.Validate() == nil,
	)
	if err != nil {
		return adapters.ExecutionRequest{}, adapters.ExecutionResult{}, core.EmergION{}, false, err
	}

	var result adapters.ExecutionResult
	var execErr error

	switch request.Adapter {
	case "LOCAL_GEMMA":
		executor := adapters.LocalGemmaExecutor{
			Store: r.Store,
			Gemma: gemma,
		}
		result, execErr = executor.Execute(request)
	default:
		return request, adapters.ExecutionResult{}, core.EmergION{}, false, fmt.Errorf(
			"no local executor connected for adapter %s",
			request.Adapter,
		)
	}

	if execErr != nil && result.Error == "" {
		result.Error = execErr.Error()
	}

	result = adapters.BindExecutionResult(request, result)

	signal, duplicate, err := r.CaptureGovernedExecutionResult(
		ctx,
		request,
		result,
	)
	if err != nil {
		return request, result, core.EmergION{}, false, err
	}

	if execErr != nil {
		return request, result, signal, duplicate, execErr
	}

	return request, result, signal, duplicate, nil
}

func (r Runtime) ExecuteOneSafeAction(
	ctx context.Context,
	gemma reason.GemmaCLI,
) (core.EmergION, bool, error) {
	if r.Store == nil {
		return core.EmergION{}, false, fmt.Errorf("runtime store not configured")
	}

	if err := gemma.Validate(); err != nil {
		return core.EmergION{}, false, err
	}

	events, err := r.Store.Events()
	if err != nil {
		return core.EmergION{}, false, err
	}

	st, err := livefield.Rebuild(events)
	if err != nil {
		return core.EmergION{}, false, err
	}

	ids := make([]string, 0, len(st.Accepted))
	for id := range st.Accepted {
		ids = append(ids, id)
	}
	sort.Strings(ids)

	for _, id := range ids {
		em := st.Accepted[id]

		var facets []string
		if em.EVO.Metadata != nil {
			for _, facet := range em.EVO.Metadata.Facets {
				facets = append(facets, string(facet))
			}
		}

		for _, candidate := range adapters.DeriveActionCandidates(
			facets,
			em.CAP,
			true,
		) {
			if !candidate.Enabled ||
				candidate.Authority != "CAP_ONLY" ||
				candidate.HumanFinalRequired ||
				candidate.Adapter != "LOCAL_GEMMA" ||
				candidate.Action != "ANALYZE" {
				continue
			}

			if executionAlreadyObserved(
				st,
				em.IDN,
				candidate.Adapter,
				candidate.Action,
			) {
				continue
			}

			request, err := adapters.PrepareExecution(
				st,
				em.IDN,
				candidate.Adapter,
				candidate.Action,
				true,
			)
			if err != nil {
				return core.EmergION{}, false, err
			}

			executor := adapters.LocalGemmaExecutor{
				Store: r.Store,
				Gemma: gemma,
			}

			result, execErr := executor.Execute(request)
			if execErr != nil && result.Error == "" {
				result.Error = execErr.Error()
			}

			result = adapters.BindExecutionResult(request, result)

			signal, duplicate, err :=
				r.CaptureGovernedExecutionResult(
					ctx,
					request,
					result,
				)
			if err != nil {
				return core.EmergION{}, false, err
			}

			if execErr != nil {
				return signal, !duplicate, execErr
			}

			return signal, !duplicate, nil
		}
	}

	return core.EmergION{}, false, nil
}

func (r Runtime) Once(ctx context.Context, dropzone string) ([]string, error) {
	if err := os.MkdirAll(dropzone, 0o700); err != nil {
		return nil, err
	}
	ents, err := os.ReadDir(dropzone)
	if err != nil {
		return nil, err
	}
	sort.Slice(ents, func(i, j int) bool { return ents[i].Name() < ents[j].Name() })
	var ids []string
	for _, ent := range ents {
		if ent.IsDir() || strings.HasPrefix(ent.Name(), ".") {
			continue
		}
		path := filepath.Join(dropzone, ent.Name())
		em, _, err := r.Capture(ctx, path, true)
		if err != nil {
			var recaptured *RecaptureError
			if !errors.As(err, &recaptured) {
				return ids, fmt.Errorf("%s: %w", ent.Name(), err)
			}

			if recaptured.EmergION.STA != core.StateAtGOV ||
				!recaptured.EmergION.VAL.Recoil ||
				!recaptured.EmergION.VAL.WVC {
				return ids, fmt.Errorf("%s: RECAPTURE not GOV-ready", ent.Name())
			}

			if removeErr := os.Remove(path); removeErr != nil && !errors.Is(removeErr, os.ErrNotExist) {
				return ids, fmt.Errorf(
					"%s: RECAPTURE succeeded but source could not clear dropzone: %w",
					ent.Name(),
					removeErr,
				)
			}

			em = recaptured.EmergION
		}
		ids = append(ids, em.IDN)
	}
	return ids, nil
}

func (r Runtime) Run(
	ctx context.Context,
	dropzone string,
	interval time.Duration,
	onCapture func(string),
	onCycle func(context.Context) error,
) error {
	if interval < 250*time.Millisecond {
		interval = 250 * time.Millisecond
	}
	ids, err := r.Once(ctx, dropzone)
	if err != nil {
		return err
	}
	if onCapture != nil {
		for _, id := range ids {
			onCapture(id)
		}
	}
	if onCycle != nil {
		if err := onCycle(ctx); err != nil {
			return err
		}
	}
	t := time.NewTicker(interval)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return nil
		case <-t.C:
			ids, err := r.Once(ctx, dropzone)
			if err != nil {
				return err
			}
			if onCapture != nil {
				for _, id := range ids {
					onCapture(id)
				}
			}
			if onCycle != nil {
				if err := onCycle(ctx); err != nil {
					return err
				}
			}
		}
	}
}
