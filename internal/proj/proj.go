package proj

import (
	"archive/zip"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"html/template"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strconv"
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

func humanFinalRows(rows []row) []row {
	var primary, alternatives []row
	for _, r := range rows {
		if r.State != core.StateAtGOV {
			continue
		}
		if strings.Contains(r.Relationships, "source_name → provider_population") {
			alternatives = append(alternatives, r)
		} else {
			primary = append(primary, r)
		}
	}
	return append(primary, alternatives...)
}

type realizationRow struct {
	TargetThing string
	Selected    string
	RunnersUp   string
	SourceID    string
	SourceKind  string
	Status      string
}

func realizationRows(st core.State) []realizationRow {
	byTarget := map[string]realizationRow{}

	// REG-accepted realization state is the current canonical order.
	acceptedIDs := make([]string, 0, len(st.Accepted))
	for id := range st.Accepted {
		acceptedIDs = append(acceptedIDs, id)
	}
	sort.Strings(acceptedIDs)
	for _, id := range acceptedIDs {
		em := st.Accepted[id]
		kind := strings.TrimSpace(em.REL["source_kind"])
		if kind != "REALIZATION_SELECTION" && kind != "REALIZATION_REORDER" {
			continue
		}
		target := strings.TrimSpace(em.REL["target_thing"])
		selected := strings.TrimSpace(em.REL["selected_realization"])
		if target == "" || selected == "" {
			continue
		}
		byTarget[target] = realizationRow{
			TargetThing: target,
			Selected:    selected,
			RunnersUp:   strings.TrimSpace(em.REL["runners_up"]),
			SourceID:    em.IDN,
			SourceKind:  kind,
			Status:      "REG ACCEPTED",
		}
	}

	// A pending reorder is the operator-relevant frontier. Project it without
	// treating it as canonical: HUMAN_FINAL must resolve it before REG changes.
	pendingIDs := make([]string, 0, len(st.AtGOV))
	for id := range st.AtGOV {
		pendingIDs = append(pendingIDs, id)
	}
	sort.Strings(pendingIDs)
	for _, id := range pendingIDs {
		em := st.AtGOV[id]
		if strings.TrimSpace(em.REL["source_kind"]) != "REALIZATION_REORDER" {
			continue
		}
		target := strings.TrimSpace(em.REL["target_thing"])
		selected := strings.TrimSpace(em.REL["selected_realization"])
		if target == "" || selected == "" {
			continue
		}
		byTarget[target] = realizationRow{
			TargetThing: target,
			Selected:    selected,
			RunnersUp:   strings.TrimSpace(em.REL["runners_up"]),
			SourceID:    em.IDN,
			SourceKind:  "REALIZATION_REORDER",
			Status:      "PENDING HUMAN_FINAL",
		}
	}

	out := make([]realizationRow, 0, len(byTarget))
	for _, item := range byTarget {
		out = append(out, item)
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i].TargetThing < out[j].TargetThing
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

type spatialFacetGroup struct {
	Facet     core.Facet
	EmergIONs []spatialEmergION
}

func spatialFacetGroups(spatial []spatialEmergION) []spatialFacetGroup {
	groups := make([]spatialFacetGroup, 0, len(spatialFacetOrder()))

	for _, facet := range spatialFacetOrder() {
		group := spatialFacetGroup{Facet: facet}

		for _, em := range spatial {
			for _, emFacet := range em.Facets {
				if emFacet == facet {
					group.EmergIONs = append(group.EmergIONs, em)
					break
				}
			}
		}

		groups = append(groups, group)
	}

	return groups
}

func spatialUnassigned(spatial []spatialEmergION) []spatialEmergION {
	out := make([]spatialEmergION, 0)

	for _, em := range spatial {
		if len(em.Facets) == 0 {
			out = append(out, em)
		}
	}

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

// engineeringTwinStatus is an external observation, never COSL/REG authority.
// It verifies every source entry against the version manifest before projecting it.
type engineeringTwinView struct {
	Status   string
	Package  string
	Captured string
	Files    int
	Added    int
	Removed  int
	Changed  int
	SHA256   string
}

func engineeringTwinStatus(dir string) engineeringTwinView {
	result := engineeringTwinView{Status: "UNAVAILABLE"}
	paths, err := filepath.Glob(filepath.Join(dir, "UNIFUSION_TWIN_*.zip"))
	if err != nil || len(paths) == 0 {
		return result
	}
	sort.Strings(paths)
	path := paths[len(paths)-1]
	result.Package = filepath.Base(path)
	result.Status = "UNVERIFIED"
	f, err := os.Open(path)
	if err != nil {
		return result
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil {
		return result
	}
	z, err := zip.NewReader(f, info.Size())
	if err != nil {
		return result
	}
	entries := make(map[string]*zip.File, len(z.File))
	for _, entry := range z.File {
		if _, exists := entries[entry.Name]; exists {
			return result
		}
		entries[entry.Name] = entry
	}
	read := func(name string) ([]byte, error) {
		entry := entries[name]
		if entry == nil || entry.UncompressedSize64 > 20<<20 {
			return nil, fmt.Errorf("missing or oversized metadata %s", name)
		}
		r, err := entry.Open()
		if err != nil {
			return nil, err
		}
		defer r.Close()
		return io.ReadAll(io.LimitReader(r, 20<<20+1))
	}
	manifestBytes, err := read("VERSION_MANIFEST.json")
	if err != nil {
		return result
	}
	var manifest struct {
		Schema     string `json:"schema"`
		Authority  string `json:"authority"`
		Capture    string `json:"capture"`
		Captured   string `json:"captured_at_utc"`
		FileCount  int    `json:"file_count"`
		LedgerHash string `json:"source_ledger_sha256"`
		Files      []struct {
			Bytes  int64  `json:"bytes"`
			SHA256 string `json:"sha256"`
		} `json:"files"`
	}
	if json.Unmarshal(manifestBytes, &manifest) != nil ||
		manifest.Schema != "UNIFUSION_VERSIONED_TWIN_v1" ||
		manifest.Authority != "READ_ONLY_NOT_COSL_REG" ||
		manifest.FileCount <= 0 || manifest.FileCount != len(manifest.Files) ||
		result.Package != "UNIFUSION_TWIN_"+manifest.Capture+".zip" {
		return result
	}
	ledger, err := read("EXTERNAL_SOURCE_LEDGER.json")
	if err != nil {
		return result
	}
	sum := sha256.Sum256(ledger)
	if !strings.EqualFold(hex.EncodeToString(sum[:]), manifest.LedgerHash) {
		return result
	}
	var ledgerInfo struct {
		Authority string            `json:"authority"`
		FileCount int               `json:"file_count"`
		Gaps      []json.RawMessage `json:"gaps"`
	}
	if json.Unmarshal(ledger, &ledgerInfo) != nil || ledgerInfo.Authority != "READ_ONLY_SOURCE_COPY_NOT_COSL_REG" ||
		ledgerInfo.FileCount != manifest.FileCount || len(ledgerInfo.Gaps) != 0 {
		return result
	}
	reportBytes, err := read("SOURCE_CHANGE_REPORT.json")
	if err != nil {
		return result
	}
	var report struct {
		Authority   string `json:"authority"`
		NewCaptured string `json:"new_captured_at"`
		Counts      struct {
			Added     int `json:"added"`
			Removed   int `json:"removed"`
			Changed   int `json:"changed"`
			Unchanged int `json:"unchanged"`
		} `json:"counts"`
		Gaps []json.RawMessage `json:"capture_gaps"`
	}
	if json.Unmarshal(reportBytes, &report) != nil || report.Authority != manifest.Authority ||
		report.NewCaptured != manifest.Captured || len(report.Gaps) != 0 ||
		report.Counts.Added < 0 || report.Counts.Removed < 0 || report.Counts.Changed < 0 ||
		report.Counts.Unchanged < 0 ||
		report.Counts.Added+report.Counts.Changed+report.Counts.Unchanged != manifest.FileCount {
		return result
	}
	// Compare source content hashes as a multiset. This does not assume root naming.
	expected := map[string]int{}
	for _, item := range manifest.Files {
		if len(item.SHA256) != 64 || item.Bytes < 0 {
			return result
		}
		expected[strings.ToLower(item.SHA256)+":"+strconv.FormatInt(item.Bytes, 10)]++
	}
	found := 0
	for _, entry := range z.File {
		if !strings.HasPrefix(entry.Name, "sources/") || entry.FileInfo().IsDir() {
			continue
		}
		if entry.UncompressedSize64 > 100<<20 {
			return result
		}
		reader, err := entry.Open()
		if err != nil {
			return result
		}
		hash := sha256.New()
		n, copyErr := io.Copy(hash, io.LimitReader(reader, 100<<20+1))
		closeErr := reader.Close() // ZIP CRC is checked when fully consumed.
		if copyErr != nil || closeErr != nil {
			return result
		}
		key := hex.EncodeToString(hash.Sum(nil)) + ":" + strconv.FormatInt(n, 10)
		if expected[key] == 0 {
			return result
		}
		expected[key]--
		found++
	}
	if found != manifest.FileCount {
		return result
	}
	for _, remaining := range expected {
		if remaining != 0 {
			return result
		}
	}
	archiveHash := sha256.New()
	if _, err := f.Seek(0, io.SeekStart); err != nil {
		return result
	}
	if _, err := io.Copy(archiveHash, f); err != nil {
		return result
	}
	result.Status = "VERIFIED (external observation)"
	result.Captured = manifest.Captured
	result.Files = manifest.FileCount
	result.Added = report.Counts.Added
	result.Removed = report.Counts.Removed
	result.Changed = report.Counts.Changed
	result.SHA256 = hex.EncodeToString(archiveHash.Sum(nil))
	return result
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
<title>UNIFUSION · Sovereign Intelligence</title>
<style>
body{font:15px system-ui;background:#020817;color:#e6eaf0;margin:0;padding:2rem}a{color:#c9a227}.ok,.good{color:#2ecc71}.bad,.err{color:#e74c3c}.muted{color:#8b95a8}.panel{background:#0f1c2e;border:1px solid #2a3548;border-radius:12px;padding:1rem;margin:0.75rem 0}h1,h2{color:#e6eaf0;border-bottom:1px solid #2d6cff;padding-bottom:0.35rem}code,pre{background:#0a1628;color:#e6eaf0;border:1px solid #2a3548}
h1{font-size:1.5rem}
h2{margin-top:2rem;font-size:1.15rem}
html,body{max-width:100%;overflow-x:hidden;box-sizing:border-box}
body{width:100%}
.panel,.field-topology,.field-facet,.field-emergion{box-sizing:border-box;min-width:0;max-width:100%}
table{width:100%;max-width:100%;min-width:0;table-layout:fixed;border-collapse:collapse}
.human-final-tools{display:flex;gap:.6rem;align-items:center;flex-wrap:wrap;margin:.75rem 0}
.human-final-tools input{min-width:0;flex:1 1 220px;padding:.65rem;background:#071120;color:#e6eaf0;border:1px solid #2a3548;border-radius:8px}
.human-final-tools button{padding:.65rem;border-radius:8px}
#human-final-count{color:#9ca3af}
#human-final-table tr[hidden]{display:none!important}
pre{max-width:100%;white-space:pre-wrap;overflow-wrap:anywhere}

th,td{text-align:left;vertical-align:top;padding:.7rem;border-bottom:1px solid #2b3140;overflow-wrap:anywhere;word-break:break-word}
code{color:#a78bfa;overflow-wrap:anywhere;word-break:break-all}
.G{color:#f59e0b}
.F{color:#22c55e}
.X{color:#ef4444}
.small{color:#9ca3af;font-size:.9rem}
.diagnostic summary{cursor:pointer;font-weight:700}
.diagnostic summary::marker{color:#9ca3af}
.field-topology{position:relative;display:grid;grid-template-columns:repeat(4,minmax(0,1fr));gap:.75rem;padding:1rem;margin:.75rem 0;background:#071120;border:1px solid #2a3548;border-radius:12px}
.field-facet{min-height:4.5rem;padding:.75rem;background:#0f1c2e;border:1px solid #2d6cff;border-radius:10px}
.field-facet summary{cursor:pointer;font-weight:700}
.field-facet summary::marker{color:#9ca3af}
.field-facet strong{display:block;margin-bottom:.35rem}
.field-emergions{display:grid;grid-template-columns:repeat(auto-fit,minmax(220px,1fr));gap:.75rem;margin:.75rem 0}
.field-emergion{padding:.75rem;background:#0f1c2e;border:1px solid #2a3548;border-radius:10px}
.field-emergion .facets{margin-top:.45rem;color:#9ca3af;font-size:.85rem}
@media (max-width:700px){
.field-topology{grid-template-columns:minmax(0,1fr)}

body{padding:1rem}
.operator-table{width:100%;min-width:0}
.operator-table thead{display:none}
.operator-table,.operator-table tbody,.operator-table tr,.operator-table td{display:block;width:100%}
.operator-table tr{box-sizing:border-box;background:#0f1c2e;border:1px solid #2a3548;border-radius:10px;padding:.75rem;margin:.75rem 0}
.operator-table td{box-sizing:border-box;border:0;padding:.35rem 0;font-size:.9rem;overflow-wrap:anywhere;word-break:break-word}
.operator-table pre{
white-space:pre-wrap;
overflow-wrap:anywhere;
word-break:break-word;
max-width:100%;
box-sizing:border-box;
}
.operator-table td::before{content:attr(data-label);display:block;color:#9ca3af;font-size:.75rem;font-weight:700;margin-bottom:.15rem}
.operator-table button{max-width:100%;margin:.2rem .35rem .2rem 0}
}
.operator-table tr[hidden]{display:none}
/* UNIFUSION — operator visual identity */
:root{color-scheme:dark;--uf-bg:#050B17;--uf-panel:#10233F;--uf-accent:#326BFF;--uf-text:#E8F0FF;--uf-muted:#A5B5CE;--uf-border:#29446D}
html{scroll-behavior:smooth}
body{background:radial-gradient(ellipse 85% 28% at 85% 0%,rgba(50,107,255,.19),transparent 75%),radial-gradient(ellipse 65% 25% at 0% 45%,rgba(31,75,150,.12),transparent 80%),var(--uf-bg);color:var(--uf-text);line-height:1.55;min-height:100vh}
h1{font-size:clamp(1.7rem,5vw,2.5rem);font-weight:800;letter-spacing:-.045em;border:0;margin:.4rem 0 1.3rem}
h2{color:var(--uf-text);font-size:1.2rem;letter-spacing:-.025em;border-bottom:1px solid var(--uf-border);padding-bottom:.65rem}
a{color:#93B0FF;text-decoration-color:rgba(147,176,255,.35);text-underline-offset:3px}
a:hover{color:#D1DFFF}
.panel{background:linear-gradient(145deg,rgba(21,47,84,.97),rgba(12,29,54,.97));border:1px solid var(--uf-border);border-radius:16px;padding:1.15rem;box-shadow:0 10px 32px rgba(0,0,0,.17)}
.small,.muted,#human-final-count{color:var(--uf-muted)}
code,pre{background:#0B1930;color:#DCE7FF;border:1px solid #29446D;border-radius:6px}
code{color:#B9CDFF}
th,td{border-bottom-color:var(--uf-border)}
th{color:#AFC6FF;font-size:.82rem;letter-spacing:.025em}
button{background:#193D80;color:#F1F5FF;border:1px solid #507DDA;border-radius:9px;padding:.65rem .9rem;font:inherit;font-weight:650;cursor:pointer}
button:hover{background:#2858AD;border-color:#91B1FF}
button:focus-visible,a:focus-visible,input:focus-visible,summary:focus-visible{outline:2px solid #AFC6FF;outline-offset:3px}
.human-final-tools{background:rgba(16,35,63,.75);border:1px solid var(--uf-border);border-radius:13px;padding:.85rem}
.human-final-tools input{background:#071428;color:var(--uf-text);border:1px solid #45618A;border-radius:9px;font:inherit}
.human-final-tools input::placeholder{color:#A5B5CE}
.field-topology{background:#071428;border-color:var(--uf-border)}
.field-facet,.field-emergion{background:linear-gradient(145deg,#142C4D,#0C1C33);border-color:var(--uf-border);border-radius:13px}
.diagnostic summary,.field-facet summary{color:#DCE7FF}
.diagnostic summary::marker,.field-facet summary::marker{color:#93B0FF}
#human-final-table td[data-label="Meaning"]{color:#F1F5FF;font-weight:550}
#human-final-table td[data-label="Relationships"]{color:#A5B5CE;font-size:.85rem}
@media(max-width:700px){
body{padding:.85rem}
.panel{padding:1rem;border-radius:14px}
.operator-table tr{background:linear-gradient(145deg,#142C4D,#0C1C33);border:1px solid var(--uf-border);border-radius:14px;padding:1rem;box-shadow:0 8px 24px rgba(0,0,0,.16)}
.operator-table td::before{color:#A5B5CE;letter-spacing:.025em}
#human-final-table td[data-label="Meaning"]{order:0}
#human-final-table td[data-label="Relationships"]{font-size:.8rem}
.human-final-tools input{flex-basis:100%}
}
@media(prefers-reduced-motion:reduce){html{scroll-behavior:auto}}

.uf-header{position:relative;overflow:hidden;padding:1.5rem 1.25rem;margin-bottom:1rem;background:linear-gradient(125deg,#122E55 0%,#08182E 55%,#030916 100%);border:1px solid #345782;border-radius:18px;box-shadow:0 14px 40px rgba(0,0,0,.28)}
.uf-header::after{content:"";position:absolute;right:-35px;top:-65px;width:190px;height:190px;border:1px solid rgba(117,165,255,.28);border-radius:50%;box-shadow:0 0 0 24px rgba(66,113,200,.035),0 0 0 48px rgba(66,113,200,.025);pointer-events:none}
.uf-header h1{position:relative;margin:.25rem 0;font-size:clamp(2rem,7vw,3.3rem);letter-spacing:.075em;font-weight:750}
.uf-eyebrow{margin:0;color:#91B5FF;font-size:.68rem;letter-spacing:.18em;font-weight:700}
.uf-subtitle{margin:.3rem 0 0;color:#B2C6E3;font-size:.9rem}
.uf-navigation{padding:1rem}
.uf-navigation summary{color:#E8F0FF;font-weight:700;cursor:pointer}
.uf-navigation nav{display:flex;flex-wrap:wrap;gap:.5rem;margin-top:1rem}
.uf-navigation nav a{display:inline-flex;padding:.55rem .8rem;background:#102746;border:1px solid #355785;border-radius:8px;color:#DCE8FF;text-decoration:none;font-size:.85rem;font-weight:600}
.uf-navigation nav a:hover,.uf-navigation nav a:focus-visible{background:#204B89;border-color:#87ACFF}

</style>
</head>
<body>

<header class="uf-header"><p class="uf-eyebrow">ECOPIVOT TECHNOLOGIES</p><h1 id="field-top">UNIFUSION</h1><p class="uf-subtitle">Sovereign intelligence · Evidence · Authority</p></header>
<section class="panel" id="engineering-twin"><h2>Engineering Twin (external observation)</h2>
<p>Status: <strong>{{.Twin.Status}}</strong></p>
<p>Package: <code>{{.Twin.Package}}</code></p>
{{if .Twin.SHA256}}<p>Captured: {{.Twin.Captured}} · Source files: {{.Twin.Files}} · Added: {{.Twin.Added}} · Removed: {{.Twin.Removed}} · Changed: {{.Twin.Changed}}</p>
<p>ZIP SHA-256: <code>{{.Twin.SHA256}}</code></p>{{end}}
<p class="small">Read-only evidence. Not COSL, REG, or HUMAN_FINAL authority.</p></section>
<details class="panel diagnostic uf-navigation" open><summary>Operator navigation</summary><nav aria-label="FIELD sections"><a href="#realization-field">Realization</a> · <a href="#human-final">HUMAN_FINAL</a> · <a href="#verified-deliveries">Deliveries</a> · <a href="#lifecycle-projection">Lifecycle</a> · <a href="#spatial-convergence">Convergence</a> · <a href="#spatial-emergion">Topology</a> · <a href="#saw-work">SAW</a></nav><p class="small">Navigation only; governance remains unchanged.</p></details>
<p>Events: {{.Events}} · Tip: <code>{{.TipHash}}</code></p>
<p class="small">Projection is derived state only. COSL + governance remain authoritative.</p>

<span id="realization-field"></span><h2>REALIZATION FIELD</h2>
<p class="small">Target-specific governed order projected from canonical state. Pending reorder is visible but is not canonical until HUMAN_FINAL + REG.</p>
{{if .Realizations}}
<table class="operator-table">
<thead>
<tr>
<th>THING</th>
<th>Strongest</th>
<th>Runners-up</th>
<th>Status</th>
<th>Evidence EmergION</th>
<th>Kind</th>
</tr>
</thead>
<tbody>
{{range .Realizations}}
<tr>
<td data-label="THING"><strong>{{.TargetThing}}</strong></td>
<td data-label="Strongest"><code>{{.Selected}}</code></td>
<td data-label="Runners-up"><code>{{.RunnersUp}}</code></td>
<td data-label="Status">{{.Status}}</td>
<td data-label="Evidence EmergION"><code>{{.SourceID}}</code></td>
<td data-label="Kind">{{.SourceKind}}</td>
</tr>
{{end}}
</tbody>
</table>
{{else}}
<p class="small">No governed realization population projected.</p>
{{end}}

<span id="human-final"></span><h2>HUMAN_FINAL</h2>
{{if .HumanFinal}}
<p class="small">{{len .HumanFinal}} decision(s) awaiting HUMAN_FINAL. Decisions require an explicit reason and confirmation.</p>
<div class="human-final-tools"><label for="human-final-filter">Find decision</label><input id="human-final-filter" type="search" placeholder="Filter by ID, risk, proof or meaning" aria-controls="human-final-table"><button type="button" id="human-final-clear">Clear</button><span id="human-final-count" role="status" aria-live="polite"></span></div>
<table class="operator-table" id="human-final-table">
<thead>
<tr>
<th>EmergION</th>
<th>Risk</th>
<th>Evidence proof</th>
<th>Relationships</th>
<th>Meaning</th>
<th>Decision</th>
</tr>
</thead>
<tbody>
{{range .HumanFinal}}
<tr>
<td data-label="EmergION"><code>{{.ID}}</code></td>
<td data-label="Risk">{{.Risk}}</td>
<td data-label="Evidence proof"><code>{{.Proof}}</code></td>
<td data-label="Relationships">{{.Relationships}}</td>
<td data-label="Meaning">{{.Summary}}</td>
<td data-label="Decision">
<button type="button" onclick="humanFinal('{{.ID}}','APPROVE')">APPROVE</button>
<button type="button" onclick="humanFinal('{{.ID}}','REJECT')">REJECT</button>
</td>
</tr>
{{end}}
</tbody>
</table>
{{else}}
<p class="small">No decisions awaiting HUMAN_FINAL.</p>
{{end}}

<span id="verified-deliveries"></span><h2>VERIFIED DELIVERIES</h2>
<p class="small">Operator projection only. Entries are published from REG-accepted successful execution results; canonical authority remains COSL + REG.</p>
<div id="deliveries" class="panel" data-field-tip="{{.TipHash}}"><span class="muted">Loading verified deliveries…</span></div>

<details class="panel diagnostic">
<span id="lifecycle-projection"></span><summary>Lifecycle Projection · {{len .Rows}} records</summary>
<p class="small">Complete reconstructed lifecycle history. Projection only; COSL + governance remain authoritative.</p>
<table class="operator-table">
<thead>
<tr>
<th>EmergION</th>
<th>State</th>
<th>Risk</th>
<th>Evidence proof</th>
<th>Governance</th>
<th>Relationships</th>
<th>Meaning</th>
</tr>
</thead>
<tbody>
{{range .Rows}}
<tr>
<td data-label="EmergION"><code>{{.ID}}</code></td>
<td data-label="State" class="{{.State}}">{{.State}}</td>
<td data-label="Risk">{{.Risk}}</td>
<td data-label="Evidence proof"><code>{{.Proof}}</code></td>
<td data-label="Governance">{{.Governance}}</td>
<td data-label="Relationships">{{.Relationships}}</td>
<td data-label="Meaning">{{.Summary}}</td>
</tr>
{{end}}
</tbody>
</table>
</details>

<details class="panel diagnostic">
<summary id="spatial-convergence">SPATIAL CONVERGENCE ZONE · {{len .Convergence}} accepted</summary>
<p class="small">Accepted governed structures only. Diagnostic structural projection; canonical authority remains COSL + REG.</p>

<table class="operator-table">
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
<td data-label="EmergION"><code>{{.ID}}</code></td>
<td data-label="Meaning">{{.Summary}}</td>
<td data-label="Archonym">{{.Archonym}}</td>
<td data-label="Kin">{{.Kin}}</td>
<td data-label="Topology">{{.Topology}}</td>
<td data-label="Facets">{{.Facets}}</td>
<td data-label="Capabilities">{{.Capabilities}}</td>
<td data-label="Relationships">{{.Relationships}}</td>
<td data-label="Build Nodes">{{.BuildNodes}}</td>
<td data-label="Build Edges">{{.BuildEdges}}</td>
</tr>
{{end}}
</tbody>
</table>
</details>

<details class="panel diagnostic">
<summary id="spatial-emergion">SPATIAL EMERGION PROJECTION · {{len .FacetGroups}} facets</summary>
<p class="small">Accepted EmergION structures projected across the canonical FIELD topology. Projection only; authority remains COSL + REG.</p>

<div class="field-topology">
{{range .FacetGroups}}
<details class="field-facet">
<summary>{{.Facet}} · {{len .EmergIONs}} accepted</summary>
<div class="field-emergions">
{{range .EmergIONs}}
<div class="field-emergion">
<strong><code>{{.ID}}</code></strong>
<div>{{.Archonym}}</div>
<div class="small">{{.Topology}}</div>
<div class="facets">{{range .Facets}}{{.}} {{end}}</div>
</div>
{{end}}
</div>
</details>
{{end}}
</div>

{{if .Unassigned}}
<details class="field-facet">
<summary>UNASSIGNED · {{len .Unassigned}} accepted</summary>
<span class="small">Accepted EmergIONs without canonical facet metadata · projection only</span>
<div class="field-emergions">
{{range .Unassigned}}
<div class="field-emergion">
<strong><code>{{.ID}}</code></strong>
<div>{{.Archonym}}</div>
<div class="small">{{.Topology}}</div>
</div>
{{end}}
</div>
</details>
{{end}}
</details>

<span id="saw-work"></span><h2>SAW WORK PROJECTION</h2>
<p class="small">Deterministic SAW artifacts derived from accepted governed composition.</p>

<table class="operator-table">
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
<td data-label="SAW"><code>{{.ID}}</code></td>
<td data-label="SAAB"><code>{{.SAABID}}</code></td>
<td data-label="Member PRMs">{{.MemberPRMIDs}}</td>
<td data-label="Capabilities">{{.Capabilities}}</td>
<td data-label="Commercial">{{.Commercial}}</td>
<td data-label="CPSL"><pre>{{.CPSL}}</pre></td>
</tr>
{{end}}
</tbody>
</table>

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
    table.className = "operator-table";
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
        ["EmergION", deliveryText(item.emergion_id)],
        ["Action", deliveryText(item.action)],
        ["Adapter", deliveryText(item.adapter)],
        ["Evidence", deliveryText(item.evidence_sha256)],
        ["Output", deliveryText(item.output_sha256)]
      ].forEach(function(entry) {
        const td = document.createElement("td");
        td.dataset.label = entry[0];
        const code = document.createElement("code");
        const value = entry[1];
        code.textContent = value;
        td.appendChild(code);
        row.appendChild(td);
      });

      const artifactCell = document.createElement("td");
      artifactCell.dataset.label = "Artifact";
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

(function initHumanFinalFilter() {
  const input = document.getElementById("human-final-filter");
  const table = document.getElementById("human-final-table");
  if (!input || !table) return;
  const alternativeRows = Array.from(table.querySelectorAll("tbody tr")).filter(function(row) {
    const cell = row.querySelector('td[data-label="Relationships"]');
    return cell && cell.textContent.includes("source_name → provider_population");
  });
  let alternativeDetails = null;
  if (alternativeRows.length) {
    alternativeDetails = document.createElement("details");
    alternativeDetails.className = "panel diagnostic";
    alternativeDetails.id = "human-final-alternatives";
    const summary = document.createElement("summary");
    summary.textContent = "Provider-population alternatives · " + alternativeRows.length;
    alternativeDetails.appendChild(summary);
    const alternativeTable = table.cloneNode(false);
    alternativeTable.id = "human-final-alternatives-table";
    alternativeTable.appendChild(table.tHead.cloneNode(true));
    const body = document.createElement("tbody");
    alternativeRows.forEach(function(row) { body.appendChild(row); });
    alternativeTable.appendChild(body);
    alternativeDetails.appendChild(alternativeTable);
    table.after(alternativeDetails);
  }
  const rows = Array.from(document.querySelectorAll("#human-final-table tbody tr, #human-final-alternatives-table tbody tr"));
  const count = document.getElementById("human-final-count");
  function update() {
    const query = input.value.trim().toLocaleLowerCase();
    let visible = 0;
    rows.forEach(function(row) {
      const matches = row.textContent.toLocaleLowerCase().includes(query);
      row.hidden = !matches;
      if (matches) visible++;
    });
    count.textContent = visible + " of " + rows.length + " shown";
    if (alternativeDetails && query) {
      const matchingAlternative = Array.from(alternativeDetails.querySelectorAll("tbody tr")).some(function(row) {
        return !row.hidden;
      });
      if (matchingAlternative) alternativeDetails.open = true;
    }
  }
  input.addEventListener("input", update);
  document.getElementById("human-final-clear").addEventListener("click", function() {
    input.value = "";
    update();
    input.focus();
  });
  update();
})();

async function humanFinal(id, decision) {
  const reason = prompt("Reason for " + decision + " of " + id + " (required):");
  if (reason === null) return;
  if (!reason.trim()) {
    alert("A HUMAN_FINAL decision requires an explicit reason.");
    return;
  }
  if (!confirm(decision + " " + id + "? This records a governed decision.")) return;
  try {
    const response = await fetch("/decide", {
      method: "POST",
      headers: {"Content-Type": "application/json"},
      body: JSON.stringify({
        emergion_id: id,
        decision: decision,
        reason: reason.trim()
      })
    });
    const result = await response.json();
    if (!response.ok || result.error) {
      alert(result.error || "HUMAN_FINAL decision was not accepted.");
      return;
    }
    location.reload();
  } catch (err) {
    alert("HUMAN_FINAL decision could not be confirmed. Refresh the FIELD before retrying.");
  }
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
	facetGroups := spatialFacetGroups(spatial)
	unassigned := spatialUnassigned(spatial)

	lifecycle := rows(st)

	return t.Execute(f, map[string]any{
		"Twin":         engineeringTwinStatus(filepath.Join(os.Getenv("HOME"), "unifusion-twin-versions")),
		"Events":       st.Events,
		"TipHash":      st.TipHash,
		"Rows":         lifecycle,
		"HumanFinal":   humanFinalRows(lifecycle),
		"Realizations": realizationRows(st),
		"Convergence":  convergence,
		"SAWs":         saws,
		"Spatial":      spatial,
		"FacetGroups":  facetGroups,
		"Unassigned":   unassigned,
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
