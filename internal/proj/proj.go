package proj

import (
	"emergion-sovereign-runtime/internal/core"
	"encoding/json"
	"fmt"
	"html/template"
	"os"
	"path/filepath"
	"sort"
	"strings"
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

type item struct{ Name, Value string }
type row struct {
	ID, State, StateLabel, Summary, Risk, Source, Captured, Reasoner, PromptSchema string
	Prompts []string
	Model, Customer, Value, RevenuePath, NextAction, Confidence string
	Facets, Nodes, Edges []item
}

func stateLabel(state string) string {
	switch state {
	case core.StateAtGOV:
		return "Awaiting governance"
	case core.StateApproved:
		return "Approved"
	case core.StateHeld:
		return "Held"
	case core.StateRejected:
		return "Rejected"
	case core.StateReturned:
		return "Returned"
	case core.StateAccepted:
		return "Registry accepted"
	default:
		return state
	}
}

func makeRow(e core.EmergION) row {
	r := row{ID: e.IDN, State: e.STA, StateLabel: stateLabel(e.STA), Summary: e.MEM.Summary, Risk: e.VAL.Risk, Source: e.REL["source_name"], Reasoner: e.VAL.Reasoner}
	m := e.EVO.Metadata
	if m == nil {
		return r
	}
	r.Captured = m.CapturedAt.UTC().Format("2006-01-02 15:04:05 UTC")
	r.PromptSchema = m.PromptSchema
	r.Prompts = m.LogicalPrompts
	for _, f := range m.Facets {
		r.Facets = append(r.Facets, item{strings.ReplaceAll(string(f), "_", " "), "enabled"})
	}
	for _, n := range m.BuildNodes {
		r.Nodes = append(r.Nodes, item{n.ID, n.System + " · " + n.State})
	}
	for _, e := range m.BuildEdges {
		r.Edges = append(r.Edges, item{e.From + " → " + e.To, e.Kind})
	}
	if m.Monetization != nil {
		r.Model, r.Customer, r.Value = m.Monetization.Model, m.Monetization.Customer, m.Monetization.Value
		r.RevenuePath, r.NextAction, r.Confidence = m.Monetization.RevenuePath, m.Monetization.NextAction, m.Monetization.Confidence
	}
	return r
}

func rows(st core.State) []row {
	var out []row
	add := func(m map[string]core.EmergION) {
		for _, e := range m {
			out = append(out, makeRow(e))
		}
	}
	add(st.AtGOV)
	add(st.Approved)
	add(st.Accepted)
	add(st.Held)
	add(st.Rejected)
	add(st.Returned)
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out
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
	t := template.Must(template.New("f").Parse(`<!doctype html><html><head><meta charset="utf-8"><meta name="viewport" content="width=device-width,initial-scale=1"><title>FIELD Registry</title><style>
:root{color-scheme:dark;--bg:#070910;--panel:#10141f;--line:#273044;--text:#f2f5fb;--muted:#9ba8bd;--violet:#a78bfa;--cyan:#67e8f9;--green:#4ade80;--amber:#fbbf24}*{box-sizing:border-box}body{margin:0;background:radial-gradient(circle at top right,#18203a 0,var(--bg) 36rem);color:var(--text);font:15px/1.55 system-ui,sans-serif}.shell{max-width:1280px;margin:auto;padding:32px 20px 72px}header{display:flex;justify-content:space-between;gap:20px;align-items:end;border-bottom:1px solid var(--line);padding-bottom:24px}h1{margin:0;font-size:clamp(2rem,6vw,4.5rem);letter-spacing:-.06em}.eyebrow{color:var(--cyan);font-size:.75rem;font-weight:800;letter-spacing:.18em;text-transform:uppercase}.sub,.meta{color:var(--muted)}.stats{display:grid;grid-template-columns:repeat(3,minmax(110px,1fr));gap:10px;margin:24px 0}.stat,.card{background:#10141fee;border:1px solid var(--line);border-radius:16px}.stat{padding:15px}.stat b{display:block;font-size:1.5rem}.grid{display:grid;gap:18px}.card{padding:22px;box-shadow:0 18px 50px #0005}.top{display:flex;gap:12px;justify-content:space-between;align-items:start}.id{font:700 .82rem ui-monospace;color:var(--violet)}.badge{border:1px solid var(--line);border-radius:99px;padding:5px 10px;font-size:.76rem}.F{color:var(--green)}.G{color:var(--amber)}h2{font-size:1.35rem;margin:12px 0 6px}.summary{font-size:1.08rem;max-width:78ch}.cols{display:grid;grid-template-columns:1fr 1fr;gap:18px;margin-top:20px}.section{border-top:1px solid var(--line);padding-top:15px}.section h3{font-size:.74rem;text-transform:uppercase;letter-spacing:.14em;color:var(--muted);margin:0 0 10px}.commercial{display:grid;grid-template-columns:repeat(2,1fr);gap:10px}.kv{background:#080b13;border-radius:10px;padding:10px}.kv small{display:block;color:var(--muted)}.facets{display:grid;grid-template-columns:repeat(3,1fr);gap:8px}.facet{border-left:2px solid var(--violet);padding:7px 9px;background:#0a0d16}.facet b{display:block;font-size:.7rem;color:var(--muted);text-transform:uppercase}.prompts{margin:0;padding-left:20px}.empty{text-align:center;padding:70px 20px;color:var(--muted)}code{color:var(--violet);word-break:break-all}@media(max-width:760px){header{display:block}.stats,.cols,.facets{grid-template-columns:1fr}.commercial{grid-template-columns:1fr}}
</style></head><body><main class="shell"><header><div><div class="eyebrow">EmergION / REG / FIELD</div><h1>Living Registry</h1><div class="sub">Governed intelligence made visible.</div></div><div class="meta">Chain tip<br><code>{{.TipHash}}</code></div></header><section class="stats"><div class="stat"><span class="meta">Events</span><b>{{.Events}}</b></div><div class="stat"><span class="meta">Entities</span><b>{{len .Rows}}</b></div><div class="stat"><span class="meta">Projection</span><b>LIVE</b></div></section><section class="grid">{{range .Rows}}<article class="card"><div class="top"><div><div class="id">{{.ID}}</div><h2>{{if .Source}}{{.Source}}{{else}}EmergION entity{{end}}</h2></div><span class="badge {{.State}}">{{.StateLabel}} · risk {{.Risk}}</span></div><p class="summary">{{.Summary}}</p><div class="meta">Captured {{if .Captured}}{{.Captured}}{{else}}before metadata timestamping{{end}} · {{if .Reasoner}}{{.Reasoner}}{{else}}reasoner not recorded{{end}} · {{.PromptSchema}}</div><div class="cols"><section class="section"><h3>Monetized idea attributes</h3><div class="commercial"><div class="kv"><small>Model</small>{{.Model}}</div><div class="kv"><small>Customer</small>{{.Customer}}</div><div class="kv"><small>Value</small>{{.Value}}</div><div class="kv"><small>Confidence</small>{{.Confidence}}</div></div><div class="kv" style="margin-top:10px"><small>Revenue path</small>{{.RevenuePath}}</div><div class="kv" style="margin-top:10px"><small>Next action</small>{{.NextAction}}</div></section><section class="section"><h3>Logical prompts</h3><ol class="prompts">{{range .Prompts}}<li>{{.}}</li>{{else}}<li>Not recorded in this legacy entity.</li>{{end}}</ol></section></div><section class="section"><h3>Dodecahedron topology · governed facets</h3><div class="facets">{{range .Facets}}<div class="facet"><b>{{.Name}}</b>{{.Value}}</div>{{else}}<div class="meta">Not recorded in this legacy entity.</div>{{end}}</div></section>{{if .Nodes}}<div class="cols"><section class="section"><h3>Build nodes</h3>{{range .Nodes}}<div class="kv"><small>{{.Name}}</small>{{.Value}}</div>{{end}}</section><section class="section"><h3>Build edges</h3>{{range .Edges}}<div class="kv"><small>{{.Name}}</small>{{.Value}}</div>{{end}}</section></div>{{end}}</article>{{else}}<div class="card empty">No governed entities yet. Capture a source, approve it at HUMAN_FINAL, then render FIELD.</div>{{end}}</section></main></body></html>`))
	return t.Execute(f, map[string]any{"Events": st.Events, "TipHash": st.TipHash, "Rows": rows(st)})
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
