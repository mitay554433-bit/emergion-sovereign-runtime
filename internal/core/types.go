package core

import (
	"fmt"
	"strings"
	"time"
)

const (
	StateAtGOV    = "G"
	StateApproved = "A"
	StateHeld     = "H"
	StateRejected = "X"
	StateReturned = "B"
	StateAccepted = "F"
)

type Memory struct {
	SourceHash string `json:"h"`
	Codec      string `json:"z"`
	Bytes      int64  `json:"b"`
	Stored     int64  `json:"q"`
	Summary    string `json:"s,omitempty"`
	Provenance string `json:"p,omitempty"`
}

type Validation struct {
	Facts       []string `json:"f,omitempty"`
	Gaps        []string `json:"g,omitempty"`
	Risk        string   `json:"r,omitempty"`
	Recoil      bool     `json:"c"`
	WVC         bool     `json:"w"`
	Reasoner    string   `json:"a,omitempty"`
	ReasonerVer string   `json:"v,omitempty"`
}

type Evolution struct {
	Version    int       `json:"v"`
	Supersedes string    `json:"s,omitempty"`
	Delta      []string  `json:"d,omitempty"`
	Metadata   *Metadata `json:"m,omitempty"`
}

// Facet is one face of the bounded dodecahedral capability projection.
// The Evolution Engine governs changes to all faces and is not a thirteenth face.
type Facet string

const (
	FacetFIELDCommand      Facet = "FIELD_COMMAND"
	FacetEmergenceCapture  Facet = "EMERGENCE_CAPTURE"
	FacetProgramForge      Facet = "PROGRAM_FORGE"
	FacetProductStore      Facet = "PRODUCT_STORE"
	FacetCustomersSales    Facet = "CUSTOMERS_SALES"
	FacetCommunications    Facet = "COMMUNICATIONS"
	FacetPaymentsFinance   Facet = "PAYMENTS_FINANCE"
	FacetGrantFunding      Facet = "GRANT_FUNDING"
	FacetPatentIP          Facet = "PATENT_IP"
	FacetMAPartnerships    Facet = "MA_PARTNERSHIPS"
	FacetDocsProjection    Facet = "DOCS_PROJECTION"
	FacetAnalyticsForecast Facet = "ANALYTICS_FORECAST"
)

type BuildNode struct {
	ID     string `json:"i"`
	System string `json:"s"`
	State  string `json:"t,omitempty"`
}

type BuildEdge struct {
	From string `json:"f"`
	To   string `json:"t"`
	Kind string `json:"k,omitempty"`
}

type Monetization struct {
	Model       string `json:"m,omitempty"`
	Customer    string `json:"c,omitempty"`
	Value       string `json:"v,omitempty"`
	RevenuePath string `json:"r,omitempty"`
	NextAction  string `json:"n,omitempty"`
	Confidence  string `json:"f,omitempty"`
}

type Topology string

const TopologyDodecahedronV1 Topology = "DODECAHEDRON_V1"

type Metadata struct {
	Topology     Topology      `json:"y,omitempty"`
	CapturedAt   time.Time     `json:"t"`
	AIIntegrated bool          `json:"a"`
	PromptSchema string        `json:"p,omitempty"`
	LogicalPrompts []string    `json:"l,omitempty"`
	Facets       []Facet       `json:"f,omitempty"`
	BuildNodes   []BuildNode   `json:"n,omitempty"`
	BuildEdges   []BuildEdge   `json:"e,omitempty"`
	Monetization *Monetization `json:"o,omitempty"`
}

func (m *Metadata) Validate() error {
	if m == nil {
		return nil
	}
	if m.CapturedAt.IsZero() {
		return fmt.Errorf("metadata capture timestamp required")
	}
	if len(m.LogicalPrompts) > 6 {
		return fmt.Errorf("logical prompts exceed bounds")
	}
	allowed := map[Facet]bool{
		FacetFIELDCommand: true, FacetEmergenceCapture: true, FacetProgramForge: true,
		FacetProductStore: true, FacetCustomersSales: true, FacetCommunications: true,
		FacetPaymentsFinance: true, FacetGrantFunding: true, FacetPatentIP: true,
		FacetMAPartnerships: true, FacetDocsProjection: true, FacetAnalyticsForecast: true,
	}
	seenFacets := map[Facet]bool{}
	for _, facet := range m.Facets {
		if !allowed[facet] || seenFacets[facet] {
			return fmt.Errorf("invalid or duplicate facet %q", facet)
		}
		seenFacets[facet] = true
	}
	if len(m.BuildNodes) > 24 || len(m.BuildEdges) > 48 {
		return fmt.Errorf("build graph exceeds bounds")
	}
	nodes := map[string]bool{}
	for _, node := range m.BuildNodes {
		if strings.TrimSpace(node.ID) == "" || strings.TrimSpace(node.System) == "" || nodes[node.ID] {
			return fmt.Errorf("invalid or duplicate build node %q", node.ID)
		}
		nodes[node.ID] = true
	}
	for _, edge := range m.BuildEdges {
		if !nodes[edge.From] || !nodes[edge.To] {
			return fmt.Errorf("build edge references unknown node %q -> %q", edge.From, edge.To)
		}
	}
	return nil
}

type EmergION struct {
	IDN string            `json:"i"`
	STA string            `json:"s"`
	MEM Memory            `json:"m"`
	REL map[string]string `json:"r,omitempty"`
	CAP []string          `json:"c,omitempty"`
	VAL Validation        `json:"v"`
	EVO Evolution         `json:"e"`
}

type DecisionReceipt struct {
	EmergIONID string    `json:"i"`
	Decision   string    `json:"d"`
	Authority  string    `json:"a"`
	Reason     string    `json:"r,omitempty"`
	At         time.Time `json:"t"`
}

type REGReceipt struct {
	EmergIONID string    `json:"i"`
	DecisionID string    `json:"d"`
	At         time.Time `json:"t"`
}

type Event struct {
	Type     string           `json:"t"`
	ID       string           `json:"i"`
	At       time.Time        `json:"a"`
	EmergION *EmergION        `json:"e,omitempty"`
	Decision *DecisionReceipt `json:"d,omitempty"`
	REG      *REGReceipt      `json:"r,omitempty"`
	PrevHash string           `json:"p,omitempty"`
	SelfHash string           `json:"h"`
}

type State struct {
	AtGOV    map[string]EmergION
	Approved map[string]EmergION
	Accepted map[string]EmergION
	Held     map[string]EmergION
	Rejected map[string]EmergION
	Returned map[string]EmergION
	Events   int
	TipHash  string
}

func EmptyState() State {
	return State{
		AtGOV:    map[string]EmergION{},
		Approved: map[string]EmergION{},
		Accepted: map[string]EmergION{},
		Held:     map[string]EmergION{},
		Rejected: map[string]EmergION{},
		Returned: map[string]EmergION{},
	}
}
