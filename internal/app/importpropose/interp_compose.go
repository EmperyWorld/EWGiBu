package importpropose

import (
	"fmt"
	"strings"

	"iacpack/internal/diagnostic"
	"iacpack/internal/manifest/loader"
)

// resolveImportLiteralScalar maps a scalar that may be ${VAR:-default} to a fixed literal.
func resolveImportLiteralScalar(s string) (literal string, ok bool) {
	s = strings.TrimSpace(s)
	if s == "" {
		return "", false
	}
	if !interpRe.MatchString(s) {
		return s, true
	}
	for i := 0; i < 32; i++ {
		def, ok := composeSingleInterpDefault(s)
		if !ok {
			return "", false
		}
		def = strings.TrimSpace(def)
		if def == "" || def == s {
			return "", false
		}
		if !interpRe.MatchString(def) {
			return def, true
		}
		s = def
	}
	return "", false
}

func composeSingleInterpDefault(s string) (string, bool) {
	s = strings.TrimSpace(s)
	if len(s) < 4 || !strings.HasPrefix(s, "${") || !strings.HasSuffix(s, "}") {
		return "", false
	}
	inner := strings.TrimSpace(s[2 : len(s)-1])
	if inner == "" {
		return "", false
	}
	for _, sep := range []string{":-", "-"} {
		idx := strings.Index(inner, sep)
		if idx <= 0 {
			continue
		}
		name := strings.TrimSpace(inner[:idx])
		if !composeInterpVarName(name) {
			continue
		}
		if strings.Contains(name, "?") {
			continue
		}
		return inner[idx+len(sep):], true
	}
	return "", false
}

func composeInterpVarName(name string) bool {
	if name == "" {
		return false
	}
	for i, r := range name {
		if i == 0 {
			if r != '_' && (r < 'A' || r > 'Z') && (r < 'a' || r > 'z') {
				return false
			}
			continue
		}
		if r != '_' && (r < 'A' || r > 'Z') && (r < 'a' || r > 'z') && (r < '0' || r > '9') {
			return false
		}
	}
	return true
}

// splitOutsideInterpolation splits s by sep, ignoring separators inside ${…}.
func splitOutsideInterpolation(s string, sep byte) []string {
	if sep == 0 {
		return nil
	}
	var parts []string
	var b strings.Builder
	depth := 0
	for i := 0; i < len(s); i++ {
		c := s[i]
		if c == '$' && i+1 < len(s) && s[i+1] == '{' {
			depth++
			b.WriteByte(c)
			i++
			b.WriteByte(s[i])
			continue
		}
		if depth > 0 {
			b.WriteByte(c)
			if c == '}' {
				depth--
			}
			continue
		}
		if c == sep {
			parts = append(parts, b.String())
			b.Reset()
			continue
		}
		b.WriteByte(c)
	}
	parts = append(parts, b.String())
	return parts
}

// unfoldComposeDollars pairs Compose $$ into a literal $ before ${…} is detected.
// $${POSTGRES_USER} is the shell string ${POSTGRES_USER}, not a Compose interpolation.
func unfoldComposeDollars(s string) (out string, unescapedInterp bool) {
	var b strings.Builder
	b.Grow(len(s))
	for i := 0; i < len(s); i++ {
		if s[i] != '$' {
			b.WriteByte(s[i])
			continue
		}
		if i+1 < len(s) && s[i+1] == '$' {
			b.WriteByte('$')
			i++
			continue
		}
		if i+1 < len(s) && s[i+1] == '{' {
			unescapedInterp = true
		}
		b.WriteByte('$')
	}
	return b.String(), unescapedInterp
}

// manifestSafeScalar returns a value safe for iacpack YAML (no raw Compose ${…}).
func manifestSafeScalar(s string) (out string, ok bool) {
	unfolded, interp := unfoldComposeDollars(s)
	if !interp {
		return unfolded, true
	}
	if lit, ok := resolveImportLiteralScalar(strings.TrimSpace(unfolded)); ok {
		return lit, true
	}
	return "", false
}

func importCommandValue(
	n *loader.Node,
	file, pointer, field string,
	parameters, secrets map[string]any,
) ([]any, []diagnostic.Diagnostic) {
	if n == nil {
		return nil, nil
	}
	if n.Kind == loader.KindString {
		return importCommandString(n, file, pointer, field, parameters, secrets)
	}
	return importCommandList(n, file, pointer, field, parameters, secrets)
}

func importCommandString(
	n *loader.Node,
	file, pointer, field string,
	parameters, secrets map[string]any,
) ([]any, []diagnostic.Diagnostic) {
	words, ok := splitComposeCommandString(n.Str)
	if !ok {
		return nil, []diagnostic.Diagnostic{diagnostic.Warning(diagnostic.CodeImportUnsupportedField,
			field+": string command omitted (unbalanced quotes)").
			WithSource(file, pointer, n.Line, n.Column)}
	}
	seq := &loader.Node{Kind: loader.KindSeq, Line: n.Line, Column: n.Column, File: n.File}
	for _, w := range words {
		seq.Seq = append(seq.Seq, &loader.Node{
			Kind: loader.KindString, Str: w, Line: n.Line, Column: n.Column, File: n.File,
		})
	}
	return importCommandList(seq, file, pointer, field, parameters, secrets)
}

// splitComposeCommandString splits a Compose string command into argv.
// A shell operator keeps the original string as /bin/sh -c.
func splitComposeCommandString(s string) ([]string, bool) {
	s = strings.TrimSpace(s)
	if s == "" {
		return nil, false
	}
	var words []string
	var b strings.Builder
	var quote byte
	for i := 0; i < len(s); i++ {
		c := s[i]
		if quote != 0 {
			if c == quote {
				quote = 0
				continue
			}
			if quote == '"' && c == '\\' && i+1 < len(s) {
				i++
				b.WriteByte(s[i])
				continue
			}
			b.WriteByte(c)
			continue
		}
		switch c {
		case ' ', '\t', '\n', '\r':
			if b.Len() > 0 {
				words = append(words, b.String())
				b.Reset()
			}
		case '\'', '"':
			quote = c
		case '\\':
			if i+1 < len(s) {
				i++
				b.WriteByte(s[i])
			}
		case '|', '&', ';', '<', '>', '(', ')', '`':
			return []string{"/bin/sh", "-c", s}, true
		default:
			b.WriteByte(c)
		}
	}
	if quote != 0 {
		return nil, false
	}
	if b.Len() > 0 {
		words = append(words, b.String())
	}
	if len(words) == 0 {
		return nil, false
	}
	return words, true
}

func importCommandList(
	n *loader.Node,
	file, pointer, field string,
	parameters, secrets map[string]any,
) ([]any, []diagnostic.Diagnostic) {
	if n == nil || n.Kind != loader.KindSeq {
		return nil, nil
	}
	var diags []diagnostic.Diagnostic
	out := make([]any, 0, len(n.Seq))
	for _, c := range n.Seq {
		raw := loaderSeqString(c)
		if c != nil && c.Kind == loader.KindString && c.Str == "" {
			out = append(out, map[string]any{"literal": ""})
			continue
		}
		if raw == "" {
			continue
		}
		unfolded, interp := unfoldComposeDollars(raw)
		if !interp {
			if strings.TrimSpace(unfolded) == "" {
				continue
			}
			out = append(out, unfolded)
			continue
		}
		parts, ok := argvInterpParts(unfolded, parameters, secrets)
		if !ok {
			diags = append(diags, diagnostic.Warning(diagnostic.CodeImportUnsupportedField,
				field+": argv element with Compose interpolation omitted (unbalanced ${…})").
				WithSource(file, pointer, c.Line, c.Column))
			continue
		}
		if len(parts) == 1 {
			switch p := parts[0].(type) {
			case string:
				out = append(out, p)
				continue
			case map[string]any:
				if _, isSec := p["secret"]; isSec && len(p) == 1 {
					out = append(out, p)
					continue
				}
			}
		}
		out = append(out, map[string]any{"parts": parts})
	}
	if len(out) == 0 {
		return nil, diags
	}
	return out, diags
}

func argvInterpParts(s string, parameters, secrets map[string]any) (parts []any, ok bool) {
	var b strings.Builder
	for i := 0; i < len(s); {
		if s[i] == '$' && i+1 < len(s) && s[i+1] == '{' {
			end := strings.IndexByte(s[i:], '}')
			if end < 0 {
				return nil, false
			}
			end = i + end
			token := s[i : end+1]
			name := paramIDFromInterp(token)
			if b.Len() > 0 {
				parts = append(parts, b.String())
				b.Reset()
			}
			id := sanitizeID(name)
			if id == "" {
				return nil, false
			}
			if isSecretLikeKey(name) {
				ensureSecret(secrets, id, "Imported secret reference (value not imported)")
				parts = append(parts, map[string]any{"secret": id})
			} else {
				ensureParam(parameters, id, "string")
				if def, has := composeSingleInterpDefault(token); has {
					def = strings.TrimSpace(def)
					if def != "" && !interpRe.MatchString(def) {
						setParamDefault(parameters, id, def)
					}
				}
				parts = append(parts, map[string]any{"parameter": id})
			}
			i = end + 1
			continue
		}
		b.WriteByte(s[i])
		i++
	}
	if b.Len() > 0 {
		parts = append(parts, b.String())
	}
	return parts, len(parts) > 0
}

func envInterpValue(raw, file, pointer string, parameters, secrets map[string]any) (map[string]any, []diagnostic.Diagnostic) {
	parts, ok := argvInterpParts(raw, parameters, secrets)
	if !ok {
		return nil, []diagnostic.Diagnostic{diagnostic.Warning(diagnostic.CodeImportUnsupportedField,
			"env value with unbalanced Compose interpolation omitted").
			WithSource(file, pointer, 0, 0)}
	}
	out := make([]any, 0, len(parts))
	var diags []diagnostic.Diagnostic
	var sawSecret bool
	for _, p := range parts {
		switch t := p.(type) {
		case string:
			out = append(out, map[string]any{"literal": t})
		case map[string]any:
			if _, ok := t["secret"]; ok {
				sawSecret = true
			}
			out = append(out, t)
		}
	}
	if sawSecret {
		diags = append(diags, diagnostic.Warning(diagnostic.CodeImportSecretLike,
			pointer+": secret interpolation mapped to a secret reference (value not imported)").
			WithSource(file, pointer, 0, 0))
	}
	if len(out) == 1 {
		if m, ok := out[0].(map[string]any); ok {
			diags = append(diags, diagnostic.Info(diagnostic.CodeImportInterpolation,
				pointer+": Compose interpolation mapped to a parameter or secret reference").
				WithSource(file, pointer, 0, 0))
			return m, diags
		}
	}
	diags = append(diags, diagnostic.Info(diagnostic.CodeImportInterpolation,
		pointer+": Compose interpolation split into literal, parameter, and secret parts").
		WithSource(file, pointer, 0, 0))
	return map[string]any{"parts": out}, diags
}

func secretIDsInEnvValue(v any) []string {
	m, ok := v.(map[string]any)
	if !ok {
		return nil
	}
	if id, ok := m["secret"].(string); ok && id != "" && m["parts"] == nil {
		return []string{id}
	}
	parts, _ := m["parts"].([]any)
	var ids []string
	for _, p := range parts {
		pm, ok := p.(map[string]any)
		if !ok {
			continue
		}
		if id, ok := pm["secret"].(string); ok && id != "" {
			ids = append(ids, id)
		}
	}
	return ids
}

func importServiceUser(raw, file, pointer string) (string, []diagnostic.Diagnostic) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", nil
	}
	if !interpRe.MatchString(raw) {
		if !validImportUser(raw) {
			return "", []diagnostic.Diagnostic{diagnostic.Warning(diagnostic.CodeImportUnsupportedField,
				"service user omitted; expected a name or uid:gid").
				WithSource(file, pointer, 0, 0)}
		}
		return raw, nil
	}
	if def, ok := composeSingleInterpDefault(raw); ok {
		def = strings.TrimSpace(def)
		if validImportUser(def) {
			return def, []diagnostic.Diagnostic{diagnostic.Info(diagnostic.CodeImportInterpolation,
				"service user imported from Compose default "+def).
				WithSource(file, pointer, 0, 0)}
		}
	}
	return "", []diagnostic.Diagnostic{diagnostic.Warning(diagnostic.CodeImportUnsupportedField,
		"service user interpolation omitted; user is a literal name or uid:gid").
		WithSource(file, pointer, 0, 0)}
}

func validImportUser(s string) bool {
	if s == "" || len(s) > 64 || strings.ContainsAny(s, " \t") {
		return false
	}
	for _, r := range s {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9':
		case r == '_' || r == '.' || r == '-' || r == ':':
		default:
			return false
		}
	}
	return true
}

func setParamDefault(parameters map[string]any, id, def string) {
	p, _ := parameters[id].(map[string]any)
	if p == nil {
		return
	}
	if _, exists := p["default"]; exists {
		return
	}
	p["default"] = def
	parameters[id] = p
}

func manifestSafeArgvList(
	n *loader.Node,
	file, pointer, field string,
) ([]any, []diagnostic.Diagnostic) {
	if n == nil || n.Kind != loader.KindSeq {
		return nil, nil
	}
	var diags []diagnostic.Diagnostic
	out := make([]any, 0, len(n.Seq))
	skipNext := false
	for i, c := range n.Seq {
		if skipNext {
			skipNext = false
			continue
		}
		var raw string
		switch c.Kind {
		case loader.KindString:
			raw = c.Str
		case loader.KindNumber, loader.KindBool:
			raw = c.Str
		default:
			continue
		}
		if raw == "--requirepass" && i+1 < len(n.Seq) {
			if _, nextOK := manifestSafeScalar(loaderSeqString(n.Seq[i+1])); !nextOK {
				skipNext = true
				diags = append(diags, diagnostic.Warning(diagnostic.CodeImportUnsupportedField,
					field+": --requirepass omitted (manifest cannot store Compose ${…} password)").
					WithSource(file, pointer, c.Line, c.Column))
				continue
			}
		}
		safe, ok := manifestSafeScalar(raw)
		if !ok {
			diags = append(diags, diagnostic.Warning(diagnostic.CodeImportUnsupportedField,
				field+": argv element with Compose interpolation omitted (manifest cannot store ${…})").
				WithSource(file, pointer, c.Line, c.Column))
			continue
		}
		if strings.TrimSpace(safe) == "" {
			if raw == "--logfile" && i+1 < len(n.Seq) {
				skipNext = true
			}
			continue
		}
		if raw == "--logfile" && i+1 < len(n.Seq) {
			if strings.TrimSpace(loaderSeqString(n.Seq[i+1])) == "" {
				skipNext = true
				continue
			}
		}
		out = append(out, safe)
	}
	if len(out) == 0 {
		return nil, diags
	}
	return out, diags
}

func loaderSeqString(c *loader.Node) string {
	if c == nil {
		return ""
	}
	switch c.Kind {
	case loader.KindString, loader.KindNumber:
		return c.Str
	case loader.KindBool:
		if c.Str != "" {
			return c.Str
		}
		if c.Bool {
			return "true"
		}
		return "false"
	default:
		return ""
	}
}

func composeInterpName(s string) string {
	i := strings.Index(s, "${")
	if i < 0 {
		return ""
	}
	rest := s[i+2:]
	end := strings.IndexAny(rest, ":}")
	if end < 0 {
		return rest
	}
	return rest[:end]
}

func healthcheckUnresolved(t *loader.Node, file, pointer, sid string) []diagnostic.Diagnostic {
	if t == nil {
		return nil
	}
	for _, c := range t.Seq {
		raw := loaderSeqString(c)
		unfolded, interp := unfoldComposeDollars(raw)
		if c != nil && c.Kind == loader.KindString && c.Str == "" {
			return []diagnostic.Diagnostic{diagnostic.Error(diagnostic.CodeImportInterpolation,
				fmt.Sprintf("service %s: healthcheck argv element is empty", sid)).
				WithSource(file, pointer+"/healthcheck/test", c.Line, c.Column)}
		}
		if !interp {
			continue
		}
		name := composeInterpName(unfolded)
		if name == "" {
			name = raw
		}
		def, has := composeSingleInterpDefault(unfolded)
		if !has || strings.TrimSpace(def) == "" {
			return []diagnostic.Diagnostic{diagnostic.Error(diagnostic.CodeImportInterpolation,
				fmt.Sprintf("service %s: healthcheck parameter %q has no value", sid, name)).
				WithSource(file, pointer+"/healthcheck/test", c.Line, c.Column)}
		}
	}
	return nil
}

// importHealthcheckCommand maps Compose healthcheck.test to manifest argv (no CMD/CMD-SHELL prefix).
func importHealthcheckCommand(
	t *loader.Node,
	file, pointer, sid string,
) ([]any, []diagnostic.Diagnostic) {
	if t == nil || t.Kind != loader.KindSeq {
		return nil, nil
	}
	if blocked := healthcheckUnresolved(t, file, pointer, sid); len(blocked) > 0 {
		return nil, blocked
	}
	arr, diags := manifestSafeArgvList(t, file, pointer+"/test", "service "+sid+" healthcheck")
	if len(arr) == 0 {
		return redisHealthcheckWithoutAuth(t, file, pointer, sid, diags)
	}
	cmd := append([]any(nil), arr...)
	if len(cmd) > 0 {
		switch cmd[0] {
		case "NONE":
			diags = append(diags, diagnostic.Warning(diagnostic.CodeImportUnsupportedField,
				fmt.Sprintf("service %s: healthcheck NONE omitted", sid)).
				WithSource(file, pointer+"/healthcheck", 0, 0))
			return nil, diags
		case "CMD":
			rest := cmd[1:]
			if len(rest) > 0 {
				return rest, diags
			}
			return redisHealthcheckWithoutAuth(t, file, pointer, sid, diags)
		case "CMD-SHELL":
			rest := cmd[1:]
			if len(rest) == 0 {
				return redisHealthcheckWithoutAuth(t, file, pointer, sid, diags)
			}
			shell, ok := manifestHealthcheckShellLine(rest)
			if !ok {
				return nil, diags
			}
			return []any{"/bin/sh", "-c", shell}, diags
		}
	}
	return cmd, diags
}

func redisHealthcheckWithoutAuth(
	t *loader.Node,
	file, pointer, sid string,
	diags []diagnostic.Diagnostic,
) ([]any, []diagnostic.Diagnostic) {
	if len(t.Seq) < 2 {
		return nil, diags
	}
	if loaderSeqString(t.Seq[0]) != "CMD-SHELL" {
		return nil, diags
	}
	shell := loaderSeqString(t.Seq[1])
	if !strings.Contains(shell, "redis-cli") || !strings.Contains(shell, "${") {
		return nil, diags
	}
	diags = append(diags, diagnostic.Warning(diagnostic.CodeImportUnsupportedField,
		fmt.Sprintf("service %s: healthcheck uses redis-cli without auth (Compose password omitted)", sid)).
		WithSource(file, pointer+"/healthcheck/test", t.Seq[1].Line, t.Seq[1].Column))
	return []any{"redis-cli", "ping"}, diags
}

func manifestHealthcheckShellLine(rest []any) (script string, ok bool) {
	if len(rest) == 0 {
		return "", false
	}
	if len(rest) == 1 {
		s, _ := rest[0].(string)
		s = strings.TrimSpace(s)
		if s == "" {
			return "", false
		}
		return s, true
	}
	parts := make([]string, 0, len(rest))
	for _, r := range rest {
		s, _ := r.(string)
		s = strings.TrimSpace(s)
		if s == "" {
			continue
		}
		parts = append(parts, s)
	}
	if len(parts) == 0 {
		return "", false
	}
	return strings.Join(parts, " "), true
}
