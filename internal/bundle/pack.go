package bundle

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"sort"
	"strings"
	"time"

	"iacpack/internal/artifact"
	"iacpack/internal/canonicaljson"
	"iacpack/internal/domain/ir"
	"iacpack/internal/policy"
	"iacpack/internal/schemaembed"
)

// PackOptions configures packing.
type PackOptions struct {
	Deployment ir.Deployment
	Policy     policy.CompilePolicy
	Tree       *artifact.Tree // content tree WITHOUT bundle.json
	HasPreview bool
}

// Pack returns deterministic gzip+USTAR bytes for an .iacbundle.
func Pack(opts PackOptions) ([]byte, error) {
	if opts.Tree == nil {
		return nil, fmt.Errorf("nil tree")
	}
	polBytes, err := policy.CanonicalBytes(opts.Policy)
	if err != nil {
		return nil, err
	}
	polSum := sha256.Sum256(polBytes)
	schemaSum := sha256.Sum256(schemaembed.ManifestV1Alpha1())

	entries := make([]MetaEntry, 0, len(opts.Tree.Paths()))
	for _, e := range opts.Tree.Sorted() {
		entries = append(entries, MetaEntry{
			Path: e.Path, Size: int64(len(e.Content)), Mode: string(e.Mode), SHA256: e.SHA256,
		})
	}
	sort.SliceStable(entries, func(i, j int) bool { return entries[i].Path < entries[j].Path })

	meta := NewMeta(opts.Deployment, opts.Policy, hex.EncodeToString(schemaSum[:]), hex.EncodeToString(polSum[:]), entries, opts.HasPreview)
	raw, err := json.Marshal(meta)
	if err != nil {
		return nil, err
	}
	var m map[string]any
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.UseNumber()
	if err := dec.Decode(&m); err != nil {
		return nil, err
	}
	metaBytes, err := canonicaljson.Marshal(m)
	if err != nil {
		return nil, err
	}
	metaBytes = append(metaBytes, '\n')

	var tarBuf bytes.Buffer
	tw := tar.NewWriter(&tarBuf)

	// Directories implied by paths, emitted in sorted order.
	dirs := map[string]bool{}
	var addDir func(string) error
	addDir = func(p string) error {
		if p == "" || p == "." {
			return nil
		}
		if dirs[p] {
			return nil
		}
		// ensure parents first
		if i := strings.LastIndex(p, "/"); i >= 0 {
			if err := addDir(p[:i]); err != nil {
				return err
			}
		}
		dirs[p] = true
		hdr := &tar.Header{
			Name:     p + "/",
			Mode:     0o755,
			Size:     0,
			ModTime:  time.Unix(0, 0).UTC(),
			Typeflag: tar.TypeDir,
			Uid:      0,
			Gid:      0,
			Uname:    "",
			Gname:    "",
			Format:   tar.FormatUSTAR,
		}
		return tw.WriteHeader(hdr)
	}

	if err := writeFile(tw, "bundle.json", 0o644, metaBytes); err != nil {
		return nil, err
	}
	for _, e := range opts.Tree.Sorted() {
		if i := strings.LastIndex(e.Path, "/"); i >= 0 {
			if err := addDir(e.Path[:i]); err != nil {
				return nil, err
			}
		}
		mode := int64(0o644)
		if e.Mode == artifact.ModeExec {
			mode = 0o755
		}
		if err := writeFile(tw, e.Path, mode, e.Content); err != nil {
			return nil, err
		}
	}
	if err := tw.Close(); err != nil {
		return nil, err
	}

	var gzBuf bytes.Buffer
	zw, err := gzip.NewWriterLevel(&gzBuf, gzip.BestCompression) // level 9
	if err != nil {
		return nil, err
	}
	zw.Name = ""
	zw.Comment = ""
	zw.ModTime = time.Unix(0, 0).UTC()
	zw.OS = 255
	if _, err := io.Copy(zw, bytes.NewReader(tarBuf.Bytes())); err != nil {
		_ = zw.Close()
		return nil, err
	}
	if err := zw.Close(); err != nil {
		return nil, err
	}
	return gzBuf.Bytes(), nil
}

func writeFile(tw *tar.Writer, name string, mode int64, content []byte) error {
	hdr := &tar.Header{
		Name:     name,
		Mode:     mode,
		Size:     int64(len(content)),
		ModTime:  time.Unix(0, 0).UTC(),
		Typeflag: tar.TypeReg,
		Uid:      0,
		Gid:      0,
		Uname:    "",
		Gname:    "",
		Format:   tar.FormatUSTAR,
	}
	if err := tw.WriteHeader(hdr); err != nil {
		return err
	}
	_, err := tw.Write(content)
	return err
}
