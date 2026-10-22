package runtime

import (
	"fmt"
	"sort"
)

type RealizationOrder struct {
	Fronts [][]string
}

func DeriveRealizationOrder(p RealizationPopulation) (RealizationOrder, error) {
	members := map[string]bool{}
	incoming := map[string]int{}
	outgoing := map[string]map[string]bool{}

	for _, id := range p.Members {
		members[id] = true
		incoming[id] = 0
		outgoing[id] = map[string]bool{}
	}

	addEdge := func(better, worse string) {
		if outgoing[better][worse] {
			return
		}
		outgoing[better][worse] = true
		incoming[worse]++
	}

	for _, c := range p.Comparisons {
		switch c.Result {
		case RealizationLeftPreferred:
			addEdge(c.Left, c.Right)
		case RealizationRightPreferred:
			addEdge(c.Right, c.Left)
		case RealizationEquivalent:
		case RealizationUnresolved:
		default:
			return RealizationOrder{}, fmt.Errorf(
				"unsupported realization comparison result %q",
				c.Result,
			)
		}
	}

	remaining := map[string]bool{}
	for id := range members {
		remaining[id] = true
	}

	fronts := [][]string{}

	for len(remaining) > 0 {
		front := []string{}

		for id := range remaining {
			if incoming[id] == 0 {
				front = append(front, id)
			}
		}

		if len(front) == 0 {
			return RealizationOrder{}, fmt.Errorf(
				"realization preference cycle prevents evidence-supported ordering",
			)
		}

		sort.Strings(front)
		fronts = append(fronts, front)

		for _, id := range front {
			delete(remaining, id)

			for worse := range outgoing[id] {
				incoming[worse]--
			}
		}
	}

	return RealizationOrder{Fronts: fronts}, nil
}

func (o RealizationOrder) Strongest() []string {
	if len(o.Fronts) == 0 {
		return nil
	}

	return append([]string(nil), o.Fronts[0]...)
}
