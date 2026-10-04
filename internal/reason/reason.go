package reason

import "context"

type Input struct {
	Name          string
	Content       []byte
	GovernedState string
}

type Result struct {
	Summary       string
	Archonym      string
	Relationships map[string]string
	Capabilities  []string
	Facts         []string
	Differences   []string
	Requirements  []string
	Gaps          []string
	Risk          string
	Supersedes    string
	Delta         []string
	Facets        []string
	BuildNodes    []BuildNode
	BuildEdges    []BuildEdge
	Monetization  *Monetization
}

type BuildNode struct {
	ID     string
	System string
	State  string
}

type BuildEdge struct {
	From string
	To   string
	Kind string
}

type Monetization struct {
	Model       string
	Customer    string
	Value       string
	RevenuePath string
}

type Reasoner interface {
	Analyze(context.Context, Input) (Result, error)
	Name() string
	Version(context.Context) string
}
