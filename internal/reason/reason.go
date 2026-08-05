package reason

import "context"

type Input struct {
	Name    string
	Content []byte
}

type Result struct {
	Summary       string            `json:"summary"`
	Relationships map[string]string `json:"relationships"`
	Capabilities  []string          `json:"capabilities"`
	Facts         []string          `json:"facts"`
	Gaps          []string          `json:"gaps"`
	Risk          string            `json:"risk"`
	Facets        []string          `json:"facets"`
	BuildNodes    []BuildNode       `json:"build_nodes"`
	BuildEdges    []BuildEdge       `json:"build_edges"`
	Monetization  *Monetization     `json:"monetization"`
	LogicalPrompts []string         `json:"logical_prompts"`
}

type BuildNode struct {
	ID     string `json:"id"`
	System string `json:"system"`
	State  string `json:"state"`
}

type BuildEdge struct {
	From string `json:"from"`
	To   string `json:"to"`
	Kind string `json:"kind"`
}

type Monetization struct {
	Model       string `json:"model"`
	Customer    string `json:"customer"`
	Value       string `json:"value"`
	RevenuePath string `json:"revenue_path"`
	NextAction  string `json:"next_action"`
	Confidence  string `json:"confidence"`
}

type Reasoner interface {
	Analyze(context.Context, Input) (Result, error)
	Name() string
	Version(context.Context) string
}
