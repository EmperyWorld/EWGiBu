package loader

import (
	"fmt"
	"unicode/utf8"

	"iacpack/internal/diagnostic"
)

func parseJSON(file string, data []byte) Document {
	p := &jsonParser{file: file, data: data}
	p.skipWS()
	if p.pos >= len(p.data) {
		p.add(diagAt(file, diagnostic.CodeManifestParseError, "empty document", 1, 1, ""))
		return Document{File: file, Diags: p.diags}
	}
	root, ok := p.parseValue(0, "")
	if !ok {
		return Document{File: file, Diags: p.diags}
	}
	p.skipWS()
	if p.pos < len(p.data) {
		line, col := p.lineCol(p.pos)
		p.add(diagAt(file, diagnostic.CodeManifestParseError, "unexpected trailing content after JSON value", line, col, ""))
		return Document{File: file, Diags: p.diags}
	}
	if len(p.diags) > 0 {
		return Document{File: file, Diags: p.diags}
	}
	return Document{Root: root, File: file}
}

type jsonParser struct {
	file  string
	data  []byte
	pos   int
	diags []diagnostic.Diagnostic
}

func (p *jsonParser) add(d diagnostic.Diagnostic) {
	p.diags = append(p.diags, d)
}

func (p *jsonParser) lineCol(pos int) (line, col int) {
	line, col = 1, 1
	for i := 0; i < pos && i < len(p.data); i++ {
		if p.data[i] == '\n' {
			line++
			col = 1
		} else {
			col++
		}
	}
	return line, col
}

func (p *jsonParser) skipWS() {
	for p.pos < len(p.data) {
		c := p.data[p.pos]
		if c == ' ' || c == '\t' || c == '\n' || c == '\r' {
			p.pos++
			continue
		}
		return
	}
}

func (p *jsonParser) parseValue(depth int, pointer string) (*Node, bool) {
	if depth > MaxDepth {
		line, col := p.lineCol(p.pos)
		p.add(diagAt(p.file, diagnostic.CodeManifestTooDeep,
			fmt.Sprintf("manifest exceeds maximum depth of %d", MaxDepth), line, col, pointer))
		return nil, false
	}
	p.skipWS()
	if p.pos >= len(p.data) {
		line, col := p.lineCol(p.pos)
		p.add(diagAt(p.file, diagnostic.CodeManifestParseError, "unexpected end of JSON", line, col, pointer))
		return nil, false
	}
	line, col := p.lineCol(p.pos)
	switch p.data[p.pos] {
	case '{':
		return p.parseObject(depth, pointer)
	case '[':
		return p.parseArray(depth, pointer)
	case '"':
		s, ok := p.parseString()
		if !ok {
			return nil, false
		}
		return &Node{Kind: KindString, Str: s, Line: line, Column: col, File: p.file}, true
	case 't':
		if !p.consumeLiteral("true") {
			p.add(diagAt(p.file, diagnostic.CodeManifestParseError, "invalid JSON literal", line, col, pointer))
			return nil, false
		}
		return &Node{Kind: KindBool, Bool: true, Line: line, Column: col, File: p.file}, true
	case 'f':
		if !p.consumeLiteral("false") {
			p.add(diagAt(p.file, diagnostic.CodeManifestParseError, "invalid JSON literal", line, col, pointer))
			return nil, false
		}
		return &Node{Kind: KindBool, Bool: false, Line: line, Column: col, File: p.file}, true
	case 'n':
		if !p.consumeLiteral("null") {
			p.add(diagAt(p.file, diagnostic.CodeManifestParseError, "invalid JSON literal", line, col, pointer))
			return nil, false
		}
		return &Node{Kind: KindNull, Line: line, Column: col, File: p.file}, true
	default:
		if p.data[p.pos] == '-' || (p.data[p.pos] >= '0' && p.data[p.pos] <= '9') {
			s, ok := p.parseNumber()
			if !ok {
				return nil, false
			}
			return &Node{Kind: KindNumber, Str: s, Line: line, Column: col, File: p.file}, true
		}
		p.add(diagAt(p.file, diagnostic.CodeManifestParseError, "invalid JSON value", line, col, pointer))
		return nil, false
	}
}

func (p *jsonParser) parseObject(depth int, pointer string) (*Node, bool) {
	line, col := p.lineCol(p.pos)
	p.pos++ // {
	node := &Node{Kind: KindMap, Line: line, Column: col, File: p.file}
	seen := map[string]struct{}{}
	p.skipWS()
	if p.pos < len(p.data) && p.data[p.pos] == '}' {
		p.pos++
		return node, true
	}
	for {
		p.skipWS()
		if p.pos >= len(p.data) || p.data[p.pos] != '"' {
			line, col := p.lineCol(p.pos)
			p.add(diagAt(p.file, diagnostic.CodeManifestParseError, "expected string key in JSON object", line, col, pointer))
			return nil, false
		}
		keyLine, keyCol := p.lineCol(p.pos)
		key, ok := p.parseString()
		if !ok {
			return nil, false
		}
		if !utf8.ValidString(key) {
			p.add(diagAt(p.file, diagnostic.CodeManifestNonStringKey, "map keys must be strings", keyLine, keyCol, pointer))
			return nil, false
		}
		if _, exists := seen[key]; exists {
			p.add(diagAt(p.file, diagnostic.CodeManifestDuplicateKey,
				fmt.Sprintf("duplicate key %q", key), keyLine, keyCol, joinPointer(pointer, key)).
				WithHint("Remove or rename the duplicate key"))
			return nil, false
		}
		seen[key] = struct{}{}
		p.skipWS()
		if p.pos >= len(p.data) || p.data[p.pos] != ':' {
			line, col := p.lineCol(p.pos)
			p.add(diagAt(p.file, diagnostic.CodeManifestParseError, "expected ':' after JSON object key", line, col, pointer))
			return nil, false
		}
		p.pos++
		childPtr := joinPointer(pointer, key)
		val, ok := p.parseValue(depth+1, childPtr)
		if !ok {
			return nil, false
		}
		node.Map = append(node.Map, MapEntry{
			Key:       key,
			KeyLine:   keyLine,
			KeyColumn: keyCol,
			Value:     val,
		})
		p.skipWS()
		if p.pos >= len(p.data) {
			line, col := p.lineCol(p.pos)
			p.add(diagAt(p.file, diagnostic.CodeManifestParseError, "unterminated JSON object", line, col, pointer))
			return nil, false
		}
		if p.data[p.pos] == ',' {
			p.pos++
			continue
		}
		if p.data[p.pos] == '}' {
			p.pos++
			return node, true
		}
		line, col := p.lineCol(p.pos)
		p.add(diagAt(p.file, diagnostic.CodeManifestParseError, "expected ',' or '}' in JSON object", line, col, pointer))
		return nil, false
	}
}

func (p *jsonParser) parseArray(depth int, pointer string) (*Node, bool) {
	line, col := p.lineCol(p.pos)
	p.pos++ // [
	node := &Node{Kind: KindSeq, Line: line, Column: col, File: p.file}
	p.skipWS()
	if p.pos < len(p.data) && p.data[p.pos] == ']' {
		p.pos++
		return node, true
	}
	index := 0
	for {
		childPtr := fmt.Sprintf("%s/%d", pointer, index)
		if pointer == "" {
			childPtr = fmt.Sprintf("/%d", index)
		}
		val, ok := p.parseValue(depth+1, childPtr)
		if !ok {
			return nil, false
		}
		node.Seq = append(node.Seq, val)
		index++
		p.skipWS()
		if p.pos >= len(p.data) {
			line, col := p.lineCol(p.pos)
			p.add(diagAt(p.file, diagnostic.CodeManifestParseError, "unterminated JSON array", line, col, pointer))
			return nil, false
		}
		if p.data[p.pos] == ',' {
			p.pos++
			continue
		}
		if p.data[p.pos] == ']' {
			p.pos++
			return node, true
		}
		line, col := p.lineCol(p.pos)
		p.add(diagAt(p.file, diagnostic.CodeManifestParseError, "expected ',' or ']' in JSON array", line, col, pointer))
		return nil, false
	}
}

func (p *jsonParser) parseString() (string, bool) {
	startLine, startCol := p.lineCol(p.pos)
	p.pos++ // "
	var b []byte
	for p.pos < len(p.data) {
		c := p.data[p.pos]
		if c == '"' {
			p.pos++
			return string(b), true
		}
		if c == '\\' {
			esc, n, ok := decodeJSONEscape(p.data[p.pos:])
			if !ok {
				line, col := p.lineCol(p.pos)
				p.add(diagAt(p.file, diagnostic.CodeManifestParseError, "invalid JSON escape sequence", line, col, ""))
				return "", false
			}
			b = append(b, esc...)
			p.pos += n
			continue
		}
		if c < 0x20 {
			line, col := p.lineCol(p.pos)
			p.add(diagAt(p.file, diagnostic.CodeManifestParseError, "invalid control character in JSON string", line, col, ""))
			return "", false
		}
		b = append(b, c)
		p.pos++
	}
	p.add(diagAt(p.file, diagnostic.CodeManifestParseError, "unterminated JSON string", startLine, startCol, ""))
	return "", false
}

func (p *jsonParser) parseNumber() (string, bool) {
	start := p.pos
	line, col := p.lineCol(p.pos)
	if p.data[p.pos] == '-' {
		p.pos++
	}
	if p.pos >= len(p.data) {
		p.add(diagAt(p.file, diagnostic.CodeManifestParseError, "invalid JSON number", line, col, ""))
		return "", false
	}
	if p.data[p.pos] == '0' {
		p.pos++
	} else if p.data[p.pos] >= '1' && p.data[p.pos] <= '9' {
		for p.pos < len(p.data) && p.data[p.pos] >= '0' && p.data[p.pos] <= '9' {
			p.pos++
		}
	} else {
		p.add(diagAt(p.file, diagnostic.CodeManifestParseError, "invalid JSON number", line, col, ""))
		return "", false
	}
	if p.pos < len(p.data) && p.data[p.pos] == '.' {
		p.pos++
		if p.pos >= len(p.data) || p.data[p.pos] < '0' || p.data[p.pos] > '9' {
			p.add(diagAt(p.file, diagnostic.CodeManifestParseError, "invalid JSON number", line, col, ""))
			return "", false
		}
		for p.pos < len(p.data) && p.data[p.pos] >= '0' && p.data[p.pos] <= '9' {
			p.pos++
		}
	}
	if p.pos < len(p.data) && (p.data[p.pos] == 'e' || p.data[p.pos] == 'E') {
		p.pos++
		if p.pos < len(p.data) && (p.data[p.pos] == '+' || p.data[p.pos] == '-') {
			p.pos++
		}
		if p.pos >= len(p.data) || p.data[p.pos] < '0' || p.data[p.pos] > '9' {
			p.add(diagAt(p.file, diagnostic.CodeManifestParseError, "invalid JSON number", line, col, ""))
			return "", false
		}
		for p.pos < len(p.data) && p.data[p.pos] >= '0' && p.data[p.pos] <= '9' {
			p.pos++
		}
	}
	return string(p.data[start:p.pos]), true
}

func (p *jsonParser) consumeLiteral(lit string) bool {
	if p.pos+len(lit) > len(p.data) {
		return false
	}
	if string(p.data[p.pos:p.pos+len(lit)]) != lit {
		return false
	}
	p.pos += len(lit)
	return true
}
