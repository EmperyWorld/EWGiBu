package workspaceingest

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"io"
	"strings"

	"iacpack/internal/diagnostic"
)

func extractTarGz(data []byte, lim Limits) ([]FilePart, []diagnostic.Diagnostic) {
	if int64(len(data)) > lim.MaxTotalBytes {
		return nil, []diagnostic.Diagnostic{diagnostic.Error(diagnostic.CodeInputIOError, "archive exceeds size limit")}
	}
	gz, err := gzip.NewReader(bytes.NewReader(data))
	if err != nil {
		return nil, []diagnostic.Diagnostic{diagnostic.Error(diagnostic.CodeInputIOError, "gzip: "+err.Error())}
	}
	defer gz.Close()
	tr := tar.NewReader(gz)
	var parts []FilePart
	var total int64
	seen := map[string]struct{}{}
	n := 0
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, []diagnostic.Diagnostic{diagnostic.Error(diagnostic.CodeInputIOError, "tar: "+err.Error())}
		}
		n++
		if n > lim.MaxFiles {
			return nil, []diagnostic.Diagnostic{diagnostic.Error(diagnostic.CodeInputIOError, "archive has too many entries")}
		}
		name := filepathToSlash(hdr.Name)
		if name == "" || strings.HasSuffix(name, "/") || hdr.Typeflag == tar.TypeDir {
			continue
		}
		if hdr.Typeflag != tar.TypeReg && hdr.Typeflag != tar.TypeRegA {
			return nil, []diagnostic.Diagnostic{diagnostic.Error(diagnostic.CodeImportPolicyDenied, "tar links and special files are forbidden: "+name)}
		}
		if err := validateMemberPath(name, lim.MaxPathBytes); err != nil {
			return nil, []diagnostic.Diagnostic{diagnostic.Error(diagnostic.CodeImportPathInvalid, err.Error())}
		}
		if _, ok := seen[name]; ok {
			return nil, []diagnostic.Diagnostic{diagnostic.Error(diagnostic.CodeImportCollision, "duplicate tar entry "+name)}
		}
		seen[name] = struct{}{}
		if hdr.Size < 0 || hdr.Size > lim.MaxFileBytes {
			return nil, []diagnostic.Diagnostic{diagnostic.Error(diagnostic.CodeInputIOError, "tar entry too large: "+name)}
		}
		body, err := io.ReadAll(io.LimitReader(tr, lim.MaxFileBytes+1))
		if err != nil {
			return nil, []diagnostic.Diagnostic{diagnostic.Error(diagnostic.CodeInputIOError, err.Error())}
		}
		if int64(len(body)) > lim.MaxFileBytes {
			return nil, []diagnostic.Diagnostic{diagnostic.Error(diagnostic.CodeInputIOError, "tar entry too large: "+name)}
		}
		total += int64(len(body))
		if total > lim.MaxTotalBytes {
			return nil, []diagnostic.Diagnostic{diagnostic.Error(diagnostic.CodeInputIOError, "archive expanded size exceeds limit")}
		}
		parts = append(parts, FilePart{Path: name, Data: body})
	}
	return parts, nil
}
