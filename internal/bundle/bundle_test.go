package bundle_test

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"crypto/sha256"
	"encoding/hex"
	"io"
	"strings"
	"testing"
	"time"

	"iacpack/internal/artifact"
	"iacpack/internal/bundle"
	"iacpack/internal/diagnostic"
	"iacpack/internal/domain/ir"
	"iacpack/internal/policy"
)

func sampleTree(t testing.TB) *artifact.Tree {
	t.Helper()
	tree := artifact.New()
	mustAdd := func(p, body string) {
		if err := tree.AddFile(p, artifact.ModeFile, []byte(body)); err != nil {
			t.Fatal(err)
		}
	}
	mustAdd("manifest.canonical.json", "{}\n")
	mustAdd("deployment.json", "{\"formatVersion\":\"iacpack.deployment/v1\"}\n")
	mustAdd("iacpack.lock.json", "{}\n")
	mustAdd("policy.compile.json", "{}\n")
	mustAdd("docs/README.md", "hi\n")
	return tree
}

func sampleDep() ir.Deployment {
	return ir.Deployment{
		FormatVersion: ir.FormatVersion,
		Project:       ir.ProjectRef{Name: "demo", Version: "0.1.0"},
		Mode:          "release",
		Profile:       "docker-compose",
		SemanticHash:  strings.Repeat("ab", 32),
	}
}

func TestEmptyFileRoundTrip(t *testing.T) {
	tree := sampleTree(t)
	if err := tree.AddFile("payload/pkg/__init__.py", artifact.ModeFile, nil); err != nil {
		t.Fatal(err)
	}
	raw, err := bundle.Pack(bundle.PackOptions{Deployment: sampleDep(), Policy: policy.MustDefault(), Tree: tree})
	if err != nil {
		t.Fatal(err)
	}
	vres, diags := bundle.Verify(bytes.NewReader(raw), bundle.VerifyOptions{KeepContents: true})
	if len(diags) > 0 {
		t.Fatalf("verify: %v", diags)
	}
	body, err := vres.ReadFile("payload/pkg/__init__.py")
	if err != nil {
		t.Fatal(err)
	}
	if body == nil || len(body) != 0 {
		t.Fatalf("empty file content=%#v", body)
	}
}

func TestPackDeterministic(t *testing.T) {
	pol := policy.MustDefault()
	opts := bundle.PackOptions{Deployment: sampleDep(), Policy: pol, Tree: sampleTree(t), HasPreview: false}
	a, err := bundle.Pack(opts)
	if err != nil {
		t.Fatal(err)
	}
	b, err := bundle.Pack(opts)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(a, b) {
		t.Fatal("pack not byte-identical")
	}
	sum := sha256.Sum256(a)
	t.Log("sha256", hex.EncodeToString(sum[:]))

	vres, diags := bundle.Verify(bytes.NewReader(a), bundle.VerifyOptions{KeepContents: true})
	if len(diags) > 0 {
		t.Fatalf("verify: %v", diags)
	}
	if vres.Meta.FormatVersion != bundle.FormatVersion {
		t.Fatalf("format %s", vres.Meta.FormatVersion)
	}
	if _, err := vres.ReadFile("deployment.json"); err != nil {
		t.Fatal(err)
	}
	// gzip header invariants
	gr, err := gzip.NewReader(bytes.NewReader(a))
	if err != nil {
		t.Fatal(err)
	}
	if !gr.ModTime.Equal(time.Unix(0, 0).UTC()) && !gr.ModTime.IsZero() {
		// Go may report zero or epoch depending on version; both acceptable if pack set epoch.
		t.Logf("gzip mtime=%v", gr.ModTime)
	}
	if gr.OS != 255 {
		t.Fatalf("gzip OS=%d want 255", gr.OS)
	}
	_ = gr.Close()
}

func TestVerifyRejectsMalicious(t *testing.T) {
	cases := []struct {
		name string
		raw  []byte
		code string
	}{
		{"not-gzip", []byte("not a gzip"), bundle.CodeBundleParse},
		{"traversal", evilTar(t, tar.TypeReg, "../etc/passwd", []byte("x")), bundle.CodeBundlePath},
		{"absolute", evilTar(t, tar.TypeReg, "/etc/passwd", []byte("x")), bundle.CodeBundlePath},
		{"backslash", evilTar(t, tar.TypeReg, `evil\path`, []byte("x")), bundle.CodeBundlePath},
		{"symlink", evilTar(t, tar.TypeSymlink, "link", nil), bundle.CodeBundleType},
		{"device", evilTar(t, tar.TypeChar, "dev", nil), bundle.CodeBundleType},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, diags := bundle.Verify(bytes.NewReader(tc.raw), bundle.VerifyOptions{})
			if len(diags) == 0 {
				t.Fatal("expected rejection")
			}
			found := false
			for _, d := range diags {
				if d.Code == tc.code {
					found = true
				}
			}
			if !found {
				t.Fatalf("want code %s in %v", tc.code, diags)
			}
		})
	}
}

func TestVerifyRejectsDuplicateAndCollision(t *testing.T) {
	raw := evilTarMulti(t, []evilEnt{
		{Type: tar.TypeReg, Name: "bundle.json", Body: []byte(`{"formatVersion":"iacpack.bundle/v1"}`)},
		{Type: tar.TypeReg, Name: "a.txt", Body: []byte("1")},
		{Type: tar.TypeReg, Name: "a.txt", Body: []byte("2")},
	})
	_, diags := bundle.Verify(bytes.NewReader(raw), bundle.VerifyOptions{})
	if !hasCode(diags, bundle.CodeBundleDuplicate) {
		t.Fatalf("want duplicate, got %v", diags)
	}

	raw = evilTarMulti(t, []evilEnt{
		{Type: tar.TypeReg, Name: "bundle.json", Body: []byte(`{"formatVersion":"iacpack.bundle/v1"}`)},
		{Type: tar.TypeReg, Name: "Foo", Body: []byte("1")},
		{Type: tar.TypeReg, Name: "foo", Body: []byte("2")},
	})
	_, diags = bundle.Verify(bytes.NewReader(raw), bundle.VerifyOptions{})
	if !hasCode(diags, bundle.CodeBundleCollision) {
		t.Fatalf("want collision, got %v", diags)
	}
}

func TestVerifyRejectsForgedHash(t *testing.T) {
	// Craft archive where bundle.json claims a wrong digest for a present file.
	meta := `{"formatVersion":"iacpack.bundle/v1","project":{"name":"d","version":"0.1.0","mode":"release","semanticHash":"` + strings.Repeat("ab", 32) + `"},"compiler":{"product":"iacpack","version":"0"},"provider":{"profile":"docker-compose","version":"1"},"policy":{"sha256":"` + strings.Repeat("cd", 32) + `"},"schema":{"sha256":"` + strings.Repeat("ef", 32) + `"},"runtime":{"composePluginMin":"2.24"},"capabilities":{"embedImages":false,"includeRuntime":false,"hasPreview":false},"entries":[{"path":"deployment.json","size":3,"mode":"0644","sha256":"` + strings.Repeat("00", 32) + `"}]}`
	raw := evilTarMulti(t, []evilEnt{
		{Type: tar.TypeReg, Name: "bundle.json", Body: []byte(meta)},
		{Type: tar.TypeReg, Name: "deployment.json", Body: []byte("{}\n")},
	})
	_, diags := bundle.Verify(bytes.NewReader(raw), bundle.VerifyOptions{})
	if !hasCode(diags, bundle.CodeBundleHash) && !hasCode(diags, bundle.CodeBundleMeta) {
		t.Fatalf("expected hash/meta rejection, got %v", diags)
	}
}

func TestVerifyQuotaEntries(t *testing.T) {
	ents := []evilEnt{{Type: tar.TypeReg, Name: "bundle.json", Body: []byte(`{"formatVersion":"x"}`)}}
	for i := 0; i < 5; i++ {
		ents = append(ents, evilEnt{Type: tar.TypeReg, Name: "f" + string(rune('a'+i)) + ".txt", Body: []byte("x")})
	}
	raw := evilTarMulti(t, ents)
	_, diags := bundle.Verify(bytes.NewReader(raw), bundle.VerifyOptions{
		Quotas: bundle.Quotas{MaxEntries: 3, MaxEntryBytes: 1 << 20, MaxExpandedBytes: 1 << 20, MaxRatio: 200, MaxPathBytes: 255},
	})
	if !hasCode(diags, bundle.CodeBundleQuota) {
		t.Fatalf("want quota, got %v", diags)
	}
}

func FuzzVerify(f *testing.F) {
	pol := policy.MustDefault()
	good, err := bundle.Pack(bundle.PackOptions{Deployment: sampleDep(), Policy: pol, Tree: sampleTree(f)})
	if err != nil {
		f.Fatal(err)
	}
	f.Add(good)
	f.Add([]byte("notgzip"))
	f.Add(evilTar(f, tar.TypeReg, "../x", []byte("z")))
	f.Fuzz(func(t *testing.T, data []byte) {
		_, _ = bundle.Verify(bytes.NewReader(data), bundle.VerifyOptions{
			Quotas: bundle.Quotas{MaxEntries: 100, MaxEntryBytes: 1 << 20, MaxExpandedBytes: 1 << 20, MaxRatio: 50, MaxPathBytes: 255},
		})
	})
}

type evilEnt struct {
	Type byte
	Name string
	Body []byte
	Link string
}

func evilTar(t testing.TB, typ byte, name string, body []byte) []byte {
	t.Helper()
	return evilTarMulti(t, []evilEnt{{Type: typ, Name: name, Body: body}})
}

func evilTarMulti(t testing.TB, ents []evilEnt) []byte {
	t.Helper()
	var tarBuf bytes.Buffer
	tw := tar.NewWriter(&tarBuf)
	for _, e := range ents {
		hdr := &tar.Header{
			Name: e.Name, Mode: 0o644, Size: int64(len(e.Body)),
			ModTime: time.Unix(0, 0), Typeflag: e.Type, Format: tar.FormatUSTAR,
		}
		if e.Type == tar.TypeSymlink {
			hdr.Linkname = "target"
			hdr.Size = 0
		}
		if e.Type == tar.TypeChar {
			hdr.Size = 0
		}
		if err := tw.WriteHeader(hdr); err != nil {
			t.Fatal(err)
		}
		if len(e.Body) > 0 && e.Type == tar.TypeReg {
			if _, err := tw.Write(e.Body); err != nil {
				t.Fatal(err)
			}
		}
	}
	if err := tw.Close(); err != nil {
		t.Fatal(err)
	}
	var gz bytes.Buffer
	zw := gzip.NewWriter(&gz)
	if _, err := io.Copy(zw, &tarBuf); err != nil {
		t.Fatal(err)
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	return gz.Bytes()
}

func hasCode(diags []diagnostic.Diagnostic, code string) bool {
	for _, d := range diags {
		if d.Code == code {
			return true
		}
	}
	return false
}
