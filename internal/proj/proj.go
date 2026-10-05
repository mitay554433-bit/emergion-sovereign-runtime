package proj

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"html/template"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"emergion-sovereign-runtime/internal/core"
	livefield "emergion-sovereign-runtime/internal/field"
)

func JSON(path string, st core.State) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	b, err := json.MarshalIndent(st, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, b, 0o644)
}

type row struct {
	ID            string
	State         string
	Summary       string
	Risk          string
	Proof         string
	Governance    string
	Relationships string
}

func rows(st core.State) []row {
	var out []row
	add := func(m map[string]core.EmergION) {
		for _, e := range m {
			proof := "source=" + e.MEM.SourceHash
			if e.VAL.Recoil && e.VAL.WVC {
				proof += "; recoil+wvc=PASS"
			}
			governance := "awaiting HUMAN_FINAL"
			switch e.STA {
			case core.StateAccepted:
				governance = "HUMAN_FINAL approved; REG accepted"
			case core.StateApproved:
				governance = "HUMAN_FINAL approved"
			case core.StateHeld:
				governance = "HUMAN_FINAL held"
			case core.StateRejected:
				governance = "HUMAN_FINAL rejected"
			case core.StateReturned:
				governance = "HUMAN_FINAL returned for rework"
			}
			keys := make([]string, 0, len(e.REL))
			for key := range e.REL {
				keys = append(keys, key)
			}
			sort.Strings(keys)

			relationships := make([]string, 0, len(keys))
			for _, key := range keys {
				relationships = append(
					relationships,
					key+" → "+e.REL[key],
				)
			}

			out = append(out, row{
				ID:            e.IDN,
				State:         e.STA,
				Summary:       e.MEM.Summary,
				Risk:          e.VAL.Risk,
				Proof:         proof,
				Governance:    governance,
				Relationships: strings.Join(relationships, "; "),
			})
		}
	}

	add(st.AtGOV)
	add(st.Approved)
	add(st.Accepted)
	add(st.Held)
	add(st.Rejected)
	add(st.Returned)

	sort.Slice(out, func(i, j int) bool {
		return out[i].ID < out[j].ID
	})

	return out
}

type convergenceRow struct {
	ID            string
	Kin           string
	Archonym      string
	Summary       string
	Topology      string
	Facets        string
	Capabilities  string
	Relationships string
	BuildNodes    string
	BuildEdges    string
}

type spatialEmergION struct {
	ID       string
	Archonym string
	Topology string
	Facets   []core.Facet
}

func spatialEmergIONs(st core.State) []spatialEmergION {
	out := make([]spatialEmergION, 0, len(st.Accepted))

	for _, em := range st.Accepted {
		item := spatialEmergION{
			ID: em.IDN,
		}

		if em.EVO.Metadata != nil {
			item.Archonym = em.EVO.Metadata.Archonym
			item.Topology = string(em.EVO.Metadata.Topology)
			item.Facets = append(
				[]core.Facet(nil),
				em.EVO.Metadata.Facets...,
			)
		}

		out = append(out, item)
	}

	sort.Slice(out, func(i, j int) bool {
		return out[i].ID < out[j].ID
	})

	return out
}

func spatialFacetOrder() []core.Facet {
	return []core.Facet{
		core.FacetFIELDCommand,
		core.FacetEmergenceCapture,
		core.FacetProgramForge,
		core.FacetProductStore,
		core.FacetCustomersSales,
		core.FacetCommunications,
		core.FacetPaymentsFinance,
		core.FacetGrantFunding,
		core.FacetPatentIP,
		core.FacetMAPartnerships,
		core.FacetDocsProjection,
		core.FacetAnalyticsForecast,
	}
}

type prm struct {
	SourceEmergIONID string
	Relationships    map[string]string
	BuildNodes       []core.BuildNode
	BuildEdges       []core.BuildEdge
}

func crystallizePRMs(st core.State) ([]prm, error) {
	out := make([]prm, 0, len(st.Accepted))

	for _, em := range st.Accepted {

		item := prm{
			SourceEmergIONID: em.IDN,
			Relationships:    map[string]string{},
		}

		if target := strings.TrimSpace(em.REL["COMPOSITION_KIN"]); target != "" {
			item.Relationships["COMPOSITION_KIN"] = target
		}

		if em.EVO.Metadata != nil {
			item.BuildNodes = append([]core.BuildNode(nil), em.EVO.Metadata.BuildNodes...)
			item.BuildEdges = append([]core.BuildEdge(nil), em.EVO.Metadata.BuildEdges...)
		}

		out = append(out, item)
	}

	sort.Slice(out, func(i, j int) bool {
		return out[i].SourceEmergIONID < out[j].SourceEmergIONID
	})

	return out, nil
}

func convergenceRows(st core.State) ([]convergenceRow, error) {
	out := make([]convergenceRow, 0, len(st.Accepted))

	for _, e := range st.Accepted {
		row := convergenceRow{
			ID:           e.IDN,
			Summary:      e.MEM.Summary,
			Capabilities: strings.Join(e.CAP, ", "),
		}

		kin := []string{}
		root, err := livefield.AcceptedKinRoot(st.Accepted, st.Returned, e.IDN)
		if err != nil {
			return nil, err
		}
		kin = append(kin, "root → "+root)
		row.Kin = strings.Join(kin, "; ")

		if e.EVO.Metadata != nil {
			row.Archonym = e.EVO.Metadata.Archonym
			row.Topology = string(e.EVO.Metadata.Topology)

			facets := make([]string, 0, len(e.EVO.Metadata.Facets))
			for _, facet := range e.EVO.Metadata.Facets {
				facets = append(facets, string(facet))
			}
			row.Facets = strings.Join(facets, ", ")

			nodes := make([]string, 0, len(e.EVO.Metadata.BuildNodes))
			for _, node := range e.EVO.Metadata.BuildNodes {
				value := node.ID + " → " + node.System
				if node.State != "" {
					value += " [" + node.State + "]"
				}
				nodes = append(nodes, value)
			}
			row.BuildNodes = strings.Join(nodes, "; ")

			edges := make([]string, 0, len(e.EVO.Metadata.BuildEdges))
			for _, edge := range e.EVO.Metadata.BuildEdges {
				value := edge.From + " → " + edge.To
				if edge.Kind != "" {
					value += " [" + edge.Kind + "]"
				}
				edges = append(edges, value)
			}
			row.BuildEdges = strings.Join(edges, "; ")
		}

		keys := make([]string, 0, len(e.REL))
		for key := range e.REL {
			keys = append(keys, key)
		}
		sort.Strings(keys)

		relationships := make([]string, 0, len(keys))
		for _, key := range keys {
			relationships = append(
				relationships,
				key+" → "+e.REL[key],
			)
		}
		row.Relationships = strings.Join(relationships, "; ")

		out = append(out, row)
	}

	sort.Slice(out, func(i, j int) bool {
		return out[i].ID < out[j].ID
	})

	return out, nil
}

func HTML(path string, st core.State) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}

	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()

	t := template.Must(template.New("f").Parse(`<!doctype html>
<html>
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width">
<title>EmergION Projection</title>
<style>
body{font:15px system-ui;background:#020817;color:#e6eaf0;margin:0;padding:2rem}a{color:#c9a227}.ok,.good{color:#2ecc71}.bad,.err{color:#e74c3c}.muted{color:#8b95a8}.panel{background:#0f1c2e;border:1px solid #2a3548;border-radius:12px;padding:1rem;margin:0.75rem 0}h1,h2{color:#e6eaf0;border-bottom:1px solid #2d6cff;padding-bottom:0.35rem}code,pre{background:#0a1628;color:#e6eaf0;border:1px solid #2a3548}
h1{font-size:1.5rem}
h2{margin-top:2rem;font-size:1.15rem}
table{width:max-content;min-width:100%;border-collapse:collapse}
th,td{text-align:left;vertical-align:top;padding:.7rem;border-bottom:1px solid #2b3140;overflow-wrap:anywhere;word-break:break-word}
code{color:#a78bfa;overflow-wrap:anywhere;word-break:break-all}
.G{color:#f59e0b}
.F{color:#22c55e}
.X{color:#ef4444}
.small{color:#9ca3af;font-size:.9rem}
@media (max-width:700px){
body{padding:1rem}
th,td{padding:.5rem;font-size:.9rem}
}
</style>
</head>
<body>

<h1>Living Projection</h1>
<p>Events: {{.Events}} · Tip: <code>{{.TipHash}}</code></p>
<p class="small">Projection is derived state only. COSL + governance remain authoritative.</p>

<h2>Lifecycle Projection</h2>
<table>
<thead>
<tr>
<th>EmergION</th>
<th>State</th>
<th>Risk</th>
<th>Evidence proof</th>
<th>Governance</th>
<th>Relationships</th>
<th>Meaning</th>
<th>HUMAN_FINAL</th>
</tr>
</thead>
<tbody>
{{range .Rows}}
<tr>
<td><code>{{.ID}}</code></td>
<td class="{{.State}}">{{.State}}</td>
<td>{{.Risk}}</td>
<td><code>{{.Proof}}</code></td>
<td>{{.Governance}}</td>
<td>{{.Relationships}}</td>
<td>{{.Summary}}</td>
{{if eq .State "G"}}
<td>
<button type="button" onclick="humanFinal('{{.ID}}','APPROVE')">APPROVE</button>
<button type="button" onclick="humanFinal('{{.ID}}','REJECT')">REJECT</button>
</td>
{{else}}
<td></td>
{{end}}
</tr>
{{end}}
</tbody>
</table>

<h2>SPATIAL CONVERGENCE ZONE</h2>
<p class="small">Accepted governed structures only.</p>

<table>
<thead>
<tr>
<th>EmergION</th>
<th>Meaning</th>
<th>Archonym</th>
<th>Kin</th>
<th>Topology</th>
<th>Facets</th>
<th>Capabilities</th>
<th>Relationships</th>
<th>Build Nodes</th>
<th>Build Edges</th>
</tr>
</thead>
<tbody>
{{range .Convergence}}
<tr>
<td><code>{{.ID}}</code></td>
<td>{{.Summary}}</td>
<td>{{.Archonym}}</td>
<td>{{.Kin}}</td>
<td>{{.Topology}}</td>
<td>{{.Facets}}</td>
<td>{{.Capabilities}}</td>
<td>{{.Relationships}}</td>
<td>{{.BuildNodes}}</td>
<td>{{.BuildEdges}}</td>
</tr>
{{end}}
</tbody>
</table>

<h2>SPATIAL EMERGION PROJECTION</h2>
<p class="small">Accepted EmergION structures projected from governed state.</p>

<table>
<thead>
<tr>
<th>EmergION</th>
<th>Archonym</th>
<th>Topology</th>
<th>Facets</th>
</tr>
</thead>
<tbody>
{{range .Spatial}}
<tr>
<td><code>{{.ID}}</code></td>
<td>{{.Archonym}}</td>
<td>{{.Topology}}</td>
<td>{{range .Facets}}{{.}} {{end}}</td>
</tr>
{{end}}
</tbody>
</table>

<h2>SAW WORK PROJECTION</h2>
<p class="small">Deterministic SAW artifacts derived from accepted governed composition.</p>

<table>
<thead>
<tr>
<th>SAW</th>
<th>SAAB</th>
<th>Member PRMs</th>
<th>Capabilities</th>
<th>Commercial</th>
<th>CPSL</th>
</tr>
</thead>
<tbody>
{{range .SAWs}}
<tr>
<td><code>{{.ID}}</code></td>
<td><code>{{.SAABID}}</code></td>
<td>{{.MemberPRMIDs}}</td>
<td>{{.Capabilities}}</td>
<td>{{.Commercial}}</td>
<td><pre>{{.CPSL}}</pre></td>
</tr>
{{end}}
</tbody>
</table>

<h2>VERIFIED DELIVERIES</h2>
<p class="small">Operator projection only. Entries are published from REG-accepted successful execution results; canonical authority remains COSL + REG.</p>
<div id="deliveries" class="panel" data-field-tip="{{.TipHash}}"><span class="muted">Loading verified deliveries…</span></div>

<script>
function deliveryText(value) {
  return value == null ? "" : String(value);
}

async function loadDeliveries() {
  const target = document.getElementById("deliveries");
  try {
    const response = await fetch("deliverables/index.json", {cache: "no-store"});
    if (!response.ok) throw new Error("delivery index unavailable");
    const index = await response.json();
    const expectedTip = deliveryText(target.dataset.fieldTip);
    if (deliveryText(index.field_tip) !== expectedTip) {
      throw new Error("delivery projection FIELD tip mismatch");
    }
    const items = Array.isArray(index.deliverables) ? index.deliverables : [];
    if (items.length === 0) {
      target.innerHTML = '<span class="muted">No verified deliverables published.</span>';
      return;
    }

    const table = document.createElement("table");
    const head = document.createElement("thead");
    const headRow = document.createElement("tr");
    ["EmergION", "Action", "Adapter", "Evidence", "Output", "Artifact"].forEach(function(label) {
      const th = document.createElement("th");
      th.textContent = label;
      headRow.appendChild(th);
    });
    head.appendChild(headRow);
    table.appendChild(head);

    const body = document.createElement("tbody");
    items.forEach(function(item) {
      const row = document.createElement("tr");
      [
        deliveryText(item.emergion_id),
        deliveryText(item.action),
        deliveryText(item.adapter),
        deliveryText(item.evidence_sha256),
        deliveryText(item.output_sha256)
      ].forEach(function(value) {
        const td = document.createElement("td");
        const code = document.createElement("code");
        code.textContent = value;
        td.appendChild(code);
        row.appendChild(td);
      });

      const artifactCell = document.createElement("td");
      const link = document.createElement("a");
      link.href = "deliverables/" + encodeURIComponent(deliveryText(item.emergion_id)) + "/output";
      link.textContent = "open";
      artifactCell.appendChild(link);
      row.appendChild(artifactCell);
      body.appendChild(row);
    });
    table.appendChild(body);

    target.replaceChildren(table);
  } catch (err) {
    target.innerHTML = '<span class="muted">Delivery projection unavailable.</span>';
  }
}

loadDeliveries();

async function humanFinal(id, decision) {
  if (!confirm(decision + " " + id + "?")) return;
  const response = await fetch("/decide", {
    method: "POST",
    headers: {"Content-Type": "application/json"},
    body: JSON.stringify({
      emergion_id: id,
      decision: decision,
      reason: "HUMAN_FINAL PIVOTAI decision"
    })
  });
  const result = await response.json();
  if (result.error) {
    alert(result.error);
    return;
  }
  location.reload();
}
</script>
</body>
</html>`))

	convergence, err := convergenceRows(st)
	if err != nil {
		return err
	}

	saws, err := extractSAWs(st)
	if err != nil {
		return err
	}

	spatial := spatialEmergIONs(st)
	facetOrder := spatialFacetOrder()

	return t.Execute(f, map[string]any{
		"Events":      st.Events,
		"TipHash":     st.TipHash,
		"Rows":        rows(st),
		"Convergence": convergence,
		"SAWs":        saws,
		"Spatial":     spatial,
		"FacetOrder":  facetOrder,
	})
}

func EnsureOutput(root string) (string, error) {
	if root == "" {
		return "", fmt.Errorf("output path required")
	}
	if err := os.MkdirAll(root, 0o755); err != nil {
		return "", err
	}
	return root, nil
}

// Receipt is the commit marker for one current projection. Consumers can use
// the hashes to reject a mixed or stale field.json/field.html pair.
type Receipt struct {
	TipHash   string    `json:"tip_hash"`
	Events    int       `json:"events"`
	JSONHash  string    `json:"field_json_sha256"`
	HTMLHash  string    `json:"field_html_sha256"`
	Generated time.Time `json:"generated_at"`
}

func fileHash(path string) (string, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("%x", sha256.Sum256(b)), nil
}

func replaceFile(tmp, final string) error {
	if err := os.Rename(tmp, final); err != nil {
		return fmt.Errorf("publish %s: %w", filepath.Base(final), err)
	}
	return nil
}

// Current publishes both compatibility projections and writes the receipt
// last. The receipt is the atomic freshness boundary for operator clients.
func Current(root string, st core.State) (Receipt, error) {
	if _, err := EnsureOutput(root); err != nil {
		return Receipt{}, err
	}
	tmp, err := os.MkdirTemp(root, ".projection-*")
	if err != nil {
		return Receipt{}, err
	}
	defer os.RemoveAll(tmp)

	jsonTmp := filepath.Join(tmp, "field.json")
	htmlTmp := filepath.Join(tmp, "field.html")
	if err := JSON(jsonTmp, st); err != nil {
		return Receipt{}, err
	}
	if err := HTML(htmlTmp, st); err != nil {
		return Receipt{}, err
	}
	jsonHash, err := fileHash(jsonTmp)
	if err != nil {
		return Receipt{}, err
	}
	htmlHash, err := fileHash(htmlTmp)
	if err != nil {
		return Receipt{}, err
	}
	receipt := Receipt{TipHash: st.TipHash, Events: st.Events, JSONHash: jsonHash, HTMLHash: htmlHash, Generated: time.Now().UTC()}
	receiptBytes, err := json.MarshalIndent(receipt, "", "  ")
	if err != nil {
		return Receipt{}, err
	}
	receiptTmp := filepath.Join(tmp, "projection.current.json")
	if err := os.WriteFile(receiptTmp, receiptBytes, 0o644); err != nil {
		return Receipt{}, err
	}
	if err := replaceFile(jsonTmp, filepath.Join(root, "field.json")); err != nil {
		return Receipt{}, err
	}
	if err := replaceFile(htmlTmp, filepath.Join(root, "field.html")); err != nil {
		return Receipt{}, err
	}
	if err := replaceFile(receiptTmp, filepath.Join(root, "projection.current.json")); err != nil {
		return Receipt{}, err
	}
	return receipt, nil
}

type saabLink struct {
	FromPRM string
	ToPRM   string
	Kind    string
}

type commercialProjection struct {
	SourcePRMID string
	Model       string
	Customer    string
	Value       string
	RevenuePath string
}

type saab struct {
	ID               string
	MemberPRMIDs     []string
	Capabilities     []string
	Commercial       []commercialProjection
	CompositionLinks []saabLink
	BuildNodes       []core.BuildNode
	BuildEdges       []core.BuildEdge
}

type cpsl struct {
	SAABID  string
	Program string
}

func deriveSAABs(st core.State) ([]saab, error) {
	prms, err := crystallizePRMs(st)
	if err != nil {
		return nil, err
	}

	byID := make(map[string]prm, len(prms))
	adjacency := make(map[string]map[string]bool, len(prms))

	for _, item := range prms {
		byID[item.SourceEmergIONID] = item
		adjacency[item.SourceEmergIONID] = map[string]bool{}
	}

	for _, item := range prms {
		target := strings.TrimSpace(
			item.Relationships["COMPOSITION_KIN"],
		)
		if target == "" {
			continue
		}

		if target == item.SourceEmergIONID {
			return nil, fmt.Errorf(
				"SAAB rejected self COMPOSITION_KIN %s",
				target,
			)
		}

		if _, ok := byID[target]; !ok {
			return nil, fmt.Errorf(
				"SAAB composition target not accepted PRM: %s",
				target,
			)
		}

		// Membership connectivity is undirected. The explicit governed
		// composition relation itself remains directed in CompositionLinks.
		adjacency[item.SourceEmergIONID][target] = true
		adjacency[target][item.SourceEmergIONID] = true
	}

	ids := make([]string, 0, len(prms))
	for id := range byID {
		ids = append(ids, id)
	}
	sort.Strings(ids)

	visited := map[string]bool{}
	var out []saab

	for _, start := range ids {
		if visited[start] || len(adjacency[start]) == 0 {
			continue
		}

		queue := []string{start}
		visited[start] = true

		var members []string

		for len(queue) > 0 {
			current := queue[0]
			queue = queue[1:]
			members = append(members, current)

			neighbors := make(
				[]string,
				0,
				len(adjacency[current]),
			)
			for neighbor := range adjacency[current] {
				neighbors = append(neighbors, neighbor)
			}
			sort.Strings(neighbors)

			for _, neighbor := range neighbors {
				if visited[neighbor] {
					continue
				}
				visited[neighbor] = true
				queue = append(queue, neighbor)
			}
		}

		sort.Strings(members)

		if len(members) < 2 {
			continue
		}

		assembly := saab{
			ID:           "SAAB:" + strings.Join(members, "+"),
			MemberPRMIDs: append([]string(nil), members...),
		}

		capSet := map[string]bool{}
		linkSet := map[string]bool{}

		for _, memberID := range members {
			item := byID[memberID]

			accepted := st.Accepted[memberID]
			if accepted.EVO.Metadata != nil &&
				accepted.EVO.Metadata.Monetization != nil {
				monetization := accepted.EVO.Metadata.Monetization
				assembly.Commercial = append(
					assembly.Commercial,
					commercialProjection{
						SourcePRMID: memberID,
						Model:       monetization.Model,
						Customer:    monetization.Customer,
						Value:       monetization.Value,
						RevenuePath: monetization.RevenuePath,
					},
				)
			}

			for _, capability := range accepted.CAP {
				if capability != "" {
					capSet[capability] = true
				}
			}

			target := strings.TrimSpace(
				item.Relationships["COMPOSITION_KIN"],
			)
			if target != "" {
				if _, inside := byID[target]; inside {
					key := memberID + "\x00" + target
					if !linkSet[key] {
						linkSet[key] = true
						assembly.CompositionLinks = append(
							assembly.CompositionLinks,
							saabLink{
								FromPRM: memberID,
								ToPRM:   target,
								Kind:    "COMPOSITION_KIN",
							},
						)
					}
				}
			}

			for _, node := range item.BuildNodes {
				namespaced := node
				namespaced.ID = memberID + "::" + node.ID

				assembly.BuildNodes = append(
					assembly.BuildNodes,
					namespaced,
				)
			}

			for _, edge := range item.BuildEdges {
				namespaced := edge
				namespaced.From = memberID + "::" + edge.From
				namespaced.To = memberID + "::" + edge.To

				assembly.BuildEdges = append(
					assembly.BuildEdges,
					namespaced,
				)
			}
		}

		for capability := range capSet {
			assembly.Capabilities = append(
				assembly.Capabilities,
				capability,
			)
		}
		sort.Strings(assembly.Capabilities)

		sort.Slice(
			assembly.CompositionLinks,
			func(i, j int) bool {
				if assembly.CompositionLinks[i].FromPRM !=
					assembly.CompositionLinks[j].FromPRM {
					return assembly.CompositionLinks[i].FromPRM <
						assembly.CompositionLinks[j].FromPRM
				}
				return assembly.CompositionLinks[i].ToPRM <
					assembly.CompositionLinks[j].ToPRM
			},
		)

		sort.Slice(
			assembly.BuildNodes,
			func(i, j int) bool {
				return assembly.BuildNodes[i].ID <
					assembly.BuildNodes[j].ID
			},
		)

		sort.Slice(
			assembly.BuildEdges,
			func(i, j int) bool {
				left := assembly.BuildEdges[i]
				right := assembly.BuildEdges[j]

				if left.From != right.From {
					return left.From < right.From
				}
				if left.To != right.To {
					return left.To < right.To
				}
				return left.Kind < right.Kind
			},
		)

		out = append(out, assembly)
	}

	sort.Slice(out, func(i, j int) bool {
		return out[i].ID < out[j].ID
	})

	return out, nil
}

func compileCPSL(st core.State) ([]cpsl, error) {
	assemblies, err := deriveSAABs(st)
	if err != nil {
		return nil, err
	}

	out := make([]cpsl, 0, len(assemblies))

	for _, assembly := range assemblies {
		var b strings.Builder

		b.WriteString("CPSL/1\n")
		fmt.Fprintf(&b, "A|%q\n", assembly.ID)

		for _, member := range assembly.MemberPRMIDs {
			fmt.Fprintf(&b, "P|%q\n", member)
		}

		for _, capability := range assembly.Capabilities {
			fmt.Fprintf(&b, "C|%q\n", capability)
		}

		for _, link := range assembly.CompositionLinks {
			fmt.Fprintf(
				&b,
				"L|%q|%q|%q\n",
				link.FromPRM,
				link.ToPRM,
				link.Kind,
			)
		}

		for _, node := range assembly.BuildNodes {
			fmt.Fprintf(
				&b,
				"N|%q|%q|%q\n",
				node.ID,
				node.System,
				node.State,
			)
		}

		for _, edge := range assembly.BuildEdges {
			fmt.Fprintf(
				&b,
				"E|%q|%q|%q\n",
				edge.From,
				edge.To,
				edge.Kind,
			)
		}

		b.WriteString("Z\n")

		out = append(out, cpsl{
			SAABID:  assembly.ID,
			Program: b.String(),
		})
	}

	return out, nil
}

type saw struct {
	ID           string
	SAABID       string
	MemberPRMIDs []string
	Capabilities []string
	Commercial   []commercialProjection
	CPSL         string
}

func extractSAWs(st core.State) ([]saw, error) {
	assemblies, err := deriveSAABs(st)
	if err != nil {
		return nil, err
	}

	programs, err := compileCPSL(st)
	if err != nil {
		return nil, err
	}

	cpslBySAAB := make(map[string]cpsl, len(programs))
	for _, program := range programs {
		if _, exists := cpslBySAAB[program.SAABID]; exists {
			return nil, fmt.Errorf(
				"duplicate CPSL for SAAB %s",
				program.SAABID,
			)
		}
		cpslBySAAB[program.SAABID] = program
	}

	out := make([]saw, 0, len(assemblies))

	for _, assembly := range assemblies {
		program, ok := cpslBySAAB[assembly.ID]
		if !ok {
			return nil, fmt.Errorf(
				"SAW missing CPSL for SAAB %s",
				assembly.ID,
			)
		}

		item := saw{
			ID:           "SAW:" + assembly.ID,
			SAABID:       assembly.ID,
			MemberPRMIDs: append([]string(nil), assembly.MemberPRMIDs...),
			Capabilities: append([]string(nil), assembly.Capabilities...),
			Commercial:   append([]commercialProjection(nil), assembly.Commercial...),
			CPSL:         program.Program,
		}

		out = append(out, item)
	}

	sort.Slice(out, func(i, j int) bool {
		return out[i].ID < out[j].ID
	})

	return out, nil
}

type SAWSource struct {
	ID      string
	Content []byte
}

func SAWSources(st core.State) ([]SAWSource, error) {
	artifacts, err := extractSAWs(st)
	if err != nil {
		return nil, err
	}

	out := make([]SAWSource, 0, len(artifacts))

	for _, artifact := range artifacts {
		var b strings.Builder

		b.WriteString("SAW/1\n")
		fmt.Fprintf(&b, "I|%q\n", artifact.ID)
		fmt.Fprintf(&b, "A|%q\n", artifact.SAABID)

		for _, member := range artifact.MemberPRMIDs {
			fmt.Fprintf(&b, "P|%q\n", member)
		}

		for _, capability := range artifact.Capabilities {
			fmt.Fprintf(&b, "C|%q\n", capability)
		}

		for _, commercial := range artifact.Commercial {
			fmt.Fprintf(
				&b,
				"M|%q|%q|%q|%q|%q\n",
				commercial.SourcePRMID,
				commercial.Model,
				commercial.Customer,
				commercial.Value,
				commercial.RevenuePath,
			)
		}

		fmt.Fprintf(&b, "X|%q\n", artifact.CPSL)
		b.WriteString("Z\n")

		out = append(out, SAWSource{
			ID:      artifact.ID,
			Content: []byte(b.String()),
		})
	}

	return out, nil
}
