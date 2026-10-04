package cosl

import (
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"

	"emergion-sovereign-runtime/internal/core"
)

const (
	Prefix            = "@COSL1|"
	EventPrefix       = "@COSL1|EVT{"
	NativeEventPrefix = "@COSL1|EVT{T="
	LegacyEventPrefix = "@COSL1|EVT{T:"
)

func legacyCanonical(e core.Event) ([]byte, error) {
	e.SelfHash = ""
	return json.Marshal(e)
}

func legacySeal(e core.Event) (core.Event, error) {
	b, err := legacyCanonical(e)
	if err != nil {
		return core.Event{}, err
	}
	sum := sha256.Sum256(b)
	e.SelfHash = hex.EncodeToString(sum[:])
	return e, nil
}

func writeString(b *strings.Builder, value string) {
	// Preserve the original COSL representation for ordinary strings so
	// existing ledger hashes remain stable. Strings containing physical
	// line breaks cross the reciprocal codec pivot through a line-safe form.
	if strings.ContainsAny(value, "\r\n") {
		encoded := base64.RawStdEncoding.EncodeToString([]byte(value))
		fmt.Fprintf(b, "B%d:", len(encoded))
		b.WriteString(encoded)
		return
	}

	fmt.Fprintf(b, "%d:", len(value))
	b.WriteString(value)
}

func writeBool(b *strings.Builder, value bool) {
	if value {
		writeString(b, "1")
		return
	}
	writeString(b, "0")
}

func writeInt(b *strings.Builder, value int) {
	writeString(b, strconv.Itoa(value))
}

func writeInt64(b *strings.Builder, value int64) {
	writeString(b, strconv.FormatInt(value, 10))
}

func writeTime(b *strings.Builder, value time.Time) {
	writeString(b, value.UTC().Format(time.RFC3339Nano))
}

func writeStrings(b *strings.Builder, values []string) {
	if values == nil {
		b.WriteString("-")
		return
	}
	fmt.Fprintf(b, "L%d[", len(values))
	for _, value := range values {
		writeString(b, value)
	}
	b.WriteByte(']')
}

func writeFacets(b *strings.Builder, values []core.Facet) {
	if values == nil {
		b.WriteString("-")
		return
	}
	fmt.Fprintf(b, "F%d[", len(values))
	for _, value := range values {
		writeString(b, string(value))
	}
	b.WriteByte(']')
}

func writeMap(b *strings.Builder, values map[string]string) {
	if values == nil {
		b.WriteString("-")
		return
	}

	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)

	fmt.Fprintf(b, "M%d{", len(keys))
	for _, key := range keys {
		writeString(b, key)
		writeString(b, values[key])
	}
	b.WriteByte('}')
}

func writeNodes(b *strings.Builder, nodes []core.BuildNode) {
	if nodes == nil {
		b.WriteString("-")
		return
	}
	fmt.Fprintf(b, "N%d[", len(nodes))
	for _, node := range nodes {
		b.WriteString("NODE{I=")
		writeString(b, node.ID)
		b.WriteString(";S=")
		writeString(b, node.System)
		b.WriteString(";T=")
		writeString(b, node.State)
		b.WriteByte('}')
	}
	b.WriteByte(']')
}

func writeEdges(b *strings.Builder, edges []core.BuildEdge) {
	if edges == nil {
		b.WriteString("-")
		return
	}
	fmt.Fprintf(b, "G%d[", len(edges))
	for _, edge := range edges {
		b.WriteString("EDGE{F=")
		writeString(b, edge.From)
		b.WriteString(";T=")
		writeString(b, edge.To)
		b.WriteString(";K=")
		writeString(b, edge.Kind)
		b.WriteByte('}')
	}
	b.WriteByte(']')
}

func writeMonetization(b *strings.Builder, value *core.Monetization) {
	if value == nil {
		b.WriteString("-")
		return
	}
	b.WriteString("MON{M=")
	writeString(b, value.Model)
	b.WriteString(";C=")
	writeString(b, value.Customer)
	b.WriteString(";V=")
	writeString(b, value.Value)
	b.WriteString(";R=")
	writeString(b, value.RevenuePath)
	b.WriteByte('}')
}

func writeMetadata(b *strings.Builder, value *core.Metadata) {
	if value == nil {
		b.WriteString("-")
		return
	}
	b.WriteString("META{Y=")
	writeString(b, string(value.Topology))
	b.WriteString(";T=")
	writeTime(b, value.CapturedAt)
	b.WriteString(";A=")
	writeBool(b, value.AIIntegrated)
	b.WriteString(";P=")
	writeString(b, value.PromptSchema)
	b.WriteString(";F=")
	writeFacets(b, value.Facets)
	b.WriteString(";N=")
	writeNodes(b, value.BuildNodes)
	b.WriteString(";E=")
	writeEdges(b, value.BuildEdges)
	b.WriteString(";O=")
	writeMonetization(b, value.Monetization)
	if len(value.FieldObservation) > 0 {
		b.WriteString(";B=")
		writeStrings(b, value.FieldObservation)
	}
	if value.Archonym != "" {
		b.WriteString(";H=")
		writeString(b, value.Archonym)
	}
	b.WriteByte('}')
}

func writeEmergION(b *strings.Builder, value *core.EmergION) {
	if value == nil {
		b.WriteString("-")
		return
	}

	b.WriteString("EMG{IDN=")
	writeString(b, value.IDN)
	b.WriteString(";STA=")
	writeString(b, value.STA)

	b.WriteString(";MEM{H=")
	writeString(b, value.MEM.SourceHash)
	b.WriteString(";Z=")
	writeString(b, value.MEM.Codec)
	b.WriteString(";B=")
	writeInt64(b, value.MEM.Bytes)
	b.WriteString(";Q=")
	writeInt64(b, value.MEM.Stored)
	b.WriteString(";S=")
	writeString(b, value.MEM.Summary)
	b.WriteString(";P=")
	writeString(b, value.MEM.Provenance)
	b.WriteByte('}')

	b.WriteString(";REL=")
	writeMap(b, value.REL)

	b.WriteString(";CAP=")
	writeStrings(b, value.CAP)

	b.WriteString(";VAL{F=")
	writeStrings(b, value.VAL.Facts)
	b.WriteString(";G=")
	writeStrings(b, value.VAL.Gaps)
	b.WriteString(";R=")
	writeString(b, value.VAL.Risk)
	b.WriteString(";C=")
	writeBool(b, value.VAL.Recoil)
	b.WriteString(";W=")
	writeBool(b, value.VAL.WVC)
	b.WriteString(";A=")
	writeString(b, value.VAL.Reasoner)
	b.WriteString(";V=")
	writeString(b, value.VAL.ReasonerVer)
	if len(value.VAL.Differences) > 0 {
		b.WriteString(";X=")
		writeStrings(b, value.VAL.Differences)
	}
	b.WriteByte('}')

	b.WriteString(";EVO{V=")
	writeInt(b, value.EVO.Version)
	b.WriteString(";S=")
	writeString(b, value.EVO.Supersedes)
	b.WriteString(";D=")
	writeStrings(b, value.EVO.Delta)
	b.WriteString(";M=")
	writeMetadata(b, value.EVO.Metadata)
	b.WriteByte('}')

	b.WriteByte('}')
}

func writeDecision(b *strings.Builder, value *core.DecisionReceipt) {
	if value == nil {
		b.WriteString("-")
		return
	}
	b.WriteString("DEC{I=")
	writeString(b, value.EmergIONID)
	b.WriteString(";D=")
	writeString(b, value.Decision)
	b.WriteString(";A=")
	writeString(b, value.Authority)
	b.WriteString(";R=")
	writeString(b, value.Reason)
	b.WriteString(";T=")
	writeTime(b, value.At)
	b.WriteByte('}')
}

func writeREG(b *strings.Builder, value *core.REGReceipt) {
	if value == nil {
		b.WriteString("-")
		return
	}
	b.WriteString("REG{I=")
	writeString(b, value.EmergIONID)
	b.WriteString(";D=")
	writeString(b, value.DecisionID)
	b.WriteString(";T=")
	writeTime(b, value.At)
	b.WriteByte('}')
}

func writeActionAuthorization(b *strings.Builder, value *core.ActionAuthorizationReceipt) {
	if value == nil {
		b.WriteByte('-')
		return
	}

	b.WriteString("AA{I=")
	writeString(b, value.EmergIONID)
	b.WriteString(";P=")
	writeString(b, value.Adapter)
	b.WriteString(";X=")
	writeString(b, value.Action)
	b.WriteString(";A=")
	writeString(b, value.Authority)
	b.WriteString(";Z=")
	if value.Authorized {
		writeString(b, "true")
	} else {
		writeString(b, "false")
	}
	b.WriteString(";R=")
	writeString(b, value.Reason)
	b.WriteString(";T=")
	writeTime(b, value.At)
	b.WriteByte('}')
}

func native(e core.Event, includeHash bool) string {
	var b strings.Builder

	b.WriteString("EVT{T=")
	writeString(&b, e.Type)
	b.WriteString(";I=")
	writeString(&b, e.ID)
	b.WriteString(";A=")
	writeTime(&b, e.At)
	b.WriteString(";P=")
	writeString(&b, e.PrevHash)
	b.WriteString(";H=")

	if includeHash {
		writeString(&b, e.SelfHash)
	} else {
		writeString(&b, "")
	}

	b.WriteString(";E=")
	writeEmergION(&b, e.EmergION)
	b.WriteString(";D=")
	writeDecision(&b, e.Decision)
	b.WriteString(";R=")
	writeREG(&b, e.REG)
	if e.ActionAuthorization != nil {
		b.WriteString(";Q=")
		writeActionAuthorization(&b, e.ActionAuthorization)
	}
	b.WriteByte('}')

	return b.String()
}

func Seal(e core.Event) (core.Event, error) {
	e.SelfHash = ""
	sum := sha256.Sum256([]byte(native(e, false)))
	e.SelfHash = hex.EncodeToString(sum[:])
	return e, nil
}

func Encode(e core.Event) (string, error) {
	sealed, err := Seal(e)
	if err != nil {
		return "", err
	}
	return Prefix + native(sealed, true), nil
}

type parser struct {
	s string
	i int
}

func (p *parser) eof() bool {
	return p.i == len(p.s)
}

func (p *parser) take(value string) error {
	if !strings.HasPrefix(p.s[p.i:], value) {
		return fmt.Errorf("expected %q at byte %d", value, p.i)
	}
	p.i += len(value)
	return nil
}

func (p *parser) stringValue() (string, error) {
	encoded := false
	if p.i < len(p.s) && p.s[p.i] == 'B' {
		encoded = true
		p.i++
	}

	start := p.i
	for p.i < len(p.s) && p.s[p.i] >= '0' && p.s[p.i] <= '9' {
		p.i++
	}
	if start == p.i || p.i >= len(p.s) || p.s[p.i] != ':' {
		return "", fmt.Errorf("invalid length-prefixed string at byte %d", start)
	}

	n, err := strconv.Atoi(p.s[start:p.i])
	if err != nil || n < 0 {
		return "", fmt.Errorf("invalid string length")
	}
	p.i++

	if p.i+n > len(p.s) {
		return "", fmt.Errorf("truncated string")
	}

	value := p.s[p.i : p.i+n]
	p.i += n

	if !encoded {
		return value, nil
	}

	decoded, err := base64.RawStdEncoding.DecodeString(value)
	if err != nil {
		return "", fmt.Errorf("invalid line-safe string: %w", err)
	}
	return string(decoded), nil
}

func (p *parser) boolValue() (bool, error) {
	value, err := p.stringValue()
	if err != nil {
		return false, err
	}
	switch value {
	case "1":
		return true, nil
	case "0":
		return false, nil
	default:
		return false, fmt.Errorf("invalid boolean %q", value)
	}
}

func (p *parser) intValue() (int, error) {
	value, err := p.stringValue()
	if err != nil {
		return 0, err
	}
	return strconv.Atoi(value)
}

func (p *parser) int64Value() (int64, error) {
	value, err := p.stringValue()
	if err != nil {
		return 0, err
	}
	return strconv.ParseInt(value, 10, 64)
}

func (p *parser) timeValue() (time.Time, error) {
	value, err := p.stringValue()
	if err != nil {
		return time.Time{}, err
	}
	return time.Parse(time.RFC3339Nano, value)
}

func (p *parser) count(prefix byte, open byte) (int, error) {
	if p.i >= len(p.s) || p.s[p.i] != prefix {
		return 0, fmt.Errorf("expected %q at byte %d", prefix, p.i)
	}
	p.i++

	start := p.i
	for p.i < len(p.s) && p.s[p.i] >= '0' && p.s[p.i] <= '9' {
		p.i++
	}
	if start == p.i || p.i >= len(p.s) || p.s[p.i] != open {
		return 0, fmt.Errorf("invalid collection count")
	}

	n, err := strconv.Atoi(p.s[start:p.i])
	if err != nil || n < 0 {
		return 0, fmt.Errorf("invalid collection count")
	}
	p.i++
	return n, nil
}

func (p *parser) stringsValue() ([]string, error) {
	if strings.HasPrefix(p.s[p.i:], "-") {
		p.i++
		return nil, nil
	}

	n, err := p.count('L', '[')
	if err != nil {
		return nil, err
	}

	out := make([]string, 0, n)
	for i := 0; i < n; i++ {
		value, err := p.stringValue()
		if err != nil {
			return nil, err
		}
		out = append(out, value)
	}

	if err := p.take("]"); err != nil {
		return nil, err
	}
	return out, nil
}

func (p *parser) facetsValue() ([]core.Facet, error) {
	if strings.HasPrefix(p.s[p.i:], "-") {
		p.i++
		return nil, nil
	}

	n, err := p.count('F', '[')
	if err != nil {
		return nil, err
	}

	out := make([]core.Facet, 0, n)
	for i := 0; i < n; i++ {
		value, err := p.stringValue()
		if err != nil {
			return nil, err
		}
		out = append(out, core.Facet(value))
	}

	if err := p.take("]"); err != nil {
		return nil, err
	}
	return out, nil
}

func (p *parser) mapValue() (map[string]string, error) {
	if strings.HasPrefix(p.s[p.i:], "-") {
		p.i++
		return nil, nil
	}

	n, err := p.count('M', '{')
	if err != nil {
		return nil, err
	}

	out := make(map[string]string, n)
	for i := 0; i < n; i++ {
		key, err := p.stringValue()
		if err != nil {
			return nil, err
		}
		value, err := p.stringValue()
		if err != nil {
			return nil, err
		}
		if _, exists := out[key]; exists {
			return nil, fmt.Errorf("duplicate map key %q", key)
		}
		out[key] = value
	}

	if err := p.take("}"); err != nil {
		return nil, err
	}
	return out, nil
}

func (p *parser) nodesValue() ([]core.BuildNode, error) {
	if strings.HasPrefix(p.s[p.i:], "-") {
		p.i++
		return nil, nil
	}

	n, err := p.count('N', '[')
	if err != nil {
		return nil, err
	}

	out := make([]core.BuildNode, 0, n)
	for i := 0; i < n; i++ {
		if err := p.take("NODE{I="); err != nil {
			return nil, err
		}
		id, err := p.stringValue()
		if err != nil {
			return nil, err
		}
		if err := p.take(";S="); err != nil {
			return nil, err
		}
		system, err := p.stringValue()
		if err != nil {
			return nil, err
		}
		if err := p.take(";T="); err != nil {
			return nil, err
		}
		state, err := p.stringValue()
		if err != nil {
			return nil, err
		}
		if err := p.take("}"); err != nil {
			return nil, err
		}

		out = append(out, core.BuildNode{
			ID:     id,
			System: system,
			State:  state,
		})
	}

	if err := p.take("]"); err != nil {
		return nil, err
	}
	return out, nil
}

func (p *parser) edgesValue() ([]core.BuildEdge, error) {
	if strings.HasPrefix(p.s[p.i:], "-") {
		p.i++
		return nil, nil
	}

	n, err := p.count('G', '[')
	if err != nil {
		return nil, err
	}

	out := make([]core.BuildEdge, 0, n)
	for i := 0; i < n; i++ {
		if err := p.take("EDGE{F="); err != nil {
			return nil, err
		}
		from, err := p.stringValue()
		if err != nil {
			return nil, err
		}
		if err := p.take(";T="); err != nil {
			return nil, err
		}
		to, err := p.stringValue()
		if err != nil {
			return nil, err
		}
		if err := p.take(";K="); err != nil {
			return nil, err
		}
		kind, err := p.stringValue()
		if err != nil {
			return nil, err
		}
		if err := p.take("}"); err != nil {
			return nil, err
		}

		out = append(out, core.BuildEdge{
			From: from,
			To:   to,
			Kind: kind,
		})
	}

	if err := p.take("]"); err != nil {
		return nil, err
	}
	return out, nil
}

func (p *parser) monetizationValue() (*core.Monetization, error) {
	if strings.HasPrefix(p.s[p.i:], "-") {
		p.i++
		return nil, nil
	}

	if err := p.take("MON{M="); err != nil {
		return nil, err
	}
	model, err := p.stringValue()
	if err != nil {
		return nil, err
	}
	if err := p.take(";C="); err != nil {
		return nil, err
	}
	customer, err := p.stringValue()
	if err != nil {
		return nil, err
	}
	if err := p.take(";V="); err != nil {
		return nil, err
	}
	value, err := p.stringValue()
	if err != nil {
		return nil, err
	}
	if err := p.take(";R="); err != nil {
		return nil, err
	}
	revenue, err := p.stringValue()
	if err != nil {
		return nil, err
	}
	if err := p.take("}"); err != nil {
		return nil, err
	}

	return &core.Monetization{
		Model:       model,
		Customer:    customer,
		Value:       value,
		RevenuePath: revenue,
	}, nil
}

func (p *parser) metadataValue() (*core.Metadata, error) {
	if strings.HasPrefix(p.s[p.i:], "-") {
		p.i++
		return nil, nil
	}

	if err := p.take("META{Y="); err != nil {
		return nil, err
	}
	topology, err := p.stringValue()
	if err != nil {
		return nil, err
	}
	if err := p.take(";T="); err != nil {
		return nil, err
	}
	capturedAt, err := p.timeValue()
	if err != nil {
		return nil, err
	}
	if err := p.take(";A="); err != nil {
		return nil, err
	}
	ai, err := p.boolValue()
	if err != nil {
		return nil, err
	}
	if err := p.take(";P="); err != nil {
		return nil, err
	}
	prompt, err := p.stringValue()
	if err != nil {
		return nil, err
	}
	if err := p.take(";F="); err != nil {
		return nil, err
	}
	facets, err := p.facetsValue()
	if err != nil {
		return nil, err
	}
	if err := p.take(";N="); err != nil {
		return nil, err
	}
	nodes, err := p.nodesValue()
	if err != nil {
		return nil, err
	}
	if err := p.take(";E="); err != nil {
		return nil, err
	}
	edges, err := p.edgesValue()
	if err != nil {
		return nil, err
	}
	if err := p.take(";O="); err != nil {
		return nil, err
	}
	monetization, err := p.monetizationValue()
	if err != nil {
		return nil, err
	}
	var fieldObservation []string
	if strings.HasPrefix(p.s[p.i:], ";B=") {
		if err := p.take(";B="); err != nil {
			return nil, err
		}
		fieldObservation, err = p.stringsValue()
		if err != nil {
			return nil, err
		}
	}
	var archonym string
	if strings.HasPrefix(p.s[p.i:], ";H=") {
		if err := p.take(";H="); err != nil {
			return nil, err
		}
		archonym, err = p.stringValue()
		if err != nil {
			return nil, err
		}
	}
	if err := p.take("}"); err != nil {
		return nil, err
	}

	out := &core.Metadata{
		Topology:         core.Topology(topology),
		CapturedAt:       capturedAt,
		AIIntegrated:     ai,
		PromptSchema:     prompt,
		Archonym:         archonym,
		Facets:           facets,
		BuildNodes:       nodes,
		BuildEdges:       edges,
		Monetization:     monetization,
		FieldObservation: fieldObservation,
	}

	if err := out.Validate(); err != nil {
		return nil, err
	}
	return out, nil
}

func (p *parser) emergionValue() (*core.EmergION, error) {
	if strings.HasPrefix(p.s[p.i:], "-") {
		p.i++
		return nil, nil
	}

	if err := p.take("EMG{IDN="); err != nil {
		return nil, err
	}
	idn, err := p.stringValue()
	if err != nil {
		return nil, err
	}
	if err := p.take(";STA="); err != nil {
		return nil, err
	}
	sta, err := p.stringValue()
	if err != nil {
		return nil, err
	}

	if err := p.take(";MEM{H="); err != nil {
		return nil, err
	}
	sourceHash, err := p.stringValue()
	if err != nil {
		return nil, err
	}
	if err := p.take(";Z="); err != nil {
		return nil, err
	}
	codec, err := p.stringValue()
	if err != nil {
		return nil, err
	}
	if err := p.take(";B="); err != nil {
		return nil, err
	}
	bytesValue, err := p.int64Value()
	if err != nil {
		return nil, err
	}
	if err := p.take(";Q="); err != nil {
		return nil, err
	}
	stored, err := p.int64Value()
	if err != nil {
		return nil, err
	}
	if err := p.take(";S="); err != nil {
		return nil, err
	}
	summary, err := p.stringValue()
	if err != nil {
		return nil, err
	}
	if err := p.take(";P="); err != nil {
		return nil, err
	}
	provenance, err := p.stringValue()
	if err != nil {
		return nil, err
	}
	if err := p.take("}"); err != nil {
		return nil, err
	}

	if err := p.take(";REL="); err != nil {
		return nil, err
	}
	relationships, err := p.mapValue()
	if err != nil {
		return nil, err
	}

	if err := p.take(";CAP="); err != nil {
		return nil, err
	}
	capabilities, err := p.stringsValue()
	if err != nil {
		return nil, err
	}

	if err := p.take(";VAL{F="); err != nil {
		return nil, err
	}
	facts, err := p.stringsValue()
	if err != nil {
		return nil, err
	}
	if err := p.take(";G="); err != nil {
		return nil, err
	}
	gaps, err := p.stringsValue()
	if err != nil {
		return nil, err
	}
	if err := p.take(";R="); err != nil {
		return nil, err
	}
	risk, err := p.stringValue()
	if err != nil {
		return nil, err
	}
	if err := p.take(";C="); err != nil {
		return nil, err
	}
	recoil, err := p.boolValue()
	if err != nil {
		return nil, err
	}
	if err := p.take(";W="); err != nil {
		return nil, err
	}
	wvc, err := p.boolValue()
	if err != nil {
		return nil, err
	}
	if err := p.take(";A="); err != nil {
		return nil, err
	}
	reasoner, err := p.stringValue()
	if err != nil {
		return nil, err
	}
	if err := p.take(";V="); err != nil {
		return nil, err
	}
	reasonerVer, err := p.stringValue()
	if err != nil {
		return nil, err
	}
	var differences []string
	if strings.HasPrefix(p.s[p.i:], ";X=") {
		if err := p.take(";X="); err != nil {
			return nil, err
		}
		differences, err = p.stringsValue()
		if err != nil {
			return nil, err
		}
	}
	if err := p.take("}"); err != nil {
		return nil, err
	}

	if err := p.take(";EVO{V="); err != nil {
		return nil, err
	}
	version, err := p.intValue()
	if err != nil {
		return nil, err
	}
	if err := p.take(";S="); err != nil {
		return nil, err
	}
	supersedes, err := p.stringValue()
	if err != nil {
		return nil, err
	}
	if err := p.take(";D="); err != nil {
		return nil, err
	}
	delta, err := p.stringsValue()
	if err != nil {
		return nil, err
	}
	if err := p.take(";M="); err != nil {
		return nil, err
	}
	metadata, err := p.metadataValue()
	if err != nil {
		return nil, err
	}
	if err := p.take("}"); err != nil {
		return nil, err
	}

	if err := p.take("}"); err != nil {
		return nil, err
	}

	return &core.EmergION{
		IDN: idn,
		STA: sta,
		MEM: core.Memory{
			SourceHash: sourceHash,
			Codec:      codec,
			Bytes:      bytesValue,
			Stored:     stored,
			Summary:    summary,
			Provenance: provenance,
		},
		REL: relationships,
		CAP: capabilities,
		VAL: core.Validation{
			Facts:       facts,
			Differences: differences,
			Gaps:        gaps,
			Risk:        risk,
			Recoil:      recoil,
			WVC:         wvc,
			Reasoner:    reasoner,
			ReasonerVer: reasonerVer,
		},
		EVO: core.Evolution{
			Version:    version,
			Supersedes: supersedes,
			Delta:      delta,
			Metadata:   metadata,
		},
	}, nil
}

func (p *parser) decisionValue() (*core.DecisionReceipt, error) {
	if strings.HasPrefix(p.s[p.i:], "-") {
		p.i++
		return nil, nil
	}

	if err := p.take("DEC{I="); err != nil {
		return nil, err
	}
	id, err := p.stringValue()
	if err != nil {
		return nil, err
	}
	if err := p.take(";D="); err != nil {
		return nil, err
	}
	decision, err := p.stringValue()
	if err != nil {
		return nil, err
	}
	if err := p.take(";A="); err != nil {
		return nil, err
	}
	authority, err := p.stringValue()
	if err != nil {
		return nil, err
	}
	if err := p.take(";R="); err != nil {
		return nil, err
	}
	reason, err := p.stringValue()
	if err != nil {
		return nil, err
	}
	if err := p.take(";T="); err != nil {
		return nil, err
	}
	at, err := p.timeValue()
	if err != nil {
		return nil, err
	}
	if err := p.take("}"); err != nil {
		return nil, err
	}

	return &core.DecisionReceipt{
		EmergIONID: id,
		Decision:   decision,
		Authority:  authority,
		Reason:     reason,
		At:         at,
	}, nil
}

func (p *parser) regValue() (*core.REGReceipt, error) {
	if strings.HasPrefix(p.s[p.i:], "-") {
		p.i++
		return nil, nil
	}

	if err := p.take("REG{I="); err != nil {
		return nil, err
	}
	id, err := p.stringValue()
	if err != nil {
		return nil, err
	}
	if err := p.take(";D="); err != nil {
		return nil, err
	}
	decisionID, err := p.stringValue()
	if err != nil {
		return nil, err
	}
	if err := p.take(";T="); err != nil {
		return nil, err
	}
	at, err := p.timeValue()
	if err != nil {
		return nil, err
	}
	if err := p.take("}"); err != nil {
		return nil, err
	}

	return &core.REGReceipt{
		EmergIONID: id,
		DecisionID: decisionID,
		At:         at,
	}, nil
}

func (p *parser) actionAuthorizationValue() (*core.ActionAuthorizationReceipt, error) {
	if strings.HasPrefix(p.s[p.i:], "-") {
		p.i++
		return nil, nil
	}

	if err := p.take("AA{I="); err != nil {
		return nil, err
	}
	id, err := p.stringValue()
	if err != nil {
		return nil, err
	}
	if err := p.take(";P="); err != nil {
		return nil, err
	}
	adapter, err := p.stringValue()
	if err != nil {
		return nil, err
	}
	if err := p.take(";X="); err != nil {
		return nil, err
	}
	action, err := p.stringValue()
	if err != nil {
		return nil, err
	}
	if err := p.take(";A="); err != nil {
		return nil, err
	}
	authority, err := p.stringValue()
	if err != nil {
		return nil, err
	}
	if err := p.take(";Z="); err != nil {
		return nil, err
	}
	authorizedValue, err := p.stringValue()
	if err != nil {
		return nil, err
	}
	var authorized bool
	switch authorizedValue {
	case "true":
		authorized = true
	case "false":
		authorized = false
	default:
		return nil, fmt.Errorf("invalid action authorization value %q", authorizedValue)
	}
	if err := p.take(";R="); err != nil {
		return nil, err
	}
	reason, err := p.stringValue()
	if err != nil {
		return nil, err
	}
	if err := p.take(";T="); err != nil {
		return nil, err
	}
	at, err := p.timeValue()
	if err != nil {
		return nil, err
	}
	if err := p.take("}"); err != nil {
		return nil, err
	}

	return &core.ActionAuthorizationReceipt{
		EmergIONID: id,
		Adapter:    adapter,
		Action:     action,
		Authority:  authority,
		Authorized: authorized,
		Reason:     reason,
		At:         at,
	}, nil
}

func parseNative(value string) (core.Event, error) {
	p := &parser{s: value}

	if err := p.take("EVT{T="); err != nil {
		return core.Event{}, err
	}
	eventType, err := p.stringValue()
	if err != nil {
		return core.Event{}, err
	}
	if err := p.take(";I="); err != nil {
		return core.Event{}, err
	}
	id, err := p.stringValue()
	if err != nil {
		return core.Event{}, err
	}
	if err := p.take(";A="); err != nil {
		return core.Event{}, err
	}
	at, err := p.timeValue()
	if err != nil {
		return core.Event{}, err
	}
	if err := p.take(";P="); err != nil {
		return core.Event{}, err
	}
	prevHash, err := p.stringValue()
	if err != nil {
		return core.Event{}, err
	}
	if err := p.take(";H="); err != nil {
		return core.Event{}, err
	}
	selfHash, err := p.stringValue()
	if err != nil {
		return core.Event{}, err
	}
	if err := p.take(";E="); err != nil {
		return core.Event{}, err
	}
	emergion, err := p.emergionValue()
	if err != nil {
		return core.Event{}, err
	}
	if err := p.take(";D="); err != nil {
		return core.Event{}, err
	}
	decision, err := p.decisionValue()
	if err != nil {
		return core.Event{}, err
	}
	if err := p.take(";R="); err != nil {
		return core.Event{}, err
	}
	reg, err := p.regValue()
	if err != nil {
		return core.Event{}, err
	}
	var actionAuthorization *core.ActionAuthorizationReceipt
	if strings.HasPrefix(p.s[p.i:], ";Q=") {
		if err := p.take(";Q="); err != nil {
			return core.Event{}, err
		}
		actionAuthorization, err = p.actionAuthorizationValue()
		if err != nil {
			return core.Event{}, err
		}
	}
	if err := p.take("}"); err != nil {
		return core.Event{}, err
	}
	if !p.eof() {
		return core.Event{}, fmt.Errorf("trailing COSL data")
	}

	return core.Event{
		Type:                eventType,
		ID:                  id,
		At:                  at,
		EmergION:            emergion,
		Decision:            decision,
		REG:                 reg,
		ActionAuthorization: actionAuthorization,
		PrevHash:            prevHash,
		SelfHash:            selfHash,
	}, nil
}

func decodeTransitional(line string) (core.Event, error) {
	if !strings.HasPrefix(line, EventPrefix) || !strings.HasSuffix(line, "}") {
		return core.Event{}, fmt.Errorf("invalid transitional COSL record")
	}

	body := strings.TrimSuffix(strings.TrimPrefix(line, EventPrefix), "}")

	fields := map[string]string{}
	for _, part := range strings.Split(body, ";") {
		key, value, ok := strings.Cut(part, ":")
		if !ok || key == "" {
			return core.Event{}, fmt.Errorf("invalid transitional COSL field")
		}
		fields[key] = value
	}

	raw, err := base64.RawURLEncoding.DecodeString(fields["D"])
	if err != nil {
		return core.Event{}, fmt.Errorf("invalid transitional COSL payload: %w", err)
	}

	var e core.Event
	if err := json.Unmarshal(raw, &e); err != nil {
		return core.Event{}, fmt.Errorf("invalid transitional COSL payload: %w", err)
	}

	if fields["T"] != e.Type ||
		fields["I"] != e.ID ||
		fields["P"] != e.PrevHash ||
		fields["H"] != e.SelfHash {
		return core.Event{}, fmt.Errorf("transitional COSL envelope mismatch")
	}

	claimed := e.SelfHash
	sealed, err := legacySeal(e)
	if err != nil {
		return core.Event{}, err
	}
	if claimed == "" || claimed != sealed.SelfHash {
		return core.Event{}, fmt.Errorf("event hash mismatch")
	}

	return e, nil
}

func Decode(line string) (core.Event, error) {
	line = strings.TrimSpace(line)

	if strings.HasPrefix(line, NativeEventPrefix) {
		e, err := parseNative(strings.TrimPrefix(line, Prefix))
		if err != nil {
			return core.Event{}, err
		}

		claimed := e.SelfHash
		sealed, err := Seal(e)
		if err != nil {
			return core.Event{}, err
		}
		if claimed == "" || claimed != sealed.SelfHash {
			return core.Event{}, fmt.Errorf("event hash mismatch")
		}

		return e, nil
	}

	if strings.HasPrefix(line, LegacyEventPrefix) {
		return decodeTransitional(line)
	}

	if strings.HasPrefix(line, Prefix) {
		var e core.Event
		if err := json.Unmarshal(
			[]byte(strings.TrimPrefix(line, Prefix)),
			&e,
		); err != nil {
			return core.Event{}, err
		}

		claimed := e.SelfHash
		sealed, err := legacySeal(e)
		if err != nil {
			return core.Event{}, err
		}
		if claimed == "" || claimed != sealed.SelfHash {
			return core.Event{}, fmt.Errorf("event hash mismatch")
		}

		return e, nil
	}

	return core.Event{}, fmt.Errorf("invalid COSL prefix")
}
