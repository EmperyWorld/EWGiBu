package apply

import (
	"context"
	"strings"

	"iacpack/internal/domain/ir"
	"iacpack/internal/runtime/provider"
)

type workdirInspector interface {
	ImageWorkDir(context.Context, string) (string, error)
}

// imageWorkDirs copies each image Config.WorkingDir onto services and tasks
// that do not already name a workdir. The manifest has no service workdir
// field; tasks keep an explicit workdir.
func imageWorkDirs(ctx context.Context, p provider.ApplyOps, dep ir.Deployment) (map[string]string, error) {
	insp, ok := p.(workdirInspector)
	if !ok {
		return nil, nil
	}
	cache := map[string]string{}
	lookup := func(imageID string) (string, error) {
		img := dep.Images[imageID]
		ref := img.Ref
		if ref == "" || strings.HasPrefix(ref, "build:") {
			return "", nil
		}
		if wd, seen := cache[ref]; seen {
			return wd, nil
		}
		wd, err := insp.ImageWorkDir(ctx, ref)
		if err != nil {
			return "", err
		}
		cache[ref] = wd
		return wd, nil
	}
	out := map[string]string{}
	for id, svc := range dep.Services {
		wd, err := lookup(svc.Image)
		if err != nil {
			return nil, err
		}
		if wd != "" {
			out[id] = wd
		}
	}
	for id, task := range dep.Tasks {
		if strings.TrimSpace(task.Workdir) != "" {
			continue
		}
		wd, err := lookup(task.Image)
		if err != nil {
			return nil, err
		}
		if wd != "" {
			out[id] = wd
		}
	}
	if len(out) == 0 {
		return nil, nil
	}
	return out, nil
}
