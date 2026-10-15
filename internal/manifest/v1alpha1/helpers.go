package v1alpha1

import (
	"fmt"
	"math"
	"strings"

	"iacpack/internal/diagnostic"
	"iacpack/internal/manifest/loader"
)

func rejectUnknown(file, base string, node *loader.Node, allowed map[string]struct{}) []diagnostic.Diagnostic {
	var diags []diagnostic.Diagnostic
	for _, e := range node.Map {
		if _, ok := allowed[e.Key]; ok {
			continue
		}
		ptr := base + "/" + escape(e.Key)
		if base == "" {
			ptr = "/" + escape(e.Key)
		}
		diags = append(diags, diagnostic.Error(diagnostic.CodeSchemaUnknownField,
			fmt.Sprintf("unknown field %q", e.Key)).
			WithSource(file, ptr, e.KeyLine, e.KeyColumn).
			WithHint("Remove the field or use a supported apiVersion"))
	}
	return diags
}

func requireString(file string, parent *loader.Node, key, pointer string) (string, []diagnostic.Diagnostic) {
	n := parent.GetMap(key)
	if n == nil {
		return "", []diagnostic.Diagnostic{missing(file, pointer, parent)}
	}
	if n.Kind != loader.KindString {
		return "", []diagnostic.Diagnostic{typeErr(file, pointer, "string", n)}
	}
	if n.Str == "" {
		return "", []diagnostic.Diagnostic{
			withLoc(diagnostic.Error(diagnostic.CodeSchemaInvalidValue, key+" must be a non-empty string"),
				file, pointer, n),
		}
	}
	return n.Str, nil
}

func missing(file, pointer string, near *loader.Node) diagnostic.Diagnostic {
	line, col := 1, 1
	if near != nil {
		line, col = near.Line, near.Column
	}
	return diagnostic.Error(diagnostic.CodeSchemaMissingField, "missing required field "+pointer).
		WithSource(file, pointer, line, col)
}

func typeErr(file, pointer, want string, n *loader.Node) diagnostic.Diagnostic {
	line, col := 1, 1
	if n != nil {
		line, col = n.Line, n.Column
	}
	return diagnostic.Error(diagnostic.CodeSchemaInvalidType,
		fmt.Sprintf("field %s must be %s", pointer, want)).
		WithSource(file, pointer, line, col)
}

func loc(n *loader.Node) (line, col int) {
	if n == nil {
		return 1, 1
	}
	return n.Line, n.Column
}

func withLoc(d diagnostic.Diagnostic, file, pointer string, n *loader.Node) diagnostic.Diagnostic {
	line, col := loc(n)
	return d.WithSource(file, pointer, line, col)
}

func escape(s string) string {
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

func hasDigest(ref string) bool {
	const marker = "@sha256:"
	i := strings.LastIndex(ref, marker)
	if i < 0 {
		return false
	}
	dig := ref[i+len(marker):]
	if len(dig) != 64 {
		return false
	}
	for _, c := range dig {
		if !((c >= '0' && c <= '9') || (c >= 'a' && c <= 'f') || (c >= 'A' && c <= 'F')) {
			return false
		}
	}
	return true
}

func isFinite(f float64) bool {
	return !math.IsInf(f, 0) && !math.IsNaN(f)
}
