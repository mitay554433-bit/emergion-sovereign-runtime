package adapters

import "sort"

type ActionCandidate struct {
	Adapter            string
	Action             string
	Authority          string
	Enabled            bool
	HumanFinalRequired bool
	SourceFacet        string
}

var facetActions = map[string][]string{
	"PROGRAM_FORGE": {
		"PROGRAM",
		"VERSION",
	},
	"PRODUCT_STORE": {
		"PRODUCT",
		"PRICE",
		"LINK",
		"SITE",
		"STORE",
		"DEPLOY",
	},
	"CUSTOMERS_SALES": {
		"CUSTOMER",
		"LEAD",
		"SALE",
		"SUPPORT",
	},
	"COMMUNICATIONS": {
		"READ",
		"DRAFT",
		"SEND",
	},
	"PAYMENTS_FINANCE": {
		"PRODUCT",
		"PRICE",
		"LINK",
		"RECEIPT",
		"TRANSFER",
	},
	"GRANT_FUNDING": {
		"GRANT",
	},
	"PATENT_IP": {
		"PATENT",
		"PATENT_EVIDENCE",
	},
	"MA_PARTNERSHIPS": {
		"MA",
	},
	"DOCS_PROJECTION": {
		"DRAFT",
	},
	"ANALYTICS_FORECAST": {
		"ANALYZE",
		"SIMULATE",
	},
}

func humanFinalAction(action string) bool {
	switch action {
	case "SEND", "TRANSFER", "DEPLOY", "CONTRACT", "ACQUIRE", "PROGRAM":
		return true
	default:
		return false
	}
}

func DeriveActionCandidates(
	facets []string,
	capabilities []string,
	localGemma bool,
) []ActionCandidate {
	catalog := Catalog(localGemma)

	requested := map[string]string{}

	for _, capability := range capabilities {
		requested[capability] = ""
	}

	for _, facet := range facets {
		for _, action := range facetActions[facet] {
			if _, exists := requested[action]; !exists {
				requested[action] = facet
			}
		}
	}

	var out []ActionCandidate
	seen := map[string]bool{}

	for _, adapter := range catalog {
		for _, capability := range adapter.Capabilities {
			sourceFacet, wanted := requested[capability]
			if !wanted {
				continue
			}

			key := adapter.ID + "\x00" + capability
			if seen[key] {
				continue
			}
			seen[key] = true

			out = append(out, ActionCandidate{
				Adapter:            adapter.ID,
				Action:             capability,
				Authority:          adapter.Authority,
				Enabled:            adapter.Enabled,
				HumanFinalRequired: humanFinalAction(capability),
				SourceFacet:        sourceFacet,
			})
		}
	}

	sort.Slice(out, func(i, j int) bool {
		if out[i].Adapter != out[j].Adapter {
			return out[i].Adapter < out[j].Adapter
		}
		return out[i].Action < out[j].Action
	})

	return out
}
