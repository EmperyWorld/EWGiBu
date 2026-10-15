package loader

import (
	"fmt"

	"iacpack/internal/diagnostic"
)

const (
	// MaxBytes is the default maximum manifest size (2 MiB).
	MaxBytes = 2 << 20
	// MaxDepth is the maximum nesting depth.
	MaxDepth = 100
)

// Kind classifies a decoded node.
type Kind int

const (
	KindNull Kind = iota
	KindBool
	KindNumber
	KindString
	KindMap
	KindSeq
)

// MapEntry is one ordered string-keyed map entry.
type MapEntry struct {
	Key       string
	KeyLine   int
	KeyColumn int
	Value     *Node
}

// Node is a location-preserving generic value.
type Node struct {
	Kind   Kind
	Str    string // KindString or KindNumber (lexical form)
	Bool   bool
	Map    []MapEntry
	Seq    []*Node
	Line   int
	Column int
	File   string
	// Tag is a Compose merge tag (override or reset) when parsed from Compose input.
	Tag string
}

// Document is a single parsed document.
type Document struct {
	Root  *Node
	File  string
	Diags []diagnostic.Diagnostic
}

// Parse detects JSON vs YAML and parses exactly one document.
func Parse(file string, data []byte) Document {
	if len(data) > MaxBytes {
		return Document{
			File: file,
			Diags: []diagnostic.Diagnostic{
				diagnostic.Error(diagnostic.CodeManifestTooLarge,
					fmt.Sprintf("manifest exceeds maximum size of %d bytes", MaxBytes)).
					WithSource(file, "", 1, 1),
			},
		}
	}
	if !isValidUTF8(data) {
		return Document{
			File: file,
			Diags: []diagnostic.Diagnostic{
				diagnostic.Error(diagnostic.CodeManifestParseError, "manifest is not valid UTF-8").
					WithSource(file, "", 1, 1),
			},
		}
	}
	trimmed := trimBOM(data)
	if looksLikeJSON(trimmed) {
		return parseJSON(file, trimmed)
	}
	return parseYAML(file, trimmed)
}

// ParseComposeInput parses one Compose YAML document for import propose.
// Flow-style sequences and maps (common in Docker Compose) are accepted; anchors and aliases are not.
func ParseComposeInput(file string, data []byte) Document {
	if len(data) > MaxBytes {
		return Document{
			File: file,
			Diags: []diagnostic.Diagnostic{
				diagnostic.Error(diagnostic.CodeManifestTooLarge,
					fmt.Sprintf("document exceeds maximum size of %d bytes", MaxBytes)).
					WithSource(file, "", 1, 1),
			},
		}
	}
	if !isValidUTF8(data) {
		return Document{
			File: file,
			Diags: []diagnostic.Diagnostic{
				diagnostic.Error(diagnostic.CodeManifestParseError, "document is not valid UTF-8").
					WithSource(file, "", 1, 1),
			},
		}
	}
	trimmed := trimBOM(data)
	if looksLikeJSON(trimmed) {
		return parseJSON(file, trimmed)
	}
	return parseYAMLCompose(file, trimmed)
}

func parseYAMLCompose(file string, data []byte) Document {
	p := &yamlParser{file: file, data: data, allowFlowCollections: true, allowComposeMergeTags: true}
	p.skipBlankAndComments()
	if p.pos >= len(p.data) {
		p.add(diagAt(file, diagnostic.CodeManifestParseError, "empty document", 1, 1, ""))
		return Document{File: file, Diags: p.diags}
	}
	if p.atDocStart() {
		p.pos += 3
		p.skipSpacesOnly()
		if p.pos < len(p.data) && (p.data[p.pos] == '\n' || p.data[p.pos] == '\r') {
			p.consumeNewline()
		} else if p.pos < len(p.data) && p.data[p.pos] != '#' {
			line, col := p.lineCol(p.pos)
			p.add(diagAt(file, diagnostic.CodeManifestParseError, "unexpected content after document start marker", line, col, ""))
			return Document{File: file, Diags: p.diags}
		}
		p.skipBlankAndComments()
	}
	root, ok := p.parseBlockNode(0, 0, "")
	if !ok {
		return Document{File: file, Diags: p.diags}
	}
	p.skipBlankAndComments()
	if p.pos < len(p.data) {
		if p.atDocStart() || p.atDocEnd() {
			line, col := p.lineCol(p.pos)
			p.add(diagAt(file, diagnostic.CodeManifestMultipleDocuments, "multiple YAML documents are forbidden", line, col, ""))
			return Document{File: file, Diags: p.diags}
		}
		line, col := p.lineCol(p.pos)
		p.add(diagAt(file, diagnostic.CodeManifestParseError, "unexpected trailing content after document", line, col, ""))
		return Document{File: file, Diags: p.diags}
	}
	if len(p.diags) > 0 {
		return Document{File: file, Diags: p.diags}
	}
	return Document{Root: root, File: file, Diags: nil}
}

func trimBOM(data []byte) []byte {
	if len(data) >= 3 && data[0] == 0xEF && data[1] == 0xBB && data[2] == 0xBF {
		return data[3:]
	}
	return data
}

func looksLikeJSON(data []byte) bool {
	for i := 0; i < len(data); i++ {
		c := data[i]
		if c == ' ' || c == '\t' || c == '\n' || c == '\r' {
			continue
		}
		return c == '{' || c == '['
	}
	return false
}

func isValidUTF8(data []byte) bool {
	// Inline to avoid importing unicode/utf8 in this file only — use stdlib.
	for i := 0; i < len(data); {
		if data[i] < 0x80 {
			i++
			continue
		}
		// Delegate remaining check via std helper in utf8util.go
		return validUTF8(data)
	}
	return true
}

// GetMap returns the map entry value for key, or nil.
func (n *Node) GetMap(key string) *Node {
	if n == nil || n.Kind != KindMap {
		return nil
	}
	for i := range n.Map {
		if n.Map[i].Key == key {
			return n.Map[i].Value
		}
	}
	return nil
}

// MapKeys returns ordered keys.
func (n *Node) MapKeys() []string {
	if n == nil || n.Kind != KindMap {
		return nil
	}
	keys := make([]string, len(n.Map))
	for i, e := range n.Map {
		keys[i] = e.Key
	}
	return keys
}

// AsCanonical returns a comparable generic Go value.
func (n *Node) AsCanonical() any {
	if n == nil {
		return nil
	}
	switch n.Kind {
	case KindNull:
		return nil
	case KindBool:
		return n.Bool
	case KindNumber:
		return n.Str
	case KindString:
		return n.Str
	case KindMap:
		m := make(map[string]any, len(n.Map))
		for _, e := range n.Map {
			m[e.Key] = e.Value.AsCanonical()
		}
		return m
	case KindSeq:
		s := make([]any, len(n.Seq))
		for i, c := range n.Seq {
			s[i] = c.AsCanonical()
		}
		return s
	default:
		return nil
	}
}

func diagAt(file, code, msg string, line, col int, pointer string) diagnostic.Diagnostic {
	return diagnostic.Error(code, msg).WithSource(file, pointer, line, col)
}

func joinPointer(base, key string) string {
	esc := escapePointer(key)
	if base == "" {
		return "/" + esc
	}
	return base + "/" + esc
}

func escapePointer(s string) string {
	r := make([]byte, 0, len(s)+4)
	for i := 0; i < len(s); i++ {
		switch s[i] {
		case '~':
			r = append(r, '~', '0')
		case '/':
			r = append(r, '~', '1')
		default:
			r = append(r, s[i])
		}
	}
	return string(r)
}
