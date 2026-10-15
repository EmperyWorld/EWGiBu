// Package destroy removes managed resources with matching ownership labels.
package destroy

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"iacpack/internal/diagnostic"
	"iacpack/internal/runtime/identity"
	"iacpack/internal/runtime/state"
)

// Provider is the destroy surface.
type Provider interface {
	Down(ctx context.Context, project, composeFile string) error
	DeleteManagedVolumes(ctx context.Context, project string, names []string) error
	ListOwned(ctx context.Context, project string) ([]string, error)
}

// Options configures destroy.
type Options struct {
	Project         string
	Instance        string
	StateHome       string
	DeleteVolumes   bool
	Yes             bool
	DryRun          bool
	Provider        Provider
	RetentionDelete []string // volume IDs allowed to delete when DeleteVolumes
	RetentionRetain []string // never deleted
}

// Result is the destroy outcome.
type Result struct {
	OK          bool
	Diagnostics []diagnostic.Diagnostic
	Model       map[string]any
	Text        string
}

// Recording is a fake destroy provider.
type Recording struct {
	Calls []string
}

func (r *Recording) Down(ctx context.Context, project, composeFile string) error {
	_ = ctx
	r.Calls = append(r.Calls, "down:"+project)
	if composeFile != "" {
		r.Calls = append(r.Calls, "compose:"+composeFile)
	}
	return nil
}
func (r *Recording) DeleteManagedVolumes(ctx context.Context, project string, names []string) error {
	_ = ctx
	for _, n := range names {
		r.Calls = append(r.Calls, "rmvol:"+project+":"+n)
	}
	return nil
}
func (r *Recording) ListOwned(ctx context.Context, project string) ([]string, error) {
	_ = ctx
	return []string{"compose:" + project}, nil
}

// Run destroys managed resources for a project/instance.
func Run(ctx context.Context, opts Options) Result {
	var res Result
	if opts.Project == "" {
		res.Diagnostics = []diagnostic.Diagnostic{diagnostic.Error(diagnostic.CodeCLIInvalidUsage, "destroy requires a project")}
		return finish(res)
	}
	instance := opts.Instance
	if instance == "" {
		instance = identity.DefaultInstance
	}
	if err := identity.ValidateInstance(instance); err != nil {
		res.Diagnostics = []diagnostic.Diagnostic{diagnostic.Error(diagnostic.CodeCLIInvalidUsage, err.Error())}
		return finish(res)
	}
	if opts.Provider == nil {
		res.Diagnostics = []diagnostic.Diagnostic{diagnostic.Error(diagnostic.CodeInternal, "destroy provider required")}
		return finish(res)
	}
	if opts.DeleteVolumes && !opts.Yes && !opts.DryRun {
		res.Diagnostics = []diagnostic.Diagnostic{diagnostic.Error(diagnostic.CodeCLIInvalidUsage,
			"--delete-volumes requires --yes in non-interactive mode")}
		return finish(res)
	}
	home := opts.StateHome
	if home == "" {
		home = state.ResolveHome(os.Getenv("XDG_STATE_HOME"), os.Getenv("HOME"))
	}
	paths := state.ForInstance(home, opts.Project, instance)
	slug := identity.ProviderSlug(opts.Project, instance)

	lock, err := state.AcquireLock(paths)
	if err != nil {
		res.Diagnostics = append(res.Diagnostics, diagnostic.Error(diagnostic.CodeInputIOError, err.Error()))
		return finish(res)
	}
	defer lock.Release()

	planned := []string{"down " + slug}
	var volDelete []string
	if opts.DeleteVolumes {
		for _, v := range opts.RetentionDelete {
			volDelete = append(volDelete, v)
			planned = append(planned, "delete-volume "+v)
		}
		for _, v := range opts.RetentionRetain {
			res.Diagnostics = append(res.Diagnostics, diagnostic.Warning("DESTROY.RETAIN",
				fmt.Sprintf("volume %q has retention=retain and will not be deleted", v)))
		}
	}
	if opts.DryRun {
		res.OK = true
		res.Model = map[string]any{"project": opts.Project, "instance": instance, "dryRun": true, "planned": planned}
		res.Text = "destroy dry-run:\n  " + strings.Join(planned, "\n  ") + "\n"
		return finish(res)
	}
	composeFile := filepath.Join(paths.Rendered, "compose.yaml")
	if _, err := os.Stat(composeFile); err != nil {
		composeFile = ""
	}
	if err := opts.Provider.Down(ctx, slug, composeFile); err != nil {
		res.Diagnostics = append(res.Diagnostics, diagnostic.Error("DESTROY.PROVIDER", err.Error()))
		return finish(res)
	}
	if opts.DeleteVolumes && len(volDelete) > 0 {
		if err := opts.Provider.DeleteManagedVolumes(ctx, slug, volDelete); err != nil {
			res.Diagnostics = append(res.Diagnostics, diagnostic.Error("DESTROY.PROVIDER", err.Error()))
			return finish(res)
		}
	}
	// Remove state tree except we keep directory cleanup of state.json
	_ = os.Remove(paths.State)
	_ = os.RemoveAll(paths.Secrets)
	_ = os.RemoveAll(paths.Rendered)
	_ = os.Remove(paths.Journal)

	res.OK = true
	res.Model = map[string]any{
		"project": opts.Project, "instance": instance, "providerSlug": slug,
		"deletedVolumes": volDelete, "dryRun": false,
	}
	res.Text = fmt.Sprintf("destroyed %s instance=%s\n", opts.Project, instance)
	return finish(res)
}

func finish(res Result) Result {
	diagnostic.SortDeterministic(res.Diagnostics)
	res.OK = res.OK && !diagnostic.HasError(res.Diagnostics)
	return res
}
