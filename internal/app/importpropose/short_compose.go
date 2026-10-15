package importpropose

import (
	"strconv"
	"strings"

	"iacpack/internal/diagnostic"
	"iacpack/internal/domain/path"
)

var volumeModeSuffix = map[string]bool{
	"ro": true, "rw": true, "z": true, "Z": true,
	"delegated": true, "cached": true, "consistent": true,
}

func volumeModeReadOnly(mode string) bool {
	for _, p := range strings.Split(mode, ",") {
		if strings.TrimSpace(p) == "ro" {
			return true
		}
	}
	return false
}

func isVolumeModeSuffix(s string) bool {
	s = strings.TrimSpace(s)
	if s == "" {
		return false
	}
	for _, p := range strings.Split(s, ",") {
		p = strings.TrimSpace(p)
		if p == "" || !volumeModeSuffix[p] {
			return false
		}
	}
	return true
}

// splitVolumeShort parses Compose short volume syntax SRC:TARGET[:MODE].
func splitVolumeShort(s string) (src, tgt, mode string, ok bool) {
	s = strings.TrimSpace(s)
	if s == "" {
		return "", "", "", false
	}
	parts := splitOutsideInterpolation(s, ':')
	if len(parts) < 2 {
		return "", "", "", false
	}
	if len(parts) >= 3 && isVolumeModeSuffix(parts[len(parts)-1]) {
		mode = parts[len(parts)-1]
		parts = parts[:len(parts)-1]
	}
	if len(parts) != 2 {
		return "", "", "", false
	}
	src = strings.TrimSpace(parts[0])
	tgt = strings.TrimSpace(parts[1])
	if src == "" || tgt == "" {
		return "", "", "", false
	}
	return src, tgt, mode, true
}

func looksLikeBindSource(src string) bool {
	if src == "" {
		return false
	}
	if strings.Contains(src, "${") || strings.Contains(src, "$") {
		return true
	}
	if strings.HasPrefix(src, "/") || strings.HasPrefix(src, ".") {
		return true
	}
	if strings.Contains(src, "/") || strings.Contains(src, "\\") {
		return true
	}
	if strings.Contains(src, "..") {
		return true
	}
	return false
}

func mapShortPortString(
	s, file, pointer string,
	parameters map[string]any,
	line, col int,
) (map[string]any, []diagnostic.Diagnostic) {
	var diags []diagnostic.Diagnostic
	raw := strings.TrimSpace(s)
	proto := "tcp"
	if i := strings.LastIndex(raw, "/"); i > 0 {
		suffix := strings.ToLower(raw[i+1:])
		if suffix == "udp" || suffix == "tcp" {
			proto = suffix
			raw = raw[:i]
		}
	}
	out := map[string]any{}
	parts := splitOutsideInterpolation(raw, ':')
	switch len(parts) {
	case 0:
		return nil, diags
	case 1:
		if interpRe.MatchString(raw) {
			diags = append(diags, diagnostic.Warning(diagnostic.CodeImportUnsupportedField,
				"string-form port with interpolated container port omitted").
				WithSource(file, pointer, line, col))
			return nil, diags
		}
		if v, err := strconv.Atoi(strings.TrimSpace(parts[0])); err == nil {
			out["container"] = v
			out["host"] = v
		}
	default:
		contPart := strings.TrimSpace(parts[len(parts)-1])
		cont, err := strconv.Atoi(contPart)
		if err != nil {
			diags = append(diags, diagnostic.Warning(diagnostic.CodeImportUnsupportedField,
				"string-form port omitted; container port must be numeric").
				WithSource(file, pointer, line, col))
			return nil, diags
		}
		out["container"] = cont
		switch len(parts) {
		case 2:
			mapHostPortPart(strings.TrimSpace(parts[0]), out, parameters, file, pointer, line, col, &diags)
		default:
			bindPart := strings.TrimSpace(parts[0])
			if lit, ok := resolveImportLiteralScalar(bindPart); ok && lit != "" {
				out["bind"] = lit
			} else if bindPart != "" && !interpRe.MatchString(bindPart) {
				out["bind"] = bindPart
			} else if bindPart != "" {
				diags = append(diags, diagnostic.Info(diagnostic.CodeImportInterpolation,
					"bind address interpolation omitted; set bind in the designer or use ${VAR:-literal}").
					WithSource(file, pointer, line, col))
			}
			mapHostPortPart(strings.TrimSpace(parts[1]), out, parameters, file, pointer, line, col, &diags)
		}
	}
	if _, ok := out["container"]; !ok {
		return nil, diags
	}
	if proto != "tcp" {
		out["protocol"] = proto
	}
	return out, diags
}

func mapHostPortPart(
	hostPart string,
	out map[string]any,
	parameters map[string]any,
	file, pointer string,
	line, col int,
	diags *[]diagnostic.Diagnostic,
) {
	if interpRe.MatchString(hostPart) {
		id := sanitizeID(paramIDFromInterp(hostPart))
		if id == "" {
			id = "port"
		}
		ensureParam(parameters, id, "integer")
		out["host"] = map[string]any{"parameter": id}
		*diags = append(*diags, diagnostic.Info(diagnostic.CodeImportInterpolation,
			"published port mapped to parameter "+strconv.Quote(id)).
			WithSource(file, pointer, line, col))
		return
	}
	if v, err := strconv.Atoi(hostPart); err == nil {
		out["host"] = v
	}
}

func mountBindFromSource(
	composeFile, sid, file, pointer, src, tgt string,
	readOnly bool,
	resources, parameters map[string]any,
	strict bool,
) (map[string]any, []diagnostic.Diagnostic) {
	var diags []diagnostic.Diagnostic
	if interpRe.MatchString(src) {
		if lit, ok := resolveImportLiteralScalar(src); ok && strings.TrimSpace(lit) != "" {
			src = strings.TrimSpace(lit)
		} else if id, ok := imageRefParameterID(src); ok {
			ensureParam(parameters, id, "string")
			diags = append(diags, diagnostic.Info(diagnostic.CodeImportInterpolation,
				"service "+sid+": host bind "+src+" mapped to parameter "+strconv.Quote(id)+" (not bundled)").
				WithSource(file, pointer, 0, 0))
			m := map[string]any{
				"target":   tgt,
				"host":     map[string]any{"parameter": id},
				"readOnly": readOnly,
			}
			return m, diags
		} else {
			diags = append(diags, diagnostic.Warning(diagnostic.CodeImportInterpolation,
				"service "+sid+": bind source with unresolved Compose interpolation omitted").
				WithSource(file, pointer, 0, 0))
			return nil, diags
		}
	}
	if strings.HasPrefix(src, "/") {
		sev := diagnostic.SeverityWarning
		if strict {
			sev = diagnostic.SeverityError
		}
		diags = append(diags, diagnostic.Diagnostic{
			Code: diagnostic.CodeImportPolicyDenied, Severity: sev,
			Message: "service " + sid + ": host/absolute bind " + strconv.Quote(src) + " refused",
		}.WithSource(file, pointer, 0, 0))
		return nil, diags
	}
	if strings.Contains(src, "docker.sock") {
		diags = append(diags, diagnostic.Error(diagnostic.CodeImportPolicyDenied,
			"service "+sid+": docker.sock bind refused").
			WithSource(file, pointer, 0, 0))
		return nil, diags
	}
	resolved, rebased, err := resolveComposeWorkspacePath(composeFile, src)
	if err != nil {
		diags = append(diags, diagnostic.Error(diagnostic.CodeImportPathInvalid,
			"service "+sid+": invalid bind source "+strconv.Quote(src)).
			WithSource(file, pointer, 0, 0))
		return nil, diags
	}
	if rebased {
		diags = append(diags, composePathRebaseDiag(file, pointer, src, resolved))
	}
	if strings.Contains(resolved, "..") {
		diags = append(diags, diagnostic.Error(diagnostic.CodeImportPolicyDenied,
			"service "+sid+": host bind "+strconv.Quote(src)+" refused").
			WithSource(file, pointer, 0, 0))
		return nil, diags
	}
	if _, err := path.ParseWorkspacePath(resolved); err != nil {
		diags = append(diags, diagnostic.Error(diagnostic.CodeImportPathInvalid,
			"service "+sid+": invalid bind source "+strconv.Quote(src)).
			WithSource(file, pointer, 0, 0))
		return nil, diags
	}
	if _, err := path.ParseContainerPath(tgt); err != nil {
		diags = append(diags, diagnostic.Error(diagnostic.CodeImportPathInvalid,
			"invalid mount target "+strconv.Quote(tgt)).
			WithSource(file, pointer, 0, 0))
		return nil, diags
	}
	rid := sanitizeID("res-" + sid + "-" + filepathBase(src))
	if rid == "" {
		rid = sanitizeID("res-" + sid)
	}
	entry := resourceEntryForBindSource(resolved)
	resources[rid] = entry
	m := map[string]any{"resource": rid, "target": tgt, "readOnly": readOnly}
	if resourceSourceLooksLikeFile(resolved) {
		m["source"] = filepathBase(resolved)
	}
	return m, diags
}

func filepathBase(s string) string {
	s = strings.TrimSuffix(s, "/")
	if i := strings.LastIndexAny(s, "/\\"); i >= 0 {
		return s[i+1:]
	}
	return s
}
