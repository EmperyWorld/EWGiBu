// Package params resolves explicit non-secret build/deploy parameters.
package params

import (
	"encoding/json"
	"fmt"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"iacpack/internal/diagnostic"
	domain "iacpack/internal/domain/manifest"
)

// Stable PARAM.* / INPUT.* codes for parameter resolution.
const (
	CodeParamMissing       = "PARAM.MISSING"
	CodeParamInvalid       = "PARAM.INVALID_VALUE"
	CodeParamUnknown       = "PARAM.UNKNOWN"
	CodeParamSecretLike    = "PARAM.SECRET_LIKE"
	CodeParamPhaseMismatch = "PARAM.PHASE_MISMATCH"
	CodeValuesParse        = "PARAM.VALUES_PARSE"
)

// Source captures where a resolved value came from (never ambient env).
type Source string

const (
	SourceCLISet  Source = "set"
	SourceValues  Source = "values"
	SourceDefault Source = "default"
)

// Resolved is one typed parameter value after precedence resolution.
type Resolved struct {
	ID     string
	Phase  domain.ParameterPhase
	Value  domain.Scalar
	Source Source
}

// Options are explicit resolution inputs. Ambient environment is never read.
type Options struct {
	// Set is --set name=value pairs (already parsed); later duplicates override earlier.
	Set map[string]string
	// Values is the decoded --values object (string keys only).
	Values map[string]any
}

// ResolveBuild resolves build-phase parameters for lock/validate/compile.
func ResolveBuild(project domain.Project, opts Options) (map[string]Resolved, []diagnostic.Diagnostic) {
	return resolvePhase(project, domain.ParameterPhaseBuild, opts)
}

// ResolveDeploy resolves deploy-phase parameters (plan/apply; unused by lock).
func ResolveDeploy(project domain.Project, opts Options) (map[string]Resolved, []diagnostic.Diagnostic) {
	return resolvePhase(project, domain.ParameterPhaseDeploy, opts)
}

func resolvePhase(project domain.Project, phase domain.ParameterPhase, opts Options) (map[string]Resolved, []diagnostic.Diagnostic) {
	var diags []diagnostic.Diagnostic
	if opts.Values != nil {
		diags = append(diags, rejectSecretLikeKeys(opts.Values)...)
	}
	out := map[string]Resolved{}
	ids := make([]string, 0, len(project.Spec.Parameters))
	for id, p := range project.Spec.Parameters {
		if p.Phase != phase {
			continue
		}
		ids = append(ids, id)
	}
	sort.Strings(ids)

	provided := map[string]bool{}
	for id := range opts.Set {
		provided[id] = true
	}
	for id := range opts.Values {
		provided[id] = true
	}
	for id := range provided {
		p, ok := project.Spec.Parameters[id]
		if !ok {
			diags = append(diags, diagnostic.Error(CodeParamUnknown,
				fmt.Sprintf("unknown parameter %q", id)))
			continue
		}
		if p.Phase != phase {
			// Allow deploy keys in values file when resolving build? No — reject
			// cross-phase explicit overrides for the phase being resolved.
			if phase == domain.ParameterPhaseBuild && p.Phase == domain.ParameterPhaseDeploy {
				// Deploy values may appear in the same file; ignore for build resolve.
				continue
			}
			diags = append(diags, diagnostic.Error(CodeParamPhaseMismatch,
				fmt.Sprintf("parameter %q has phase %s, not %s", id, p.Phase, phase)))
		}
	}

	for _, id := range ids {
		p := project.Spec.Parameters[id]
		raw, src, ok := pick(id, opts, p)
		if !ok {
			diags = append(diags, diagnostic.Error(CodeParamMissing,
				fmt.Sprintf("%s parameter %q has no value (--set, --values, or default)", phase, id)).
				WithSource("", "/spec/parameters/"+id, 0, 0))
			continue
		}
		sc, err := coerce(p, raw)
		if err != nil {
			diags = append(diags, diagnostic.Error(CodeParamInvalid,
				fmt.Sprintf("parameter %q: %v", id, err)).
				WithSource("", "/spec/parameters/"+id, 0, 0))
			continue
		}
		if err := checkConstraints(p, sc); err != nil {
			diags = append(diags, diagnostic.Error(CodeParamInvalid,
				fmt.Sprintf("parameter %q: %v", id, err)).
				WithSource("", "/spec/parameters/"+id, 0, 0))
			continue
		}
		out[id] = Resolved{ID: id, Phase: phase, Value: sc, Source: src}
	}
	diagnostic.SortDeterministic(diags)
	return out, diags
}

func pick(id string, opts Options, p domain.Parameter) (any, Source, bool) {
	if v, ok := opts.Set[id]; ok {
		return v, SourceCLISet, true
	}
	if opts.Values != nil {
		if v, ok := opts.Values[id]; ok {
			return v, SourceValues, true
		}
	}
	if p.Default != nil {
		return scalarToAny(*p.Default), SourceDefault, true
	}
	return nil, "", false
}

func scalarToAny(s domain.Scalar) any {
	switch s.Kind {
	case domain.ScalarString:
		return s.Str
	case domain.ScalarNumber:
		return json.Number(s.Str)
	case domain.ScalarBool:
		return s.Bool
	default:
		return nil
	}
}

func coerce(p domain.Parameter, raw any) (domain.Scalar, error) {
	switch p.Type {
	case domain.ParameterTypeString, domain.ParameterTypeEnum:
		s, err := asString(raw)
		if err != nil {
			return domain.Scalar{}, err
		}
		return domain.Scalar{Kind: domain.ScalarString, Str: s}, nil
	case domain.ParameterTypeBoolean:
		b, err := asBool(raw)
		if err != nil {
			return domain.Scalar{}, err
		}
		return domain.Scalar{Kind: domain.ScalarBool, Bool: b}, nil
	case domain.ParameterTypeInteger:
		n, err := asNumber(raw)
		if err != nil {
			return domain.Scalar{}, err
		}
		if strings.ContainsAny(n, ".eE") {
			f, err := strconv.ParseFloat(n, 64)
			if err != nil || f != float64(int64(f)) {
				return domain.Scalar{}, fmt.Errorf("expected integer")
			}
			n = strconv.FormatInt(int64(f), 10)
		}
		return domain.Scalar{Kind: domain.ScalarNumber, Str: n, Num: mustFloat(n)}, nil
	case domain.ParameterTypeNumber:
		n, err := asNumber(raw)
		if err != nil {
			return domain.Scalar{}, err
		}
		return domain.Scalar{Kind: domain.ScalarNumber, Str: n, Num: mustFloat(n)}, nil
	default:
		return domain.Scalar{}, fmt.Errorf("unsupported type %s", p.Type)
	}
}

func checkConstraints(p domain.Parameter, sc domain.Scalar) error {
	switch p.Type {
	case domain.ParameterTypeEnum:
		for _, c := range p.Choices {
			if scalarEqual(c, sc) {
				return nil
			}
		}
		return fmt.Errorf("value not in choices")
	case domain.ParameterTypeString:
		if p.Pattern != "" {
			re, err := regexp.Compile(p.Pattern)
			if err != nil {
				return fmt.Errorf("invalid pattern")
			}
			if !re.MatchString(sc.Str) {
				return fmt.Errorf("does not match pattern")
			}
		}
	case domain.ParameterTypeInteger, domain.ParameterTypeNumber:
		if p.Minimum != nil && sc.Num < *p.Minimum {
			return fmt.Errorf("below minimum")
		}
		if p.Maximum != nil && sc.Num > *p.Maximum {
			return fmt.Errorf("above maximum")
		}
	}
	return nil
}

func scalarEqual(a, b domain.Scalar) bool {
	if a.Kind != b.Kind {
		return false
	}
	switch a.Kind {
	case domain.ScalarString:
		return a.Str == b.Str
	case domain.ScalarNumber:
		return a.Str == b.Str || a.Num == b.Num
	case domain.ScalarBool:
		return a.Bool == b.Bool
	default:
		return false
	}
}

func asString(v any) (string, error) {
	switch t := v.(type) {
	case string:
		return t, nil
	case json.Number:
		return t.String(), nil
	case float64:
		return strconv.FormatFloat(t, 'f', -1, 64), nil
	case bool:
		return strconv.FormatBool(t), nil
	default:
		return "", fmt.Errorf("expected string")
	}
}

func asBool(v any) (bool, error) {
	switch t := v.(type) {
	case bool:
		return t, nil
	case string:
		if strings.TrimSpace(t) == "" {
			return false, fmt.Errorf("expected boolean, got empty string")
		}
		switch strings.ToLower(t) {
		case "true", "yes", "1":
			return true, nil
		case "false", "no", "0":
			return false, nil
		default:
			return false, fmt.Errorf("expected boolean")
		}
	default:
		return false, fmt.Errorf("expected boolean")
	}
}

func asNumber(v any) (string, error) {
	switch t := v.(type) {
	case json.Number:
		return t.String(), nil
	case float64:
		return strconv.FormatFloat(t, 'f', -1, 64), nil
	case int:
		return strconv.Itoa(t), nil
	case int64:
		return strconv.FormatInt(t, 10), nil
	case string:
		if strings.TrimSpace(t) == "" {
			return "", fmt.Errorf("expected number, got empty string")
		}
		if _, err := strconv.ParseFloat(t, 64); err != nil {
			return "", fmt.Errorf("expected number")
		}
		return t, nil
	default:
		return "", fmt.Errorf("expected number")
	}
}

func mustFloat(s string) float64 {
	f, _ := strconv.ParseFloat(s, 64)
	return f
}

// SecretLikeName reports whether a parameter name is treated as a credential.
// Values for these names are never written into a portable kit.
func SecretLikeName(key string) bool {
	return looksSecretLikeKey(key)
}

func looksSecretLikeKey(key string) bool {
	segs := nameSegments(key)
	for i, seg := range segs {
		switch seg {
		case "secret", "password", "passwd", "credential":
			return true
		case "token":
			// "tokens" is a different word (max-tokens). token-ttl is a duration, not a credential.
			if i+1 < len(segs) && segs[i+1] == "ttl" {
				continue
			}
			return true
		case "apikey":
			return true
		case "api":
			if i+1 < len(segs) && segs[i+1] == "key" {
				return true
			}
		case "private":
			if i+1 < len(segs) && segs[i+1] == "key" {
				return true
			}
		}
	}
	return false
}

func nameSegments(key string) []string {
	var segs []string
	var b strings.Builder
	flush := func() {
		if b.Len() == 0 {
			return
		}
		segs = append(segs, b.String())
		b.Reset()
	}
	for _, r := range strings.ToLower(key) {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
			b.WriteRune(r)
			continue
		}
		flush()
	}
	flush()
	return segs
}

func rejectSecretLikeKeys(m map[string]any) []diagnostic.Diagnostic {
	var diags []diagnostic.Diagnostic
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		if looksSecretLikeKey(k) {
			diags = append(diags, diagnostic.Error(CodeParamSecretLike,
				fmt.Sprintf("values file key %q looks secret-like and is rejected", k)).
				WithHint("Secrets must use --secret-file, never values files or --set"))
		}
		if nested, ok := m[k].(map[string]any); ok {
			diags = append(diags, rejectSecretLikeKeys(nested)...)
		}
	}
	return diags
}

// ParseSetPairs parses --set name=value flags into a map (last wins).
func ParseSetPairs(pairs []string) (map[string]string, []diagnostic.Diagnostic) {
	out := map[string]string{}
	var diags []diagnostic.Diagnostic
	for _, p := range pairs {
		name, val, ok := strings.Cut(p, "=")
		if !ok || name == "" {
			diags = append(diags, diagnostic.Error(diagnostic.CodeCLIInvalidUsage,
				fmt.Sprintf("invalid --set %q (want name=value)", p)))
			continue
		}
		if looksSecretLikeKey(name) {
			diags = append(diags, diagnostic.Error(CodeParamSecretLike,
				fmt.Sprintf("--set key %q looks secret-like and is rejected", name)))
			continue
		}
		out[name] = val
	}
	return out, diags
}

// ParseValuesJSON parses a values file object from JSON bytes.
func ParseValuesJSON(data []byte) (map[string]any, []diagnostic.Diagnostic) {
	dec := json.NewDecoder(strings.NewReader(string(data)))
	dec.UseNumber()
	var raw any
	if err := dec.Decode(&raw); err != nil {
		return nil, []diagnostic.Diagnostic{diagnostic.Error(CodeValuesParse, "invalid values JSON: "+err.Error())}
	}
	m, ok := raw.(map[string]any)
	if !ok {
		return nil, []diagnostic.Diagnostic{diagnostic.Error(CodeValuesParse, "values file must be a JSON object")}
	}
	return m, nil
}

// PublicMap returns a deterministic map of id -> JSON scalar for lockfiles.
func PublicMap(resolved map[string]Resolved) map[string]any {
	keys := make([]string, 0, len(resolved))
	for k := range resolved {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	out := make(map[string]any, len(keys))
	for _, k := range keys {
		out[k] = scalarToAny(resolved[k].Value)
	}
	return out
}
