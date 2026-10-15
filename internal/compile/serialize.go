package compile

import (
	"encoding/json"
	"strings"

	"iacpack/internal/canonicaljson"
	"iacpack/internal/domain/ir"
	"iacpack/internal/domain/plan"
)

// MarshalDeploymentCanonical returns RFC 8785 deployment JSON + newline.
func MarshalDeploymentCanonical(d ir.Deployment) ([]byte, error) {
	raw, err := json.Marshal(d)
	if err != nil {
		return nil, err
	}
	var m map[string]any
	dec := json.NewDecoder(strings.NewReader(string(raw)))
	dec.UseNumber()
	if err := dec.Decode(&m); err != nil {
		return nil, err
	}
	canon, err := canonicaljson.Marshal(m)
	if err != nil {
		return nil, err
	}
	return append(canon, '\n'), nil
}

// GraphInspectModel is the machine view for inspect graph.
func GraphInspectModel(d ir.Deployment, g plan.Graph) map[string]any {
	contentHash, _ := graphContentHash(g)
	return map[string]any{
		"deploymentHash": g.DeploymentHash,
		"graphHash":      contentHash,
		"project": map[string]any{
			"name":    d.Project.Name,
			"version": d.Project.Version,
		},
		"actions": g.Actions,
		"edges":   g.Edges,
		"order":   g.Order,
	}
}

// FormatGraphText renders a stable text view.
func FormatGraphText(d ir.Deployment, g plan.Graph) string {
	var b strings.Builder
	b.WriteString("project: " + d.Project.Name + "@" + d.Project.Version + "\n")
	b.WriteString("deploymentHash: " + g.DeploymentHash + "\n")
	b.WriteString("actions: " + itoa(len(g.Actions)) + "\n")
	b.WriteString("edges: " + itoa(len(g.Edges)) + "\n")
	b.WriteString("order:\n")
	for _, id := range g.Order {
		b.WriteString("  - " + id + "\n")
	}
	b.WriteString("edgesDetail:\n")
	for _, e := range g.Edges {
		line := "  " + e.From + " -> " + e.To
		if e.Condition != "" {
			line += " [" + e.Condition + "]"
		}
		b.WriteString(line + "\n")
	}
	return b.String()
}

// FormatGraphMermaid renders a deterministic flowchart with escaped labels.
func FormatGraphMermaid(g plan.Graph) string {
	var b strings.Builder
	b.WriteString("flowchart TD\n")
	for _, a := range g.Actions {
		nid := mermaidNodeID(a.ID)
		b.WriteString("  " + nid + "[\"" + escapeMermaid(a.Label) + "\"]\n")
	}
	for _, e := range g.Edges {
		from := mermaidNodeID(e.From)
		to := mermaidNodeID(e.To)
		if e.Condition != "" {
			b.WriteString("  " + from + " -->|" + escapeMermaid(e.Condition) + "| " + to + "\n")
		} else {
			b.WriteString("  " + from + " --> " + to + "\n")
		}
	}
	return b.String()
}

func mermaidNodeID(actionID string) string {
	var b strings.Builder
	b.WriteByte('n')
	for _, r := range actionID {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9':
			b.WriteRune(r)
		default:
			b.WriteByte('_')
		}
	}
	return b.String()
}

func escapeMermaid(s string) string {
	s = strings.ReplaceAll(s, `\`, `\\`)
	s = strings.ReplaceAll(s, `"`, `#quot;`)
	s = strings.ReplaceAll(s, "\n", " ")
	s = strings.ReplaceAll(s, "]", "")
	s = strings.ReplaceAll(s, "[", "")
	s = strings.ReplaceAll(s, "|", "/")
	return s
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var buf [20]byte
	i := len(buf)
	for n > 0 {
		i--
		buf[i] = byte('0' + n%10)
		n /= 10
	}
	return string(buf[i:])
}
