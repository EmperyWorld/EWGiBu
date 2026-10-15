package loader

import (
	"fmt"
	"unicode/utf8"

	"iacpack/internal/diagnostic"
)

type yamlParser struct {
	file                  string
	data                  []byte
	pos                   int
	diags                 []diagnostic.Diagnostic
	allowFlowCollections  bool
	allowComposeMergeTags bool
}

func parseYAML(file string, data []byte) Document {
	p := &yamlParser{file: file, data: data}
	p.skipBlankAndComments()
	if p.pos >= len(p.data) {
		p.add(diagAt(file, diagnostic.CodeManifestParseError, "empty document", 1, 1, ""))
		return Document{File: file, Diags: p.diags}
	}
	// Optional leading --- for a single document.
	if p.atDocStart() {
		p.pos += 3
		p.skipSpacesOnly()
		if p.pos < len(p.data) && (p.data[p.pos] == '\n' || p.data[p.pos] == '\r') {
			p.consumeNewline()
		} else if p.pos < len(p.data) && p.data[p.pos] != '#' {
			// ---value is invalid for our subset; treat as parse error.
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

func (p *yamlParser) add(d diagnostic.Diagnostic) {
	p.diags = append(p.diags, d)
}

func (p *yamlParser) lineCol(pos int) (line, col int) {
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

func (p *yamlParser) atDocStart() bool {
	if p.pos+2 >= len(p.data) {
		return false
	}
	if p.data[p.pos] != '-' || p.data[p.pos+1] != '-' || p.data[p.pos+2] != '-' {
		return false
	}
	// Must be at line start (or after whitespace we already skipped to content).
	end := p.pos + 3
	if end == len(p.data) {
		return true
	}
	c := p.data[end]
	return c == ' ' || c == '\t' || c == '\n' || c == '\r' || c == '#'
}

func (p *yamlParser) atDocEnd() bool {
	if p.pos+2 >= len(p.data) {
		return false
	}
	if p.data[p.pos] != '.' || p.data[p.pos+1] != '.' || p.data[p.pos+2] != '.' {
		return false
	}
	end := p.pos + 3
	if end == len(p.data) {
		return true
	}
	c := p.data[end]
	return c == ' ' || c == '\t' || c == '\n' || c == '\r' || c == '#'
}

func (p *yamlParser) skipSpacesOnly() {
	for p.pos < len(p.data) && (p.data[p.pos] == ' ' || p.data[p.pos] == '\t') {
		p.pos++
	}
}

func (p *yamlParser) consumeNewline() {
	if p.pos < len(p.data) && p.data[p.pos] == '\r' {
		p.pos++
		if p.pos < len(p.data) && p.data[p.pos] == '\n' {
			p.pos++
		}
		return
	}
	if p.pos < len(p.data) && p.data[p.pos] == '\n' {
		p.pos++
	}
}

func (p *yamlParser) skipBlankAndComments() {
	for p.pos < len(p.data) {
		p.skipSpacesOnly()
		if p.pos >= len(p.data) {
			return
		}
		if p.data[p.pos] == '#' {
			for p.pos < len(p.data) && p.data[p.pos] != '\n' && p.data[p.pos] != '\r' {
				p.pos++
			}
			p.consumeNewline()
			continue
		}
		if p.data[p.pos] == '\n' || p.data[p.pos] == '\r' {
			p.consumeNewline()
			continue
		}
		return
	}
}

func (p *yamlParser) peekIndent() (indent int, ok bool) {
	save := p.pos
	p.skipBlankAndComments()
	if p.pos >= len(p.data) {
		p.pos = save
		return 0, false
	}
	// Find line start.
	lineStart := p.pos
	for lineStart > 0 && p.data[lineStart-1] != '\n' {
		lineStart--
	}
	ind := 0
	i := lineStart
	for i < len(p.data) {
		if p.data[i] == ' ' {
			ind++
			i++
			continue
		}
		if p.data[i] == '\t' {
			p.pos = save
			return -1, false
		}
		break
	}
	p.pos = save
	return ind, true
}

// consumeComposeMergeTag accepts Compose Spec !override and !reset on values.
// Other tags are left in place so the caller reports them as forbidden.
func (p *yamlParser) consumeComposeMergeTag() string {
	if !p.allowComposeMergeTags || p.pos >= len(p.data) || p.data[p.pos] != '!' {
		return ""
	}
	start := p.pos
	p.pos++
	nameStart := p.pos
	for p.pos < len(p.data) {
		c := p.data[p.pos]
		if (c >= 'A' && c <= 'Z') || (c >= 'a' && c <= 'z') || (c >= '0' && c <= '9') || c == '_' || c == '-' {
			p.pos++
			continue
		}
		break
	}
	name := string(p.data[nameStart:p.pos])
	if name != "override" && name != "reset" {
		p.pos = start
		return ""
	}
	return name
}

// parseBlockNode parses a block value. Compose !override and !reset tags are recorded on the node.
func (p *yamlParser) parseBlockNode(minIndent, depth int, pointer string) (*Node, bool) {
	mergeTag := ""
	if p.allowComposeMergeTags && p.pos < len(p.data) && p.data[p.pos] == '!' {
		mergeTag = p.consumeComposeMergeTag()
		if mergeTag != "" {
			p.skipSpacesOnly()
			if p.pos < len(p.data) && (p.data[p.pos] == '\n' || p.data[p.pos] == '\r' || p.data[p.pos] == '#') {
				p.skipToEOLComment()
				p.skipBlankAndComments()
			}
			if p.pos >= len(p.data) {
				line, col := p.lineCol(p.pos)
				return &Node{Kind: KindNull, Tag: mergeTag, Line: line, Column: col, File: p.file}, true
			}
		}
	}
	n, ok := p.parseBlockNodeBody(minIndent, depth, pointer)
	if ok && n != nil && mergeTag != "" {
		n.Tag = mergeTag
	}
	return n, ok
}

// parseBlockNodeBody parses a block or flow-incompatible scalar/collection at indent minIndent.
func (p *yamlParser) parseBlockNodeBody(minIndent, depth int, pointer string) (*Node, bool) {
	if depth > MaxDepth {
		line, col := p.lineCol(p.pos)
		p.add(diagAt(p.file, diagnostic.CodeManifestTooDeep,
			fmt.Sprintf("manifest exceeds maximum depth of %d", MaxDepth), line, col, pointer))
		return nil, false
	}
	p.skipSpacesOnly()
	if p.pos >= len(p.data) {
		line, col := p.lineCol(p.pos)
		p.add(diagAt(p.file, diagnostic.CodeManifestParseError, "unexpected end of document", line, col, pointer))
		return nil, false
	}

	// Forbidden constructs at value start.
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
		line, col := p.lineCol(p.pos)
		p.add(diagAt(p.file, diagnostic.CodeManifestCustomTagForbidden, "YAML custom tags are forbidden", line, col, pointer))
		return nil, false
	case '{', '[':
		if p.allowFlowCollections {
			return p.parseFlowNode(depth, pointer)
		}
		line, col := p.lineCol(p.pos)
		p.add(diagAt(p.file, diagnostic.CodeManifestParseError, "flow-style collections are not supported in this YAML subset", line, col, pointer).
			WithHint("Use block style maps and sequences"))
		return nil, false
	case '|', '>':
		line, col := p.lineCol(p.pos)
		p.add(diagAt(p.file, diagnostic.CodeManifestParseError, "literal/folded block scalars are not supported in this YAML subset", line, col, pointer))
		return nil, false
	}

	// Sequence indicator at current position.
	if p.data[p.pos] == '-' && p.isSeqIndicator() {
		return p.parseBlockSeq(minIndent, depth, pointer)
	}

	// Inline quoted/plain scalar on this line, or start of block map.
	line, col := p.lineCol(p.pos)
	if p.data[p.pos] == '"' || p.data[p.pos] == '\'' {
		s, ok := p.parseQuoted()
		if !ok {
			return nil, false
		}
		p.skipSpacesOnly()
		if p.pos < len(p.data) && p.data[p.pos] == ':' {
			// Quoted key of a map — rewind conceptually by parsing as map.
			// We already consumed the key; handle map with known first key.
			return p.parseBlockMapAfterKey(s, line, col, minIndent, depth, pointer)
		}
		p.skipToEOLComment()
		return &Node{Kind: KindString, Str: s, Line: line, Column: col, File: p.file}, true
	}

	// Look ahead: if this line has "key:" pattern, it's a map.
	if p.lineLooksLikeMapKey() {
		return p.parseBlockMap(minIndent, depth, pointer)
	}

	// Plain scalar.
	s, ok := p.parsePlainScalar()
	if !ok {
		return nil, false
	}
	node := classifyPlainScalar(s, line, col, p.file)
	p.skipToEOLComment()
	return node, true
}

func (p *yamlParser) isSeqIndicator() bool {
	// "-" followed by space or EOL.
	if p.pos+1 >= len(p.data) {
		return true
	}
	c := p.data[p.pos+1]
	return c == ' ' || c == '\t' || c == '\n' || c == '\r' || c == '#'
}

func (p *yamlParser) lineLooksLikeMapKey() bool {
	i := p.pos
	if i >= len(p.data) {
		return false
	}
	if p.data[i] == '"' || p.data[i] == '\'' {
		// quoted key handled elsewhere
		return false
	}
	// Scan until ':' that is followed by space/EOL/comment, not part of plain value mid-token carelessly.
	for i < len(p.data) && p.data[i] != '\n' && p.data[i] != '\r' {
		if p.data[i] == '#' && (i == p.pos || p.data[i-1] == ' ' || p.data[i-1] == '\t') {
			return false
		}
		if p.data[i] == ':' {
			next := byte('\n')
			if i+1 < len(p.data) {
				next = p.data[i+1]
			}
			if next == ' ' || next == '\t' || next == '\n' || next == '\r' || next == '#' || i+1 >= len(p.data) {
				// Reject empty key.
				if i == p.pos {
					return false
				}
				return true
			}
		}
		i++
	}
	return false
}

func (p *yamlParser) parseBlockMap(minIndent, depth int, pointer string) (*Node, bool) {
	line, col := p.lineCol(p.pos)
	node := &Node{Kind: KindMap, Line: line, Column: col, File: p.file}
	seen := map[string]struct{}{}

	for {
		p.skipBlankAndComments()
		if p.pos >= len(p.data) {
			break
		}
		if p.atDocStart() || p.atDocEnd() {
			break
		}
		ind, ok := p.peekIndent()
		if !ok || ind < 0 {
			line, col := p.lineCol(p.pos)
			p.add(diagAt(p.file, diagnostic.CodeManifestParseError, "tabs are not allowed in indentation", line, col, pointer))
			return nil, false
		}
		if ind < minIndent {
			break
		}
		// First entry establishes indent; subsequent must match.
		entryIndent := ind
		if len(node.Map) == 0 {
			minIndent = entryIndent
		} else if entryIndent != minIndent {
			if entryIndent > minIndent {
				line, col := p.lineCol(p.pos)
				p.add(diagAt(p.file, diagnostic.CodeManifestParseError, "unexpected indentation in mapping", line, col, pointer))
				return nil, false
			}
			break
		}

		// Position at key content.
		p.skipBlankAndComments()
		keyLine, keyCol := p.lineCol(p.pos)
		key, ok := p.parseMapKey()
		if !ok {
			return nil, false
		}
		if key == "<<" {
			p.add(diagAt(p.file, diagnostic.CodeManifestMergeKeyForbidden, "YAML merge keys are forbidden", keyLine, keyCol, pointer))
			return nil, false
		}
		if _, exists := seen[key]; exists {
			p.add(diagAt(p.file, diagnostic.CodeManifestDuplicateKey,
				fmt.Sprintf("duplicate key %q", key), keyLine, keyCol, joinPointer(pointer, key)).
				WithHint("Remove or rename the duplicate key"))
			return nil, false
		}
		seen[key] = struct{}{}

		p.skipSpacesOnly()
		if p.pos >= len(p.data) || p.data[p.pos] != ':' {
			line, col := p.lineCol(p.pos)
			p.add(diagAt(p.file, diagnostic.CodeManifestParseError, "expected ':' after map key", line, col, pointer))
			return nil, false
		}
		p.pos++ // :
		p.skipSpacesOnly()

		childPtr := joinPointer(pointer, key)
		var val *Node
		if p.pos >= len(p.data) || p.data[p.pos] == '\n' || p.data[p.pos] == '\r' || p.data[p.pos] == '#' {
			p.skipToEOLComment()
			p.skipBlankAndComments()
			if p.pos >= len(p.data) {
				val = &Node{Kind: KindNull, Line: keyLine, Column: keyCol, File: p.file}
			} else {
				nextInd, indOK := p.peekIndent()
				if !indOK || nextInd < 0 {
					line, col := p.lineCol(p.pos)
					p.add(diagAt(p.file, diagnostic.CodeManifestParseError, "tabs are not allowed in indentation", line, col, childPtr))
					return nil, false
				}
				if nextInd <= minIndent {
					val = &Node{Kind: KindNull, Line: keyLine, Column: keyCol, File: p.file}
				} else {
					var valOK bool
					val, valOK = p.parseBlockNode(nextInd, depth+1, childPtr)
					if !valOK {
						return nil, false
					}
				}
			}
		} else {
			// Same-line value: reuse block-node parsing so forbidden constructs
			// (aliases, anchors, tags, flow style) share one code path.
			var valOK bool
			val, valOK = p.parseBlockNode(minIndent+1, depth+1, childPtr)
			if !valOK {
				return nil, false
			}
		}
		node.Map = append(node.Map, MapEntry{
			Key:       key,
			KeyLine:   keyLine,
			KeyColumn: keyCol,
			Value:     val,
		})
	}
	return node, true
}

func (p *yamlParser) parseBlockMapAfterKey(key string, keyLine, keyCol, minIndent, depth int, pointer string) (*Node, bool) {
	// We've consumed a quoted key and are at ':'.
	p.skipSpacesOnly()
	if p.pos >= len(p.data) || p.data[p.pos] != ':' {
		line, col := p.lineCol(p.pos)
		p.add(diagAt(p.file, diagnostic.CodeManifestParseError, "expected ':' after map key", line, col, pointer))
		return nil, false
	}
	// Rebuild by putting key back into parseBlockMap flow — simpler: finish first entry then continue.
	line, col := keyLine, keyCol
	node := &Node{Kind: KindMap, Line: line, Column: col, File: p.file}
	seen := map[string]struct{}{key: {}}
	p.pos++ // :
	p.skipSpacesOnly()
	childPtr := joinPointer(pointer, key)
	var val *Node
	if p.pos >= len(p.data) || p.data[p.pos] == '\n' || p.data[p.pos] == '\r' || p.data[p.pos] == '#' {
		p.skipToEOLComment()
		p.skipBlankAndComments()
		nextInd, indOK := p.peekIndent()
		if indOK && nextInd > minIndent {
			var valOK bool
			val, valOK = p.parseBlockNode(nextInd, depth+1, childPtr)
			if !valOK {
				return nil, false
			}
		} else {
			val = &Node{Kind: KindNull, Line: keyLine, Column: keyCol, File: p.file}
		}
	} else {
		var valOK bool
		val, valOK = p.parseBlockNode(minIndent+1, depth+1, childPtr)
		if !valOK {
			return nil, false
		}
	}
	node.Map = append(node.Map, MapEntry{Key: key, KeyLine: keyLine, KeyColumn: keyCol, Value: val})

	// Continue with remaining keys at same indent — reuse parseBlockMap by setting minIndent.
	rest, ok := p.parseBlockMap(minIndent, depth, pointer)
	if !ok {
		return nil, false
	}
	for _, e := range rest.Map {
		if _, exists := seen[e.Key]; exists {
			p.add(diagAt(p.file, diagnostic.CodeManifestDuplicateKey,
				fmt.Sprintf("duplicate key %q", e.Key), e.KeyLine, e.KeyColumn, joinPointer(pointer, e.Key)))
			return nil, false
		}
		seen[e.Key] = struct{}{}
		node.Map = append(node.Map, e)
	}
	return node, true
}

func (p *yamlParser) parseMapKey() (string, bool) {
	if p.pos >= len(p.data) {
		line, col := p.lineCol(p.pos)
		p.add(diagAt(p.file, diagnostic.CodeManifestParseError, "expected map key", line, col, ""))
		return "", false
	}
	if p.data[p.pos] == '*' || p.data[p.pos] == '&' || p.data[p.pos] == '!' {
		line, col := p.lineCol(p.pos)
		switch p.data[p.pos] {
		case '*':
			p.add(diagAt(p.file, diagnostic.CodeManifestAliasForbidden, "YAML aliases are forbidden", line, col, ""))
		case '&':
			p.add(diagAt(p.file, diagnostic.CodeManifestAnchorForbidden, "YAML anchors are forbidden", line, col, ""))
		default:
			p.add(diagAt(p.file, diagnostic.CodeManifestCustomTagForbidden, "YAML custom tags are forbidden", line, col, ""))
		}
		return "", false
	}
	if p.data[p.pos] == '"' || p.data[p.pos] == '\'' {
		return p.parseQuoted()
	}
	// Plain key until ':'.
	start := p.pos
	for p.pos < len(p.data) {
		c := p.data[p.pos]
		if c == ':' {
			break
		}
		if c == '\n' || c == '\r' {
			line, col := p.lineCol(start)
			p.add(diagAt(p.file, diagnostic.CodeManifestParseError, "expected ':' after map key", line, col, ""))
			return "", false
		}
		p.pos++
	}
	key := string(bytesTrimRightSpace(p.data[start:p.pos]))
	if key == "" {
		line, col := p.lineCol(start)
		p.add(diagAt(p.file, diagnostic.CodeManifestParseError, "empty map keys are forbidden", line, col, ""))
		return "", false
	}
	// Reject complex keys that look non-string (we only allow plain/quoted strings).
	if !utf8.ValidString(key) {
		line, col := p.lineCol(start)
		p.add(diagAt(p.file, diagnostic.CodeManifestNonStringKey, "map keys must be strings", line, col, ""))
		return "", false
	}
	return key, true
}

func (p *yamlParser) parseBlockSeq(minIndent, depth int, pointer string) (*Node, bool) {
	line, col := p.lineCol(p.pos)
	node := &Node{Kind: KindSeq, Line: line, Column: col, File: p.file}
	index := 0
	for {
		p.skipBlankAndComments()
		if p.pos >= len(p.data) {
			break
		}
		ind, ok := p.peekIndent()
		if !ok || ind < 0 {
			line, col := p.lineCol(p.pos)
			p.add(diagAt(p.file, diagnostic.CodeManifestParseError, "tabs are not allowed in indentation", line, col, pointer))
			return nil, false
		}
		if ind < minIndent {
			break
		}
		if len(node.Seq) == 0 {
			minIndent = ind
		} else if ind != minIndent {
			if ind > minIndent {
				break // nested content belongs to previous item; should have been consumed
			}
			break
		}
		p.skipBlankAndComments()
		if p.pos >= len(p.data) || p.data[p.pos] != '-' || !p.isSeqIndicator() {
			break
		}
		itemLine, itemCol := p.lineCol(p.pos)
		p.pos++ // -
		p.skipSpacesOnly()
		childPtr := fmt.Sprintf("%s/%d", pointer, index)
		if pointer == "" {
			childPtr = fmt.Sprintf("/%d", index)
		}
		var item *Node
		if p.pos >= len(p.data) || p.data[p.pos] == '\n' || p.data[p.pos] == '\r' || p.data[p.pos] == '#' {
			p.skipToEOLComment()
			p.skipBlankAndComments()
			nextInd, _ := p.peekIndent()
			if nextInd > minIndent {
				item, ok = p.parseBlockNode(nextInd, depth+1, childPtr)
				if !ok {
					return nil, false
				}
			} else {
				item = &Node{Kind: KindNull, Line: itemLine, Column: itemCol, File: p.file}
			}
		} else if p.lineLooksLikeMapKey() || (p.data[p.pos] == '"' || p.data[p.pos] == '\'') {
			// Same-line map entry after "- key: value" (continuation keys use seq line indent + 2).
			if p.lineLooksLikeMapKey() {
				keyLine, keyCol := p.lineCol(p.pos)
				key, keyOK := p.parseMapKey()
				if !keyOK {
					return nil, false
				}
				itemIndent := ind + 2
				item, ok = p.parseBlockMapAfterKey(key, keyLine, keyCol, itemIndent, depth+1, childPtr)
			} else {
				item, ok = p.parseBlockNode(minIndent+2, depth+1, childPtr)
			}
			if !ok {
				return nil, false
			}
		} else {
			item, ok = p.parseBlockNode(minIndent+2, depth+1, childPtr)
			if !ok {
				return nil, false
			}
		}
		node.Seq = append(node.Seq, item)
		index++
	}
	return node, true
}

func (p *yamlParser) parseQuoted() (string, bool) {
	quote := p.data[p.pos]
	startLine, startCol := p.lineCol(p.pos)
	p.pos++
	var b []byte
	for p.pos < len(p.data) {
		c := p.data[p.pos]
		if quote == '\'' {
			if c == '\'' {
				if p.pos+1 < len(p.data) && p.data[p.pos+1] == '\'' {
					b = append(b, '\'')
					p.pos += 2
					continue
				}
				p.pos++
				return string(b), true
			}
			if c == '\n' || c == '\r' {
				p.add(diagAt(p.file, diagnostic.CodeManifestParseError, "unterminated single-quoted string", startLine, startCol, ""))
				return "", false
			}
			b = append(b, c)
			p.pos++
			continue
		}
		// double quote
		if c == '"' {
			p.pos++
			return string(b), true
		}
		if c == '\\' {
			if p.pos+1 >= len(p.data) {
				p.add(diagAt(p.file, diagnostic.CodeManifestParseError, "unterminated escape in string", startLine, startCol, ""))
				return "", false
			}
			esc, n, ok := decodeJSONEscape(p.data[p.pos:])
			if !ok {
				line, col := p.lineCol(p.pos)
				p.add(diagAt(p.file, diagnostic.CodeManifestParseError, "invalid escape sequence", line, col, ""))
				return "", false
			}
			b = append(b, esc...)
			p.pos += n
			continue
		}
		if c == '\n' || c == '\r' {
			p.add(diagAt(p.file, diagnostic.CodeManifestParseError, "unterminated double-quoted string", startLine, startCol, ""))
			return "", false
		}
		b = append(b, c)
		p.pos++
	}
	p.add(diagAt(p.file, diagnostic.CodeManifestParseError, "unterminated quoted string", startLine, startCol, ""))
	return "", false
}

func (p *yamlParser) parsePlainScalar() (string, bool) {
	start := p.pos
	for p.pos < len(p.data) {
		c := p.data[p.pos]
		if c == '\n' || c == '\r' {
			break
		}
		if c == '#' && (p.pos == start || p.data[p.pos-1] == ' ' || p.data[p.pos-1] == '\t') {
			break
		}
		// Detect forbidden indicators mid-stream for aliases etc. at start only — already checked.
		p.pos++
	}
	raw := bytesTrimRightSpace(p.data[start:p.pos])
	if len(raw) == 0 {
		line, col := p.lineCol(start)
		p.add(diagAt(p.file, diagnostic.CodeManifestParseError, "expected scalar value", line, col, ""))
		return "", false
	}
	s := string(raw)
	if len(s) > 0 {
		switch s[0] {
		case '*':
			line, col := p.lineCol(start)
			p.add(diagAt(p.file, diagnostic.CodeManifestAliasForbidden, "YAML aliases are forbidden", line, col, ""))
			return "", false
		case '&':
			line, col := p.lineCol(start)
			p.add(diagAt(p.file, diagnostic.CodeManifestAnchorForbidden, "YAML anchors are forbidden", line, col, ""))
			return "", false
		case '!':
			line, col := p.lineCol(start)
			p.add(diagAt(p.file, diagnostic.CodeManifestCustomTagForbidden, "YAML custom tags are forbidden", line, col, ""))
			return "", false
		case '{', '[':
			line, col := p.lineCol(start)
			p.add(diagAt(p.file, diagnostic.CodeManifestParseError, "flow-style collections are not supported in this YAML subset", line, col, "").
				WithHint("Use block style maps and sequences"))
			return "", false
		case '|', '>':
			line, col := p.lineCol(start)
			p.add(diagAt(p.file, diagnostic.CodeManifestParseError, "literal/folded block scalars are not supported in this YAML subset", line, col, ""))
			return "", false
		}
	}
	if !p.allowFlowCollections {
		for i := 0; i < len(raw); i++ {
			switch raw[i] {
			case '{', '}', '[', ']':
				line, col := p.lineCol(start + i)
				p.add(diagAt(p.file, diagnostic.CodeManifestParseError, "flow-style collections are not supported in this YAML subset", line, col, "").
					WithHint("Use block style maps and sequences"))
				return "", false
			}
		}
	}
	return s, true
}

func (p *yamlParser) skipToEOLComment() {
	p.skipSpacesOnly()
	if p.pos < len(p.data) && p.data[p.pos] == '#' {
		for p.pos < len(p.data) && p.data[p.pos] != '\n' && p.data[p.pos] != '\r' {
			p.pos++
		}
	}
	if p.pos < len(p.data) && (p.data[p.pos] == '\n' || p.data[p.pos] == '\r') {
		p.consumeNewline()
	}
}

func classifyPlainScalar(s string, line, col int, file string) *Node {
	switch s {
	case "null", "Null", "NULL", "~":
		return &Node{Kind: KindNull, Line: line, Column: col, File: file}
	case "true", "True", "TRUE":
		return &Node{Kind: KindBool, Bool: true, Line: line, Column: col, File: file}
	case "false", "False", "FALSE":
		return &Node{Kind: KindBool, Bool: false, Line: line, Column: col, File: file}
	}
	if isYAMLNumber(s) {
		return &Node{Kind: KindNumber, Str: s, Line: line, Column: col, File: file}
	}
	return &Node{Kind: KindString, Str: s, Line: line, Column: col, File: file}
}

func isYAMLNumber(s string) bool {
	if s == "" {
		return false
	}
	i := 0
	if s[0] == '+' || s[0] == '-' {
		i++
		if i >= len(s) {
			return false
		}
	}
	dots := 0
	digits := 0
	for ; i < len(s); i++ {
		c := s[i]
		if c >= '0' && c <= '9' {
			digits++
			continue
		}
		if c == '.' {
			dots++
			if dots > 1 {
				return false
			}
			continue
		}
		return false
	}
	return digits > 0
}

func bytesTrimRightSpace(b []byte) []byte {
	i := len(b)
	for i > 0 && (b[i-1] == ' ' || b[i-1] == '\t') {
		i--
	}
	return b[:i]
}

func decodeJSONEscape(data []byte) (out []byte, n int, ok bool) {
	if len(data) < 2 || data[0] != '\\' {
		return nil, 0, false
	}
	switch data[1] {
	case '"', '\\', '/':
		return []byte{data[1]}, 2, true
	case 'b':
		return []byte{'\b'}, 2, true
	case 'f':
		return []byte{'\f'}, 2, true
	case 'n':
		return []byte{'\n'}, 2, true
	case 'r':
		return []byte{'\r'}, 2, true
	case 't':
		return []byte{'\t'}, 2, true
	case 'u':
		if len(data) < 6 {
			return nil, 0, false
		}
		var r rune
		for i := 2; i < 6; i++ {
			r <<= 4
			c := data[i]
			switch {
			case c >= '0' && c <= '9':
				r |= rune(c - '0')
			case c >= 'a' && c <= 'f':
				r |= rune(c - 'a' + 10)
			case c >= 'A' && c <= 'F':
				r |= rune(c - 'A' + 10)
			default:
				return nil, 0, false
			}
		}
		buf := make([]byte, 4)
		m := utf8.EncodeRune(buf, r)
		return buf[:m], 6, true
	default:
		return nil, 0, false
	}
}
