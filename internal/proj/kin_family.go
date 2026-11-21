package proj

import (
	"sort"
	"strings"

	"emergion-sovereign-runtime/internal/core"
)

// kinFamilies groups REG-accepted records into kin families for display.
// It starts from each record's engine kin root (Supersedes lineage, from
// AcceptedKinRoot; a record whose root errs stands alone) and joins two
// families when a REG-accepted SOURCE_KIN_LINK names an accepted record at
// each end. The accepted link record joins the family it links. Links that
// are AtGOV, Held, Rejected or Returned, or that name an end that is not
// REG-accepted, join nothing. The family ID is the smallest engine kin root
// in the family. Derived, read-only, rebuilt on every call; never authority.
func kinFamilies(st core.State, roots map[string]string) map[string]string {
	parent := map[string]string{}
	var find func(string) string
	find = func(x string) string {
		p, ok := parent[x]
		if !ok {
			parent[x] = x
			return x
		}
		if p == x {
			return x
		}
		r := find(p)
		parent[x] = r
		return r
	}
	union := func(a, b string) {
		ra, rb := find(a), find(b)
		if ra == rb {
			return
		}
		if rb < ra {
			ra, rb = rb, ra
		}
		parent[rb] = ra
	}

	base := func(id string) string {
		if r := roots[id]; r != "" {
			return r
		}
		return id
	}

	ids := make([]string, 0, len(st.Accepted))
	for id := range st.Accepted {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	for _, id := range ids {
		find(base(id))
	}
	for _, id := range ids {
		em := st.Accepted[id]
		if strings.TrimSpace(em.REL["source_kind"]) != "SOURCE_KIN_LINK" {
			continue
		}
		from := strings.TrimSpace(em.REL["kin_link_from"])
		to := strings.TrimSpace(em.REL["kin_link_to"])
		if from == "" || to == "" || from == to {
			continue
		}
		if _, ok := st.Accepted[from]; !ok {
			continue
		}
		if _, ok := st.Accepted[to]; !ok {
			continue
		}
		union(base(from), base(to))
		union(base(from), base(id))
	}

	out := make(map[string]string, len(ids))
	for _, id := range ids {
		out[id] = find(base(id))
	}
	return out
}
