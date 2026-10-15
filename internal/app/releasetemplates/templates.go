// Package releasetemplates writes opted-in GitHub CI/release helper files (ADR-019).
// Propose never writes; Write refuses overwrite unless Force is set.
// Templates never invoke gh, push, remotes, or network publish.
package releasetemplates

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"iacpack/internal/diagnostic"
	"iacpack/internal/domain/path"
	"iacpack/internal/inputs"
)

const (
	markerBegin = "# BEGIN iacpack"
	markerEnd   = "# END iacpack"

	pathWorkflow      = ".github/workflows/iacpack-ci.yml"
	pathGitignore     = ".gitignore"
	pathGitattributes = ".gitattributes"
	pathDocs          = "docs/iacpack-release.md"
)

// FilePlan describes one planned write.
type FilePlan struct {
	Path   string `json:"path"`
	Action string `json:"action"` // create | replace | append | skip
	SHA256 string `json:"sha256"`
	Bytes  int    `json:"bytes"`
}

// Result is propose/write outcome (no network).
type Result struct {
	OK          bool                    `json:"ok"`
	Force       bool                    `json:"force"`
	Wrote       bool                    `json:"wrote"`
	Files       []FilePlan              `json:"files"`
	Diagnostics []diagnostic.Diagnostic `json:"-"`
}

// Request configures propose or write.
type Request struct {
	Workspace string
	Force     bool
	Write     bool
}

// Propose plans template writes without touching the workspace.
func Propose(req Request) Result {
	return run(req, false)
}

// Write applies template writes (authoring only; no publish).
func Write(req Request) Result {
	req.Write = true
	return run(req, true)
}

func run(req Request, doWrite bool) Result {
	var diags []diagnostic.Diagnostic
	if strings.TrimSpace(req.Workspace) == "" {
		d := diagnostic.Error(diagnostic.CodeInputIOError, "workspace is required")
		return Result{OK: false, Force: req.Force, Diagnostics: []diagnostic.Diagnostic{d}}
	}
	ws := filepath.Clean(req.Workspace)

	bodies, err := loadBodies()
	if err != nil {
		d := diagnostic.Error(diagnostic.CodeInternal, "load templates: "+err.Error())
		return Result{OK: false, Force: req.Force, Diagnostics: []diagnostic.Diagnostic{d}}
	}

	type item struct {
		rel     string
		body    []byte
		mode    string // full | markers
		snippet []byte
	}
	items := []item{
		{rel: pathWorkflow, body: bodies.workflow, mode: "full"},
		{rel: pathDocs, body: bodies.docs, mode: "full"},
		{rel: pathGitignore, mode: "markers", snippet: bodies.gitignore},
		{rel: pathGitattributes, mode: "markers", snippet: bodies.gitattributes},
	}

	var plans []FilePlan
	writes := map[string][]byte{}

	for _, it := range items {
		if _, err := path.ParseWorkspacePath(it.rel); err != nil {
			diags = append(diags, diagnostic.Error(diagnostic.CodeImportPathInvalid, "template path invalid: "+it.rel))
			continue
		}
		if err := inputs.ValidateRelPath(it.rel); err != nil {
			diags = append(diags, diagnostic.Error(diagnostic.CodeImportPathInvalid, "template path invalid: "+it.rel))
			continue
		}
		abs := filepath.Join(ws, filepath.FromSlash(it.rel))
		exists := false
		var cur []byte
		if st, err := os.Lstat(abs); err == nil && !st.IsDir() {
			exists = true
			cur, err = os.ReadFile(abs)
			if err != nil {
				diags = append(diags, diagnostic.Error(diagnostic.CodeInputIOError, "read "+it.rel+": "+err.Error()).
					WithSource(it.rel, "", 0, 0))
				continue
			}
		} else if err != nil && !os.IsNotExist(err) {
			diags = append(diags, diagnostic.Error(diagnostic.CodeInputIOError, "stat "+it.rel+": "+err.Error()).
				WithSource(it.rel, "", 0, 0))
			continue
		}

		var next []byte
		action := "create"
		switch it.mode {
		case "full":
			next = append([]byte(nil), it.body...)
			if exists {
				if bytes.Equal(cur, next) {
					action = "skip"
				} else if req.Force {
					action = "replace"
				} else {
					diags = append(diags, diagnostic.Error(diagnostic.CodeInputIOError,
						fmt.Sprintf("%s exists; pass --force to replace", it.rel)).
						WithSource(it.rel, "", 0, 0))
					continue
				}
			}
		case "markers":
			merged, act, err := mergeMarkers(cur, exists, it.snippet, req.Force)
			if err != nil {
				diags = append(diags, diagnostic.Error(diagnostic.CodeInputIOError, it.rel+": "+err.Error()).
					WithSource(it.rel, "", 0, 0))
				continue
			}
			next = merged
			action = act
		}

		sum := sha256.Sum256(next)
		plans = append(plans, FilePlan{
			Path:   it.rel,
			Action: action,
			SHA256: hex.EncodeToString(sum[:]),
			Bytes:  len(next),
		})
		if action != "skip" {
			writes[it.rel] = next
		}
	}

	sort.SliceStable(plans, func(i, j int) bool { return plans[i].Path < plans[j].Path })

	ok := !diagnostic.HasError(diags)
	res := Result{OK: ok, Force: req.Force, Wrote: false, Files: plans, Diagnostics: diags}
	if !ok || !doWrite {
		return res
	}

	// Deterministic write order
	rels := make([]string, 0, len(writes))
	for rel := range writes {
		rels = append(rels, rel)
	}
	sort.Strings(rels)
	for _, rel := range rels {
		abs := filepath.Join(ws, filepath.FromSlash(rel))
		if err := writeFile(abs, writes[rel]); err != nil {
			diags = append(diags, diagnostic.Error(diagnostic.CodeInputIOError, "write "+rel+": "+err.Error()).
				WithSource(rel, "", 0, 0))
			return Result{OK: false, Force: req.Force, Wrote: false, Files: plans, Diagnostics: diags}
		}
	}
	res.Wrote = true
	res.Diagnostics = diags
	return res
}

func mergeMarkers(cur []byte, exists bool, snippet []byte, force bool) (next []byte, action string, err error) {
	snip := append(bytes.TrimRight(snippet, "\n"), '\n')
	if !exists || len(cur) == 0 {
		return snip, "create", nil
	}
	begin := bytes.Index(cur, []byte(markerBegin))
	end := bytes.Index(cur, []byte(markerEnd))
	if begin >= 0 && end > begin {
		endLine := end + len(markerEnd)
		for endLine < len(cur) && (cur[endLine] == '\n' || cur[endLine] == '\r') {
			endLine++
		}
		existingBlock := cur[begin:endLine]
		if bytes.Equal(bytes.TrimRight(existingBlock, "\r\n"), bytes.TrimRight(snip, "\r\n")) {
			return cur, "skip", nil
		}
		if !force {
			return nil, "", fmt.Errorf("iacpack markers present with different content; pass --force to replace block")
		}
		var b bytes.Buffer
		b.Write(cur[:begin])
		b.Write(snip)
		if len(snip) == 0 || snip[len(snip)-1] != '\n' {
			b.WriteByte('\n')
		}
		b.Write(cur[endLine:])
		return b.Bytes(), "replace", nil
	}
	// Append markers
	var b bytes.Buffer
	b.Write(cur)
	if len(cur) > 0 && cur[len(cur)-1] != '\n' {
		b.WriteByte('\n')
	}
	b.Write(snip)
	return b.Bytes(), "append", nil
}

func writeFile(abs string, data []byte) error {
	dir := filepath.Dir(abs)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(dir, ".iacpack-tpl-*")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	defer func() { _ = os.Remove(tmpName) }()
	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Chmod(0o644); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmpName, abs)
}

type bodies struct {
	workflow      []byte
	docs          []byte
	gitignore     []byte
	gitattributes []byte
}

func loadBodies() (bodies, error) {
	w, err := embedded.ReadFile("data/iacpack-ci.yml")
	if err != nil {
		return bodies{}, err
	}
	d, err := embedded.ReadFile("data/iacpack-release.md")
	if err != nil {
		return bodies{}, err
	}
	g, err := embedded.ReadFile("data/gitignore.snippet")
	if err != nil {
		return bodies{}, err
	}
	a, err := embedded.ReadFile("data/gitattributes.snippet")
	if err != nil {
		return bodies{}, err
	}
	return bodies{workflow: w, docs: d, gitignore: g, gitattributes: a}, nil
}

// TemplateRelPaths is the documented write set (for tests/docs).
func TemplateRelPaths() []string {
	return []string{pathWorkflow, pathGitignore, pathGitattributes, pathDocs}
}
