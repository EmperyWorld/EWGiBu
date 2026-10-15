// Package diff implements semantic comparison of two explicit sources.
package diff

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"iacpack/internal/app/validate"
	"iacpack/internal/app/verify"
	"iacpack/internal/compile"
	"iacpack/internal/diagnostic"
	"iacpack/internal/domain/ir"
	"iacpack/internal/inputs"
	"iacpack/internal/lockfile"
	"iacpack/internal/policy"
)

// FileReader reads files for manifest/lock sources.
type FileReader interface {
	ReadFile(path string) ([]byte, error)
}

// Options configures a semantic diff.
type Options struct {
	From      string
	To        string
	Workspace inputs.FS // optional; used for manifest sources
	Policy    policy.CompilePolicy
}

// Change is one semantic difference.
type Change struct {
	Kind   string `json:"kind"`
	Path   string `json:"path"`
	Detail string `json:"detail,omitempty"`
	Risk   string `json:"risk,omitempty"` // none|destructive|irreversible
}

// Result is the diff outcome.
type Result struct {
	OK          bool
	Diagnostics []diagnostic.Diagnostic
	Model       map[string]any
	Text        string
	Changes     []Change
}

// Run compares --from and --to explicit sources.
func Run(fs FileReader, opts Options) Result {
	if opts.Policy.APIVersion == "" {
		opts.Policy = policy.MustDefault()
	}
	var res Result
	if opts.From == "" || opts.To == "" {
		res.Diagnostics = []diagnostic.Diagnostic{diagnostic.Error(diagnostic.CodeCLIInvalidUsage, "diff requires --from and --to")}
		return finish(res)
	}
	left, ldiags := loadSource(fs, opts.From, opts)
	res.Diagnostics = append(res.Diagnostics, ldiags...)
	right, rdiags := loadSource(fs, opts.To, opts)
	res.Diagnostics = append(res.Diagnostics, rdiags...)
	if diagnostic.HasError(res.Diagnostics) {
		return finish(res)
	}
	changes := compare(left, right)
	res.Changes = changes
	res.OK = true
	kinds := map[string]int{}
	for _, c := range changes {
		kinds[c.Kind]++
	}
	kindKeys := make([]string, 0, len(kinds))
	for k := range kinds {
		kindKeys = append(kindKeys, k)
	}
	sort.Strings(kindKeys)
	summary := map[string]any{}
	for _, k := range kindKeys {
		summary[k] = kinds[k]
	}
	changeMaps := make([]map[string]any, 0, len(changes))
	for _, c := range changes {
		m := map[string]any{"kind": c.Kind, "path": c.Path}
		if c.Detail != "" {
			m["detail"] = c.Detail
		}
		if c.Risk != "" && c.Risk != "none" {
			m["risk"] = c.Risk
		}
		changeMaps = append(changeMaps, m)
	}
	res.Model = map[string]any{
		"from":             opts.From,
		"to":               opts.To,
		"fromSemanticHash": left.SemanticHash,
		"toSemanticHash":   right.SemanticHash,
		"identical":        left.SemanticHash == right.SemanticHash && len(changes) == 0,
		"summary":          summary,
		"changes":          changeMaps,
	}
	var b strings.Builder
	fmt.Fprintf(&b, "from: %s (%s)\n", opts.From, left.SemanticHash)
	fmt.Fprintf(&b, "to: %s (%s)\n", opts.To, right.SemanticHash)
	if len(changes) == 0 {
		b.WriteString("identical: true\n")
	} else {
		fmt.Fprintf(&b, "changes: %d\n", len(changes))
		for _, c := range changes {
			line := "  " + c.Kind + " " + c.Path
			if c.Risk != "" && c.Risk != "none" {
				line += " risk=" + c.Risk
			}
			if c.Detail != "" {
				line += " — " + c.Detail
			}
			b.WriteString(line + "\n")
		}
	}
	res.Text = b.String()
	return finish(res)
}

func finish(res Result) Result {
	diagnostic.SortDeterministic(res.Diagnostics)
	res.OK = res.OK && !diagnostic.HasError(res.Diagnostics)
	return res
}

type snap struct {
	Kind         string
	SemanticHash string
	Deployment   ir.Deployment
	Lock         *lockfile.Document
}

func loadSource(fs FileReader, path string, opts Options) (snap, []diagnostic.Diagnostic) {
	lower := strings.ToLower(path)
	base := filepath.Base(lower)
	switch {
	case strings.HasSuffix(lower, ".iacbundle"):
		data, _, diags := verify.ReadVerifiedFile(path, "deployment.json")
		if len(diags) > 0 {
			return snap{}, diags
		}
		dep, err := compile.UnmarshalDeployment(data)
		if err != nil {
			return snap{}, []diagnostic.Diagnostic{diagnostic.Error(bundleMetaCode(), "deployment.json: "+err.Error())}
		}
		return snap{Kind: "bundle", SemanticHash: dep.SemanticHash, Deployment: dep}, nil
	case base == "iacpack.lock.json" || strings.HasSuffix(lower, ".lock.json"):
		raw, err := fs.ReadFile(path)
		if err != nil {
			return snap{}, []diagnostic.Diagnostic{diagnostic.Error(diagnostic.CodeInputIOError, err.Error())}
		}
		doc, err := lockfile.Parse(raw)
		if err != nil {
			return snap{}, []diagnostic.Diagnostic{diagnostic.Error(lockfile.CodeLockParse, err.Error())}
		}
		return snap{Kind: "lock", SemanticHash: doc.Manifest.SemanticHash, Lock: &doc}, nil
	case strings.HasSuffix(lower, ".yaml") || strings.HasSuffix(lower, ".yml") || base == "iacpack.yaml":
		vres := validate.RunWithOptions(fs, path, validate.Options{
			Policy:    opts.Policy,
			Workspace: opts.Workspace,
		})
		if !vres.OK {
			return snap{}, vres.Diagnostics
		}
		dep, err := compile.ToIR(vres.Project)
		if err != nil {
			return snap{}, []diagnostic.Diagnostic{diagnostic.Error(diagnostic.CodeInternal, err.Error())}
		}
		return snap{Kind: "manifest", SemanticHash: dep.SemanticHash, Deployment: dep}, nil
	case base == "state.json":
		raw, err := fs.ReadFile(path)
		if err != nil {
			return snap{}, []diagnostic.Diagnostic{diagnostic.Error(diagnostic.CodeInputIOError, err.Error())}
		}
		// Minimal state-as-diff-source: compare recorded semantic hash only.
		type active struct {
			SemanticHash string `json:"semanticHash"`
		}
		type doc struct {
			Active *active `json:"active"`
		}
		var d doc
		if err := json.Unmarshal(raw, &d); err != nil || d.Active == nil {
			return snap{}, []diagnostic.Diagnostic{diagnostic.Error(diagnostic.CodeCLIUnsupportedSource,
				"state.json missing active semantic hash")}
		}
		return snap{Kind: "state", SemanticHash: d.Active.SemanticHash}, nil
	default:
		// Try as bundle if file exists and looks like gzip.
		if st, err := os.Stat(path); err == nil && !st.IsDir() {
			if raw, err := os.ReadFile(path); err == nil && len(raw) >= 2 && raw[0] == 0x1f && raw[1] == 0x8b {
				data, _, diags := verify.ReadVerifiedFile(path, "deployment.json")
				if len(diags) > 0 {
					return snap{}, diags
				}
				dep, err := compile.UnmarshalDeployment(data)
				if err != nil {
					return snap{}, []diagnostic.Diagnostic{diagnostic.Error(bundleMetaCode(), err.Error())}
				}
				return snap{Kind: "bundle", SemanticHash: dep.SemanticHash, Deployment: dep}, nil
			}
		}
		return snap{}, []diagnostic.Diagnostic{diagnostic.Error(diagnostic.CodeCLIUnsupportedSource,
			fmt.Sprintf("unsupported diff source %q", path)).
			WithHint("Provide a manifest (.yaml), lockfile, or .iacbundle")}
	}
}

func bundleMetaCode() string { return "BUNDLE.META_INVALID" }

func compare(a, b snap) []Change {
	var out []Change
	if a.SemanticHash != b.SemanticHash {
		out = append(out, Change{Kind: "project", Path: "semanticHash", Detail: a.SemanticHash + " -> " + b.SemanticHash, Risk: "none"})
	}
	if a.Lock != nil || b.Lock != nil {
		out = append(out, compareLocks(a, b)...)
		sortChanges(out)
		return out
	}
	out = append(out, compareMaps("service", keysOf(a.Deployment.Services), keysOf(b.Deployment.Services), a.Deployment.Services, b.Deployment.Services)...)
	out = append(out, compareMaps("image", keysOf(a.Deployment.Images), keysOf(b.Deployment.Images), a.Deployment.Images, b.Deployment.Images)...)
	out = append(out, compareMaps("parameter", keysOf(a.Deployment.Parameters), keysOf(b.Deployment.Parameters), a.Deployment.Parameters, b.Deployment.Parameters)...)
	out = append(out, compareMaps("resource", keysOf(a.Deployment.Resources), keysOf(b.Deployment.Resources), a.Deployment.Resources, b.Deployment.Resources)...)
	out = append(out, compareMaps("network", keysOf(a.Deployment.Networks), keysOf(b.Deployment.Networks), a.Deployment.Networks, b.Deployment.Networks)...)
	out = append(out, compareVolumes(a.Deployment.Volumes, b.Deployment.Volumes)...)
	out = append(out, compareMaps("task", keysOf(a.Deployment.Tasks), keysOf(b.Deployment.Tasks), a.Deployment.Tasks, b.Deployment.Tasks)...)
	sortChanges(out)
	return out
}

func compareLocks(a, b snap) []Change {
	var out []Change
	al, bl := a.Lock, b.Lock
	if al == nil || bl == nil {
		out = append(out, Change{Kind: "lock", Path: "/", Detail: "lock presence changed", Risk: "none"})
		return out
	}
	if al.Manifest.SemanticHash != bl.Manifest.SemanticHash {
		out = append(out, Change{Kind: "lock", Path: "manifest.semanticHash",
			Detail: al.Manifest.SemanticHash + " -> " + bl.Manifest.SemanticHash, Risk: "none"})
	}
	am := map[string]lockfile.InputLock{}
	bm := map[string]lockfile.InputLock{}
	for _, in := range al.Inputs {
		am[in.Path] = in
	}
	for _, in := range bl.Inputs {
		bm[in.Path] = in
	}
	for _, p := range sortedKeys(am) {
		if _, ok := bm[p]; !ok {
			out = append(out, Change{Kind: "resource", Path: p, Detail: "removed from lock", Risk: "none"})
			continue
		}
		if am[p].SHA256 != bm[p].SHA256 {
			out = append(out, Change{Kind: "resource", Path: p, Detail: "content hash changed", Risk: "none"})
		}
	}
	for _, p := range sortedKeys(bm) {
		if _, ok := am[p]; !ok {
			out = append(out, Change{Kind: "resource", Path: p, Detail: "added to lock", Risk: "none"})
		}
	}
	return out
}

func compareVolumes(a, b map[string]ir.Volume) []Change {
	var out []Change
	for _, id := range unionSorted(keysOf(a), keysOf(b)) {
		av, aok := a[id]
		bv, bok := b[id]
		switch {
		case aok && !bok:
			risk := "destructive"
			if av.Retention == "retain" {
				risk = "irreversible"
			}
			out = append(out, Change{Kind: "volume", Path: id, Detail: "removed", Risk: risk})
		case !aok && bok:
			out = append(out, Change{Kind: "volume", Path: id, Detail: "added", Risk: "none"})
		case fmt.Sprintf("%v", av) != fmt.Sprintf("%v", bv):
			risk := "none"
			if av.Retention != bv.Retention {
				risk = "destructive"
			}
			out = append(out, Change{Kind: "volume", Path: id, Detail: "changed", Risk: risk})
		}
	}
	return out
}

func compareMaps[T any](kind string, aKeys, bKeys []string, a, b map[string]T) []Change {
	var out []Change
	for _, id := range unionSorted(aKeys, bKeys) {
		av, aok := a[id]
		bv, bok := b[id]
		switch {
		case aok && !bok:
			out = append(out, Change{Kind: kind, Path: id, Detail: "removed", Risk: "none"})
		case !aok && bok:
			out = append(out, Change{Kind: kind, Path: id, Detail: "added", Risk: "none"})
		case fmt.Sprintf("%v", av) != fmt.Sprintf("%v", bv):
			out = append(out, Change{Kind: kind, Path: id, Detail: "changed", Risk: "none"})
		}
	}
	return out
}

func keysOf[T any](m map[string]T) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

func sortedKeys[T any](m map[string]T) []string { return keysOf(m) }

func unionSorted(a, b []string) []string {
	seen := map[string]bool{}
	var out []string
	for _, x := range a {
		if !seen[x] {
			seen[x] = true
			out = append(out, x)
		}
	}
	for _, x := range b {
		if !seen[x] {
			seen[x] = true
			out = append(out, x)
		}
	}
	sort.Strings(out)
	return out
}

func sortChanges(c []Change) {
	sort.SliceStable(c, func(i, j int) bool {
		if c[i].Kind != c[j].Kind {
			return c[i].Kind < c[j].Kind
		}
		return c[i].Path < c[j].Path
	})
}
