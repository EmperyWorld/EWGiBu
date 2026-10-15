package workspaceingest

import (
	"archive/zip"
	"bytes"
	"io"
	"strings"

	"iacpack/internal/diagnostic"
)

func extractZip(data []byte, lim Limits) ([]FilePart, []diagnostic.Diagnostic) {
	if int64(len(data)) > lim.MaxTotalBytes {
		return nil, []diagnostic.Diagnostic{diagnostic.Error(diagnostic.CodeInputIOError, "zip exceeds size limit")}
	}
	zr, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return nil, []diagnostic.Diagnostic{diagnostic.Error(diagnostic.CodeInputIOError, "zip: "+err.Error())}
	}
	if len(zr.File) > lim.MaxFiles {
		return nil, []diagnostic.Diagnostic{diagnostic.Error(diagnostic.CodeInputIOError, "zip has too many entries")}
	}
	var parts []FilePart
	var total int64
	seen := map[string]struct{}{}
	for _, f := range zr.File {
		name := filepathToSlash(f.Name)
		if strings.HasSuffix(name, "/") {
			continue
		}
		if err := validateMemberPath(name, lim.MaxPathBytes); err != nil {
			return nil, []diagnostic.Diagnostic{diagnostic.Error(diagnostic.CodeImportPathInvalid, err.Error())}
		}
		if f.FileInfo().Mode()&0o120000 != 0 {
			return nil, []diagnostic.Diagnostic{diagnostic.Error(diagnostic.CodeImportPolicyDenied, "zip symlinks/devices forbidden")}
		}
		if _, ok := seen[name]; ok {
			return nil, []diagnostic.Diagnostic{diagnostic.Error(diagnostic.CodeImportCollision, "duplicate zip entry "+name)}
		}
		seen[name] = struct{}{}
		if f.UncompressedSize64 > uint64(lim.MaxFileBytes) {
			return nil, []diagnostic.Diagnostic{diagnostic.Error(diagnostic.CodeInputIOError, "zip entry too large: "+name)}
		}
		rc, err := f.Open()
		if err != nil {
			return nil, []diagnostic.Diagnostic{diagnostic.Error(diagnostic.CodeInputIOError, err.Error())}
		}
		body, err := io.ReadAll(io.LimitReader(rc, lim.MaxFileBytes+1))
		_ = rc.Close()
		if err != nil {
			return nil, []diagnostic.Diagnostic{diagnostic.Error(diagnostic.CodeInputIOError, err.Error())}
		}
		if int64(len(body)) > lim.MaxFileBytes {
			return nil, []diagnostic.Diagnostic{diagnostic.Error(diagnostic.CodeInputIOError, "zip entry too large: "+name)}
		}
		total += int64(len(body))
		if total > lim.MaxTotalBytes {
			return nil, []diagnostic.Diagnostic{diagnostic.Error(diagnostic.CodeInputIOError, "zip expanded size exceeds limit")}
		}
		parts = append(parts, FilePart{Path: name, Data: body})
	}
	return parts, nil
}

func filepathToSlash(s string) string {
	return strings.TrimPrefix(strings.ReplaceAll(s, "\\", "/"), "/")
}
