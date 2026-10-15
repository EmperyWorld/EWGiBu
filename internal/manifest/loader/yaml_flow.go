package loader

import "iacpack/internal/diagnostic"

// parseFlowNode parses a flow-style or scalar value inside [...] or {...}.
func (p *yamlParser) parseFlowNode(depth int, pointer string) (*Node, bool) {
	if depth > MaxDepth {
		line, col := p.lineCol(p.pos)
		p.add(diagAt(p.file, diagnostic.CodeManifestTooDeep,
			"document exceeds maximum depth", line, col, pointer))
		return nil, false
	}
	p.skipFlowSpaces()
	if p.pos >= len(p.data) {
		line, col := p.lineCol(p.pos)
		p.add(diagAt(p.file, diagnostic.CodeManifestParseError, "unexpected end in flow collection", line, col, pointer))
		return nil, false
	}
	switch p.data[p.pos] {
	case '*':
		line, col := p.lineCol(p.pos)
		p.add(diagAt(p.file, diagnostic.CodeManifestAliasForbidden, "YAML aliases are forbidden", line, col, pointer))
		return nil, false
	case '&':
		line, col := p.lineCol(p.pos)
		p.add(diagAt(p.file, diagnostic.CodeManifestAnchorForbidden, "YAML anchors are forbidden", line, col, pointer))
		return nil, false
	case '!':
		if tag := p.consumeComposeMergeTag(); tag != "" {
			p.skipFlowSpaces()
			if p.pos >= len(p.data) {
				line, col := p.lineCol(p.pos)
				return &Node{Kind: KindNull, Tag: tag, Line: line, Column: col, File: p.file}, true
			}
			n, ok := p.parseFlowNode(depth, pointer)
			if ok && n != nil {
				n.Tag = tag
			}
			return n, ok
		}
		line, col := p.lineCol(p.pos)
		p.add(diagAt(p.file, diagnostic.CodeManifestCustomTagForbidden, "YAML custom tags are forbidden", line, col, pointer))
		return nil, false
	case '[':
		return p.parseFlowSeq(depth, pointer)
	case '{':
		return p.parseFlowMap(depth, pointer)
	case '"', '\'':
		line, col := p.lineCol(p.pos)
		s, ok := p.parseQuoted()
		if !ok {
			return nil, false
		}
		return &Node{Kind: KindString, Str: s, Line: line, Column: col, File: p.file}, true
	default:
		return p.parseFlowScalar(pointer)
	}
}

func (p *yamlParser) parseFlowSeq(depth int, pointer string) (*Node, bool) {
	line, col := p.lineCol(p.pos)
	if p.data[p.pos] != '[' {
		return nil, false
	}
	p.pos++
	var seq []*Node
	p.skipFlowSpaces()
	if p.pos < len(p.data) && p.data[p.pos] == ']' {
		p.pos++
		return &Node{Kind: KindSeq, Seq: seq, Line: line, Column: col, File: p.file}, true
	}
	for {
		child, ok := p.parseFlowNode(depth+1, pointer)
		if !ok {
			return nil, false
		}
		seq = append(seq, child)
		p.skipFlowSpaces()
		if p.pos >= len(p.data) {
			p.add(diagAt(p.file, diagnostic.CodeManifestParseError, "unterminated flow sequence", line, col, pointer))
			return nil, false
		}
		if p.data[p.pos] == ']' {
			p.pos++
			break
		}
		if p.data[p.pos] != ',' {
			badLine, badCol := p.lineCol(p.pos)
			p.add(diagAt(p.file, diagnostic.CodeManifestParseError, "expected ',' or ']' in flow sequence", badLine, badCol, pointer))
			return nil, false
		}
		p.pos++
		p.skipFlowSpaces()
		if p.pos < len(p.data) && p.data[p.pos] == ']' {
			p.pos++
			break
		}
	}
	return &Node{Kind: KindSeq, Seq: seq, Line: line, Column: col, File: p.file}, true
}

func (p *yamlParser) parseFlowMap(depth int, pointer string) (*Node, bool) {
	line, col := p.lineCol(p.pos)
	if p.data[p.pos] != '{' {
		return nil, false
	}
	p.pos++
	var entries []MapEntry
	p.skipFlowSpaces()
	if p.pos < len(p.data) && p.data[p.pos] == '}' {
		p.pos++
		return &Node{Kind: KindMap, Map: entries, Line: line, Column: col, File: p.file}, true
	}
	for {
		key, keyLine, keyCol, ok := p.parseFlowMapKey()
		if !ok {
			return nil, false
		}
		p.skipFlowSpaces()
		if p.pos >= len(p.data) || p.data[p.pos] != ':' {
			badLine, badCol := p.lineCol(p.pos)
			p.add(diagAt(p.file, diagnostic.CodeManifestParseError, "expected ':' after flow map key", badLine, badCol, pointer))
			return nil, false
		}
		p.pos++
		p.skipFlowSpaces()
		val, ok := p.parseFlowNode(depth+1, joinPointer(pointer, key))
		if !ok {
			return nil, false
		}
		entries = append(entries, MapEntry{Key: key, KeyLine: keyLine, KeyColumn: keyCol, Value: val})
		p.skipFlowSpaces()
		if p.pos >= len(p.data) {
			p.add(diagAt(p.file, diagnostic.CodeManifestParseError, "unterminated flow mapping", line, col, pointer))
			return nil, false
		}
		if p.data[p.pos] == '}' {
			p.pos++
			break
		}
		if p.data[p.pos] != ',' {
			badLine, badCol := p.lineCol(p.pos)
			p.add(diagAt(p.file, diagnostic.CodeManifestParseError, "expected ',' or '}' in flow mapping", badLine, badCol, pointer))
			return nil, false
		}
		p.pos++
		p.skipFlowSpaces()
		if p.pos < len(p.data) && p.data[p.pos] == '}' {
			p.pos++
			break
		}
	}
	return &Node{Kind: KindMap, Map: entries, Line: line, Column: col, File: p.file}, true
}

func (p *yamlParser) parseFlowMapKey() (key string, keyLine, keyCol int, ok bool) {
	p.skipFlowSpaces()
	keyLine, keyCol = p.lineCol(p.pos)
	if p.pos >= len(p.data) {
		p.add(diagAt(p.file, diagnostic.CodeManifestParseError, "expected flow map key", keyLine, keyCol, ""))
		return "", 0, 0, false
	}
	if p.data[p.pos] == '"' || p.data[p.pos] == '\'' {
		s, okQ := p.parseQuoted()
		if !okQ {
			return "", 0, 0, false
		}
		return s, keyLine, keyCol, true
	}
	s, okS := p.scanFlowScalarToken()
	if !okS || s == "" {
		p.add(diagAt(p.file, diagnostic.CodeManifestParseError, "expected flow map key", keyLine, keyCol, ""))
		return "", 0, 0, false
	}
	return s, keyLine, keyCol, true
}

func (p *yamlParser) parseFlowScalar(pointer string) (*Node, bool) {
	line, col := p.lineCol(p.pos)
	s, ok := p.scanFlowScalarToken()
	if !ok {
		return nil, false
	}
	node := classifyPlainScalar(s, line, col, p.file)
	return node, true
}

// scanFlowScalarToken reads one unquoted flow scalar (stops at , ] } or # outside quotes).
func (p *yamlParser) scanFlowScalarToken() (string, bool) {
	start := p.pos
	inSingle := false
	inDouble := false
	depthSq := 0
	depthCur := 0
	for p.pos < len(p.data) {
		c := p.data[p.pos]
		if !inSingle && !inDouble {
			if c == '#' && (p.pos == start || p.data[p.pos-1] == ' ' || p.data[p.pos-1] == '\t') {
				break
			}
			switch c {
			case '\'':
				inSingle = true
				p.pos++
				continue
			case '"':
				inDouble = true
				p.pos++
				continue
			case '[':
				depthSq++
				p.pos++
				continue
			case ']':
				if depthSq == 0 && depthCur == 0 {
					goto done
				}
				if depthSq > 0 {
					depthSq--
				}
				p.pos++
				continue
			case '{':
				depthCur++
				p.pos++
				continue
			case '}':
				if depthSq == 0 && depthCur == 0 {
					goto done
				}
				if depthCur > 0 {
					depthCur--
				}
				p.pos++
				continue
			case ',':
				if depthSq == 0 && depthCur == 0 {
					goto done
				}
				p.pos++
				continue
			case ' ', '\t', '\n', '\r':
				if depthSq == 0 && depthCur == 0 {
					goto done
				}
				p.pos++
				continue
			}
			p.pos++
			continue
		}
		if inSingle {
			if c == '\'' {
				if p.pos+1 < len(p.data) && p.data[p.pos+1] == '\'' {
					p.pos += 2
					continue
				}
				inSingle = false
				p.pos++
				continue
			}
			p.pos++
			continue
		}
		if c == '\\' && inDouble {
			if p.pos+1 < len(p.data) {
				p.pos += 2
				continue
			}
		}
		if c == '"' && inDouble {
			inDouble = false
			p.pos++
			continue
		}
		p.pos++
	}
done:
	raw := p.data[start:p.pos]
	if len(raw) == 0 {
		line, col := p.lineCol(start)
		p.add(diagAt(p.file, diagnostic.CodeManifestParseError, "expected scalar value in flow collection", line, col, ""))
		return "", false
	}
	return string(bytesTrimRightSpace(raw)), true
}

func (p *yamlParser) skipFlowSpaces() {
	for p.pos < len(p.data) {
		c := p.data[p.pos]
		if c == ' ' || c == '\t' || c == '\n' || c == '\r' {
			p.pos++
			continue
		}
		break
	}
}
