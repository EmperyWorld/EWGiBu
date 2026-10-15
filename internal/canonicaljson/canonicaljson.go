// Package canonicaljson implements RFC 8785 JSON Canonicalization Scheme (JCS).
package canonicaljson

import (
	"encoding/json"
	"fmt"
	"math"
	"strings"
	"unicode/utf16"
)

// Marshal returns RFC 8785 canonical JSON bytes with no trailing newline.
func Marshal(v any) ([]byte, error) {
	normalized, err := toJSONValue(v)
	if err != nil {
		return nil, err
	}
	var b strings.Builder
	if err := writeValue(&b, normalized); err != nil {
		return nil, err
	}
	return []byte(b.String()), nil
}

func toJSONValue(v any) (any, error) {
	raw, err := json.Marshal(v)
	if err != nil {
		return nil, err
	}
	dec := json.NewDecoder(strings.NewReader(string(raw)))
	dec.UseNumber()
	var out any
	if err := dec.Decode(&out); err != nil {
		return nil, err
	}
	return out, nil
}

func writeValue(b *strings.Builder, v any) error {
	switch t := v.(type) {
	case nil:
		b.WriteString("null")
		return nil
	case bool:
		if t {
			b.WriteString("true")
		} else {
			b.WriteString("false")
		}
		return nil
	case json.Number:
		f, err := t.Float64()
		if err != nil {
			return fmt.Errorf("canonicaljson: invalid number %q", t)
		}
		return writeFloat(b, f)
	case float64:
		return writeFloat(b, t)
	case string:
		return writeString(b, t)
	case []any:
		b.WriteByte('[')
		for i, e := range t {
			if i > 0 {
				b.WriteByte(',')
			}
			if err := writeValue(b, e); err != nil {
				return err
			}
		}
		b.WriteByte(']')
		return nil
	case map[string]any:
		keys := make([]string, 0, len(t))
		for k := range t {
			keys = append(keys, k)
		}
		sortUTF16(keys)
		b.WriteByte('{')
		for i, k := range keys {
			if i > 0 {
				b.WriteByte(',')
			}
			if err := writeString(b, k); err != nil {
				return err
			}
			b.WriteByte(':')
			if err := writeValue(b, t[k]); err != nil {
				return err
			}
		}
		b.WriteByte('}')
		return nil
	default:
		return fmt.Errorf("canonicaljson: unsupported type %T", v)
	}
}

func writeFloat(b *strings.Builder, f float64) error {
	if math.IsNaN(f) || math.IsInf(f, 0) {
		return fmt.Errorf("canonicaljson: NaN/Inf not allowed")
	}
	// encoding/json emits ES-compatible Number.toString forms for finite floats.
	raw, err := json.Marshal(f)
	if err != nil {
		return err
	}
	b.Write(raw)
	return nil
}

func writeString(b *strings.Builder, s string) error {
	raw, err := json.Marshal(s)
	if err != nil {
		return err
	}
	b.Write(raw)
	return nil
}

func sortUTF16(keys []string) {
	// Insertion sort by UTF-16 code units (RFC 8785 §3.2.3).
	for i := 1; i < len(keys); i++ {
		j := i
		for j > 0 && lessUTF16(keys[j], keys[j-1]) {
			keys[j], keys[j-1] = keys[j-1], keys[j]
			j--
		}
	}
}

func lessUTF16(a, b string) bool {
	aa := utf16.Encode([]rune(a))
	bb := utf16.Encode([]rune(b))
	n := len(aa)
	if len(bb) < n {
		n = len(bb)
	}
	for i := 0; i < n; i++ {
		if aa[i] != bb[i] {
			return aa[i] < bb[i]
		}
	}
	return len(aa) < len(bb)
}
