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
	Differences []string `json:"x,omitempty"`
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
}

type Topology string

const TopologyDodecahedronV1 Topology = "DODECAHEDRON_V1"

type Metadata struct {
	Topology         Topology      `json:"y,omitempty"`
	CapturedAt       time.Time     `json:"t"`
	AIIntegrated     bool          `json:"a"`
	PromptSchema     string        `json:"p,omitempty"`
	Archonym         string        `json:"h,omitempty"`
	Facets           []Facet       `json:"f,omitempty"`
	BuildNodes       []BuildNode   `json:"n,omitempty"`
	BuildEdges       []BuildEdge   `json:"e,omitempty"`
	Monetization     *Monetization `json:"o,omitempty"`
	FieldObservation []string      `json:"b,omitempty"`
}

func (m *Metadata) Validate() error {
	if m == nil {
		return nil
	}
	if m.Archonym != "" {
		archonym := strings.TrimSpace(m.Archonym)
		if archonym == "" || archonym != m.Archonym || len(archonym) > 160 {
			return fmt.Errorf("invalid Archonym %q", m.Archonym)
		}
	}
	seenFieldObservation := map[string]bool{}
	for _, observation := range m.FieldObservation {
		observation = strings.TrimSpace(observation)
		if observation == "" || seenFieldObservation[observation] {
			return fmt.Errorf("invalid or duplicate field observation %q", observation)
		}

		valid := observation == "FIELD_EMPTY" ||
			observation == "FIELD_NO_STRUCTURAL_OBSERVATION" ||
			strings.HasPrefix(observation, "FIELD_CAP_KNOWN:") ||
			strings.HasPrefix(observation, "FIELD_CAP_NOVEL:") ||
			strings.HasPrefix(observation, "FIELD_REL_NOVEL:") ||
			strings.HasPrefix(observation, "FIELD_REL_MATCH:") ||
			strings.HasPrefix(observation, "FIELD_REL_VARIANT:") ||
			strings.HasPrefix(observation, "FIELD_FACET_KNOWN:") ||
			strings.HasPrefix(observation, "FIELD_FACET_NOVEL:") ||
			observation == "F0" ||
			observation == "F_" ||
			strings.HasPrefix(observation, "FC:K:") ||
			strings.HasPrefix(observation, "FC:N:") ||
			strings.HasPrefix(observation, "FR:N:") ||
			strings.HasPrefix(observation, "FR:M:") ||
			strings.HasPrefix(observation, "FR:V:") ||
			strings.HasPrefix(observation, "FF:K:") ||
			strings.HasPrefix(observation, "FF:N:")

		if !valid {
			return fmt.Errorf("invalid field observation %q", observation)
		}

		if strings.HasSuffix(observation, ":") {
			return fmt.Errorf("invalid field observation %q", observation)
		}

		seenFieldObservation[observation] = true
	}
	if m.CapturedAt.IsZero() {
		return fmt.Errorf("metadata capture timestamp required")
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

type ActionAuthorizationReceipt struct {
	EventID    string    `json:"-"`
	EmergIONID string    `json:"i"`
	Adapter    string    `json:"p"`
	Action     string    `json:"x"`
	Authority  string    `json:"a"`
	Authorized bool      `json:"z"`
	Reason     string    `json:"r,omitempty"`
	At         time.Time `json:"t"`
}

type Event struct {
	Type                string                      `json:"t"`
	ID                  string                      `json:"i"`
	At                  time.Time                   `json:"a"`
	EmergION            *EmergION                   `json:"e,omitempty"`
	Decision            *DecisionReceipt            `json:"d,omitempty"`
	REG                 *REGReceipt                 `json:"r,omitempty"`
	ActionAuthorization *ActionAuthorizationReceipt `json:"q,omitempty"`
	PrevHash            string                      `json:"p,omitempty"`
	SelfHash            string                      `json:"h"`
}

type State struct {
	AtGOV                map[string]EmergION
	Approved             map[string]EmergION
	Accepted             map[string]EmergION
	Held                 map[string]EmergION
	Rejected             map[string]EmergION
	Returned             map[string]EmergION
	ActionAuthorizations []ActionAuthorizationReceipt
	Events               int
	TipHash              string
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

func symbolicTransition(state string) (string, string) {
	switch state {
	case StateAtGOV:
		return "HUMAN_FINAL_REVIEW", "HUMAN_FINAL_REVIEW"
	case StateApproved:
		return "REG_ACCEPTANCE", "REG_ACCEPTANCE"
	case StateAccepted:
		return "REPEAT_ON_NEW_SOURCE", "WAIT_FOR_NEW_SOURCE"
	case StateHeld:
		return "HUMAN_FINAL_REVIEW", "HUMAN_FINAL_REVIEW"
	case StateRejected:
		return "NONE", "NONE"
	case StateReturned:
		return "RETURN_TO_SOURCE", "RETURN_TO_SOURCE"
	default:
		return "UNRESOLVED_TRANSITION", "UNRESOLVED_TRANSITION"
	}
}

func (e EmergION) Symbolic() string {
	var b strings.Builder

	b.WriteString("萌現/1\n")
	fmt.Fprintf(&b, "識=%s\n", e.IDN)

	b.WriteString("中界/始\n")
	fmt.Fprintf(&b, "中心=%s\n", e.MEM.SourceHash)
	b.WriteString("法=FORWARD_CREATES_BACKWARD_IMPRINT;BACKWARD_CREATES_FORWARD_IMPRINT\n")
	b.WriteString("中界/終\n")

	b.WriteString("進行/始\n")
	fmt.Fprintf(&b, "状態=%s\n", e.STA)
	if e.MEM.Provenance != "" {
		fmt.Fprintf(&b, "源=%s\n", e.MEM.Provenance)
	}
	fmt.Fprintf(&b, "源根=%s\n", e.MEM.SourceHash)
	if len(e.VAL.Gaps) > 0 {
		fmt.Fprintf(&b, "差=%s\n", strings.Join(e.VAL.Gaps, "|"))
	}
	if e.EVO.Supersedes != "" {
		fmt.Fprintf(&b, "退印=SUPERSEDES:%s\n", e.EVO.Supersedes)
	} else {
		fmt.Fprintf(&b, "退印=RETURN_TO:%s\n", e.MEM.SourceHash)
	}
	if len(e.EVO.Delta) > 0 {
		fmt.Fprintf(&b, "変=%s\n", strings.Join(e.EVO.Delta, "|"))
	}
	b.WriteString("進行/終\n")

	b.WriteString("退行/始\n")
	fmt.Fprintf(&b, "動=RETURN_TO:%s\n", e.MEM.SourceHash)
	forward, next := symbolicTransition(e.STA)
	fmt.Fprintf(&b, "進印=%s\n", forward)
	b.WriteString("退行/終\n")

	b.WriteString("検証/始\n")
	fmt.Fprintf(&b, "RECOIL=%t\n", e.VAL.Recoil)
	fmt.Fprintf(&b, "WVC=%t\n", e.VAL.WVC)
	if e.VAL.Recoil && e.VAL.WVC {
		b.WriteString("結果=PASS\n")
	} else {
		b.WriteString("結果=CANDIDATE\n")
	}
	b.WriteString("検証/終\n")

	fmt.Fprintf(&b, "次=%s\n", next)
	b.WriteString("終/萌現\n")
	return b.String()
}
