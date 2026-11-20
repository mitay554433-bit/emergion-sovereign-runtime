package proj

import (
	"sort"
	"strings"

	"emergion-sovereign-runtime/internal/core"
	livefield "emergion-sovereign-runtime/internal/field"
)

// LinkedProjection is the derived, read-only view that HTML already renders
// (spatial facet groups, accepted kin roots, SAAB composition assemblies with
// their CPSL programs, SAWs and realizations), exposed as JSON for the Living
// Universe. It is rebuilt from state on every call, writes nothing, and is
// never authority: COSL + REG remain authoritative.
type LinkedProjection struct {
	Authority       string                 `json:"authority"`
	TipHash         string                 `json:"tip_hash"`
	Events          int                    `json:"events"`
	FacetOrder      []string               `json:"facet_order"`
	FacetGroups     []LinkedFacetGroup     `json:"facet_groups"`
	Unassigned      []string               `json:"unassigned"`
	Records         []LinkedRecord         `json:"records"`
	Assemblies      []LinkedAssembly       `json:"assemblies"`
	AssembliesError string                 `json:"assemblies_error,omitempty"`
	SAWs            []string               `json:"saws"`
	Realizations    []LinkedRealization    `json:"realizations"`
	HumanFinalQueue []LinkedHumanFinalItem `json:"human_final_queue"`
	// Trajectories is filled by the caller that holds the events (capture
	// times); Linked(st) leaves it empty. Derived, read-only.
	Trajectories []LinkedTrajectory `json:"trajectories"`
	// ArchonymConflicts lists REG receipts that replay refused because the
	// Archonym already belongs to another sovereign EmergION (the owner keeps
	// it; the refused record stays Approved). Derived, read-only.
	ArchonymConflicts []core.ArchonymConflict `json:"archonym_conflicts"`
	// ArchonymWaiting lists AtGOV records whose proposed Archonym already
	// belongs to another sovereign EmergION. They stay visible by ID and
	// summary; a HUMAN_FINAL APPROVE of them is refused at accept time.
	ArchonymWaiting []string `json:"archonym_waiting_conflicts"`
}

type LinkedFacetGroup struct {
	Facet   string   `json:"facet"`
	Members []string `json:"members"`
}

type LinkedRecord struct {
	ID           string   `json:"id"`
	Archonym     string   `json:"archonym,omitempty"`
	Topology     string   `json:"topology,omitempty"`
	Facets       []string `json:"facets,omitempty"`
	KinRoot      string   `json:"kin_root,omitempty"`
	KinRootError string   `json:"kin_root_error,omitempty"`
	Capabilities []string `json:"capabilities,omitempty"`
	Summary      string   `json:"summary,omitempty"`
}

type LinkedLink struct {
	From string `json:"from"`
	To   string `json:"to"`
	Kind string `json:"kind"`
}

type LinkedAssembly struct {
	ID           string       `json:"id"`
	Members      []string     `json:"members"`
	Capabilities []string     `json:"capabilities,omitempty"`
	Links        []LinkedLink `json:"links"`
	CPSL         string       `json:"cpsl"`
}

type LinkedRealization struct {
	TargetThing string `json:"target_thing"`
	Selected    string `json:"selected"`
	RunnersUp   string `json:"runners_up,omitempty"`
	SourceID    string `json:"source_id"`
	SourceKind  string `json:"source_kind"`
	Status      string `json:"status"`
}

type LinkedHumanFinalItem struct {
	ID      string `json:"id"`
	Summary string `json:"summary,omitempty"`
}

// Linked derives the LinkedProjection from st using the same engine functions
// as HTML. A SAAB derivation error is reported in AssembliesError instead of
// failing the whole view.
func Linked(st core.State) LinkedProjection {
	out := LinkedProjection{
		Authority:       "COSL + REG remain authoritative · visualization only",
		TipHash:         st.TipHash,
		Events:          st.Events,
		FacetGroups:     []LinkedFacetGroup{},
		Unassigned:      []string{},
		Records:         []LinkedRecord{},
		Assemblies:      []LinkedAssembly{},
		SAWs:            []string{},
		Realizations:    []LinkedRealization{},
		HumanFinalQueue: []LinkedHumanFinalItem{},
		Trajectories:    []LinkedTrajectory{},
		ArchonymConflicts: append(
			[]core.ArchonymConflict{},
			st.ArchonymConflicts...,
		),
	}
	for _, f := range spatialFacetOrder() {
		out.FacetOrder = append(out.FacetOrder, string(f))
	}

	spatial := spatialEmergIONs(st)
	for _, g := range spatialFacetGroups(spatial) {
		members := make([]string, 0, len(g.EmergIONs))
		for _, em := range g.EmergIONs {
			members = append(members, em.ID)
		}
		out.FacetGroups = append(out.FacetGroups, LinkedFacetGroup{Facet: string(g.Facet), Members: members})
	}
	for _, em := range spatialUnassigned(spatial) {
		out.Unassigned = append(out.Unassigned, em.ID)
	}

	for _, s := range spatial {
		em := st.Accepted[s.ID]
		rec := LinkedRecord{ID: s.ID, Archonym: s.Archonym, Topology: s.Topology, Summary: em.MEM.Summary}
		for _, f := range s.Facets {
			rec.Facets = append(rec.Facets, string(f))
		}
		for _, c := range em.CAP {
			if c != "" {
				rec.Capabilities = append(rec.Capabilities, c)
			}
		}
		if root, err := livefield.AcceptedKinRoot(st.Accepted, st.Returned, s.ID); err != nil {
			rec.KinRootError = err.Error()
		} else {
			rec.KinRoot = root
		}
		out.Records = append(out.Records, rec)
	}

	assemblies, err := deriveSAABs(st)
	if err != nil {
		out.AssembliesError = err.Error()
	} else {
		programs, perr := compileCPSL(st)
		if perr != nil {
			out.AssembliesError = perr.Error()
		}
		bySAAB := map[string]string{}
		for _, p := range programs {
			bySAAB[p.SAABID] = p.Program
		}
		for _, a := range assemblies {
			la := LinkedAssembly{
				ID:           a.ID,
				Members:      append([]string{}, a.MemberPRMIDs...),
				Capabilities: append([]string(nil), a.Capabilities...),
				Links:        []LinkedLink{},
				CPSL:         bySAAB[a.ID],
			}
			for _, l := range a.CompositionLinks {
				la.Links = append(la.Links, LinkedLink{From: l.FromPRM, To: l.ToPRM, Kind: l.Kind})
			}
			out.Assemblies = append(out.Assemblies, la)
		}
		if saws, serr := extractSAWs(st); serr == nil {
			for _, s := range saws {
				out.SAWs = append(out.SAWs, s.ID)
			}
		}
	}

	for _, r := range realizationRows(st) {
		out.Realizations = append(out.Realizations, LinkedRealization(r))
	}

	out.ArchonymWaiting = []string{}
	for id, em := range st.AtGOV {
		if owner, _, err := livefield.ArchonymOwner(st, em); err != nil || owner != "" {
			out.ArchonymWaiting = append(out.ArchonymWaiting, id)
		}
	}
	sort.Strings(out.ArchonymWaiting)

	lifecycle := rows(st)
	sort.Slice(lifecycle, func(i, j int) bool { return lifecycle[i].ID < lifecycle[j].ID })
	for _, r := range humanFinalRows(lifecycle) {
		out.HumanFinalQueue = append(out.HumanFinalQueue, LinkedHumanFinalItem{ID: r.ID, Summary: strings.TrimSpace(r.Summary)})
	}
	return out
}
