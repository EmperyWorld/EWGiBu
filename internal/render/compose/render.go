package compose

import (
	"fmt"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"iacpack/internal/domain/ir"
	"iacpack/internal/lockfile"
	"iacpack/internal/params"
	"iacpack/internal/runtime/secrets"
)

// safeContainerDir is an absolute container path that can be placed unquoted
// in `cd <dir> && exec "$@"`.
var safeContainerDir = regexp.MustCompile(`^/[A-Za-z0-9._/-]+$`)

// RenderOptions controls preview rendering.
type RenderOptions struct {
	// DeployValues are resolved deploy-phase parameter values (id -> display string).
	DeployValues map[string]string
	// Lock provides digest-pinned image refs.
	Lock lockfile.Document
	// HostNVIDIADevices are /dev/nvidia* nodes for gpu: devices-only. Empty omits the devices list.
	HostNVIDIADevices []string
	// SELinuxEnforcing makes devices-only turn off confinement for that service.
	// container_t cannot open /dev/nvidia* (xserver_misc_device_t); CUDA then
	// reports that no device exists.
	SELinuxEnforcing bool
	// ImageWorkDirs are container working directories taken from image
	// Config.WorkingDir, keyed by service or task id. A task workdir already
	// set in the manifest wins.
	ImageWorkDirs map[string]string
}

// CanPreview reports whether all deploy parameters needed for rendering have values.
func CanPreview(d ir.Deployment, deployResolved map[string]params.Resolved) (ok bool, missing []string) {
	needed := map[string]bool{}
	collectParamRefs(d, needed)
	for id := range needed {
		if _, ok := deployResolved[id]; !ok {
			missing = append(missing, id)
		}
	}
	sortStrings(missing)
	return len(missing) == 0, missing
}

func collectParamRefs(d ir.Deployment, out map[string]bool) {
	for _, svc := range d.Services {
		for _, vs := range svc.Environment {
			if vs.Parameter != "" {
				out[vs.Parameter] = true
			}
		}
		for _, p := range svc.Ports {
			if p.Host != nil && p.Host.Parameter != "" {
				out[p.Host.Parameter] = true
			}
		}
	}
	for _, img := range d.Images {
		if img.RefParameter != "" && img.Ref == "" {
			out[img.RefParameter] = true
		}
	}
	collectArgvParams(d, out)
	for _, svc := range d.Services {
		for _, m := range svc.Mounts {
			if m.HostParameter != "" {
				out[m.HostParameter] = true
			}
		}
	}
	for _, task := range d.Tasks {
		for _, m := range task.Mounts {
			if m.HostParameter != "" {
				out[m.HostParameter] = true
			}
		}
	}
}

func collectArgvParams(d ir.Deployment, out map[string]bool) {
	note := func(cmd []ir.ArgvElement) {
		for _, el := range cmd {
			for _, p := range el.Parts {
				if p.Parameter != "" {
					out[p.Parameter] = true
				}
			}
		}
	}
	for _, svc := range d.Services {
		note(svc.Command)
		note(svc.Entrypoint)
	}
	for _, task := range d.Tasks {
		note(task.Command)
	}
}

func resolvedPullRef(img ir.Image, locked lockfile.ImageLock, d ir.Deployment, opts RenderOptions) (string, error) {
	if locked.Ref != "" {
		return locked.Ref, nil
	}
	if img.Ref != "" {
		return img.Ref, nil
	}
	if img.RefParameter != "" {
		if v := strings.TrimSpace(opts.DeployValues[img.RefParameter]); v != "" {
			return v, nil
		}
		if p, ok := d.Parameters[img.RefParameter]; ok && p.Default != nil {
			s := strings.TrimSpace(fmt.Sprint(p.Default))
			if s != "" {
				return s, nil
			}
		}
		return "", fmt.Errorf("image parameter %q has no value", img.RefParameter)
	}
	return "", fmt.Errorf("image has no ref")
}

// Render builds a Compose File from IR + resolved deploy values + lock images.
func Render(d ir.Deployment, opts RenderOptions) (File, error) {
	f := File{
		Name:     "iacp-" + d.Project.Name,
		Services: map[string]Service{},
		Networks: map[string]Network{},
		Volumes:  map[string]Volume{},
		Secrets:  map[string]Secret{},
	}
	for id, n := range d.Networks {
		if n.External != nil {
			f.Networks[id] = Network{External: true, Name: n.External.Name}
		} else {
			f.Networks[id] = Network{}
		}
	}
	for id, v := range d.Volumes {
		if v.External != nil {
			f.Volumes[id] = Volume{External: true, Name: v.External.Name}
		} else {
			f.Volumes[id] = Volume{}
		}
	}
	for id := range d.Secrets {
		f.Secrets[id] = Secret{File: "../secrets/" + id}
	}

	for id, svc := range d.Services {
		out, err := renderService(id, svc, d, opts)
		if err != nil {
			return File{}, err
		}
		f.Services[id] = out
	}
	// One-shot lifecycle tasks become profiled services so `compose up` skips
	// them while `compose run --rm <task>` (HostApply.RunTask) can execute them.
	for id, task := range d.Tasks {
		if _, exists := f.Services[id]; exists {
			return File{}, fmt.Errorf("task %q collides with service id", id)
		}
		out, err := renderTask(id, task, d, opts)
		if err != nil {
			return File{}, err
		}
		f.Services[id] = out
	}
	return f, nil
}

func renderTask(id string, task ir.Task, d ir.Deployment, opts RenderOptions) (Service, error) {
	cmd, err := resolveArgv(task.Command, opts.DeployValues)
	if err != nil {
		return Service{}, fmt.Errorf("task %q: %w", id, err)
	}
	ep, err := resolveArgv(task.Entrypoint, opts.DeployValues)
	if err != nil {
		return Service{}, fmt.Errorf("task %q: %w", id, err)
	}
	out := Service{
		Command:         cmd,
		Entrypoint:      ep,
		EntrypointClear: task.ResetEntrypoint,
		User:            task.User,
		Networks:        append([]string(nil), task.Networks...),
		WorkingDir:      firstWorkDir(task.Workdir, imageWorkDir(opts, id)),
		Restart:         "no",
		Profiles:        []string{"iacpack-task"},
		DependsOn:       map[string]DependsOn{},
		Environment:     map[string]string{},
	}
	for k, vs := range task.Environment {
		val, err := resolveVS(vs, opts.DeployValues)
		if err != nil {
			return Service{}, fmt.Errorf("task %q env %q: %w", id, k, err)
		}
		if val == "" {
			continue
		}
		out.Environment[k] = val
	}
	img, ok := d.Images[task.Image]
	if !ok {
		return Service{}, fmt.Errorf("task %q references unknown image %q", id, task.Image)
	}
	limg := opts.Lock.Images[task.Image]
	if img.Build != nil {
		out.Build = &Build{
			Context:    "./builds/" + task.Image,
			Dockerfile: img.Build.Dockerfile,
			Args:       map[string]string{},
		}
	} else {
		ref, err := resolvedPullRef(img, limg, d, opts)
		if err != nil {
			return Service{}, fmt.Errorf("task %q: %w", id, err)
		}
		out.Image = ref
	}
	vols, err := renderMounts(task.Mounts, opts)
	if err != nil {
		return Service{}, fmt.Errorf("task %q: %w", id, err)
	}
	out.Volumes = append(out.Volumes, vols...)
	for _, s := range task.Secrets {
		target := s.Target
		if i := strings.LastIndex(target, "/"); i >= 0 && i+1 < len(target) {
			target = target[i+1:]
		}
		out.Secrets = append(out.Secrets, SecretRef{Source: s.Ref, Target: target})
	}
	pinWorkDir(&out)
	return out, nil
}

func renderService(id string, svc ir.Service, d ir.Deployment, opts RenderOptions) (Service, error) {
	cmd, err := resolveArgv(svc.Command, opts.DeployValues)
	if err != nil {
		return Service{}, fmt.Errorf("service %q: %w", id, err)
	}
	ep, err := resolveArgv(svc.Entrypoint, opts.DeployValues)
	if err != nil {
		return Service{}, fmt.Errorf("service %q: %w", id, err)
	}
	out := Service{
		Command:     cmd,
		Entrypoint:  ep,
		User:        svc.User,
		WorkingDir:  imageWorkDir(opts, id),
		Restart:     svc.Restart,
		Networks:    append([]string(nil), svc.Networks...),
		Environment: map[string]string{},
		DependsOn:   map[string]DependsOn{},
	}
	img, ok := d.Images[svc.Image]
	if !ok {
		return Service{}, fmt.Errorf("service %q references unknown image %q", id, svc.Image)
	}
	limg := opts.Lock.Images[svc.Image]
	if img.Build != nil {
		out.Build = &Build{
			Context:    "./builds/" + svc.Image,
			Dockerfile: img.Build.Dockerfile,
			Args:       map[string]string{},
		}
		// Build args from authoring use build parameters already locked; not re-resolved here.
	} else {
		ref, err := resolvedPullRef(img, limg, d, opts)
		if err != nil {
			return Service{}, fmt.Errorf("service %q: %w", id, err)
		}
		out.Image = ref
	}

	envSrc, err := mergedEnvSources(d, id, nil)
	if err != nil {
		return Service{}, fmt.Errorf("service %q: %w", id, err)
	}
	for k, vs := range envSrc {
		val, err := resolveVS(vs, opts.DeployValues)
		if err != nil {
			return Service{}, fmt.Errorf("service %q env %q: %w", id, k, err)
		}
		if vs.Parameter != "" {
			keep, err := acceptEnvParam(d, vs.Parameter, val)
			if err != nil {
				return Service{}, fmt.Errorf("service %q env %q: %w", id, k, err)
			}
			if !keep {
				continue
			}
		} else if val == "" {
			// Literal empty string is rare; omit to avoid typed container parsers seeing "".
			continue
		}
		out.Environment[k] = val
	}
	for _, p := range svc.Ports {
		cp := Port{Target: p.Container, HostIP: p.Bind, Protocol: p.Protocol}
		if p.Host != nil {
			if p.Host.Literal != nil {
				cp.Published = strconv.Itoa(*p.Host.Literal)
			} else if p.Host.Parameter != "" {
				v, ok := opts.DeployValues[p.Host.Parameter]
				if !ok {
					return Service{}, fmt.Errorf("missing deploy parameter %q for port", p.Host.Parameter)
				}
				cp.Published = v
			}
		}
		out.Ports = append(out.Ports, cp)
	}
	vols, err := renderMounts(svc.Mounts, opts)
	if err != nil {
		return Service{}, fmt.Errorf("service %q: %w", id, err)
	}
	out.Volumes = append(out.Volumes, vols...)
	for _, s := range svc.Secrets {
		target := s.Target
		if i := strings.LastIndex(target, "/"); i >= 0 && i+1 < len(target) {
			target = target[i+1:]
		}
		out.Secrets = append(out.Secrets, SecretRef{Source: s.Ref, Target: target})
	}
	for _, dep := range svc.DependsOn {
		cond := "service_started"
		switch dep.Condition {
		case "healthy":
			cond = "service_healthy"
		case "completedSuccessfully":
			cond = "service_completed_successfully"
		case "started", "":
			cond = "service_started"
		}
		out.DependsOn[dep.Target] = DependsOn{Condition: cond}
	}
	if svc.Healthcheck != nil {
		h := svc.Healthcheck
		test := HealthcheckTestFromManifest(h.Command)
		out.Healthcheck = &Healthcheck{
			Test: test, Interval: h.Interval, Timeout: h.Timeout,
			StartPeriod: h.StartPeriod, Retries: h.Retries,
		}
	}
	if len(svc.Profiles) > 0 {
		out.Profiles = append([]string(nil), svc.Profiles...)
	}
	if svc.Resources != nil {
		mem := ""
		if svc.Resources.Memory > 0 {
			mem = strconv.FormatInt(svc.Resources.Memory, 10)
		}
		out.Deploy = &Deploy{
			CPUs: svc.Resources.CPUs, Memory: mem, GPU: svc.Resources.GPU, GPUAll: svc.Resources.GPUAll,
			GPUDevicesOnly: svc.Resources.GPUDevicesOnly,
		}
		if svc.Resources.GPUDevicesOnly {
			out.Devices = append([]string(nil), opts.HostNVIDIADevices...)
			if out.Environment == nil {
				out.Environment = map[string]string{}
			}
			lib := svc.Resources.LDLibraryPath
			if cur := out.Environment["LD_LIBRARY_PATH"]; cur != "" && !strings.Contains(cur, lib) {
				out.Environment["LD_LIBRARY_PATH"] = lib + ":" + cur
			} else if cur == "" {
				out.Environment["LD_LIBRARY_PATH"] = lib
			}
			// Podman treats NVIDIA_VISIBLE_DEVICES=all as a CDI request for
			// nvidia.com/gpu=all even when the compose file has no device
			// reservation. void hides every GPU from CUDA, including nodes
			// passed with --device. List the mounted card indexes instead
			// (nvidia0 -> 0) so the driver can see them.
			out.Environment["NVIDIA_VISIBLE_DEVICES"] = VisibleNVIDIADevices(out.Devices)
			delete(out.Environment, "NVIDIA_DRIVER_CAPABILITIES")
			if opts.SELinuxEnforcing {
				out.SecurityOpt = append(out.SecurityOpt, "label=disable")
			}
		}
	}
	pinWorkDir(&out)
	return out, nil
}

func VisibleNVIDIADevices(devices []string) string {
	var idx []string
	for _, dev := range devices {
		base := dev
		if i := strings.LastIndex(dev, "/"); i >= 0 {
			base = dev[i+1:]
		}
		n := strings.TrimPrefix(base, "nvidia")
		if n == base || n == "" {
			continue
		}
		allNum := true
		for _, r := range n {
			if r < '0' || r > '9' {
				allNum = false
				break
			}
		}
		if allNum {
			idx = append(idx, n)
		}
	}
	sort.Slice(idx, func(i, j int) bool {
		if len(idx[i]) != len(idx[j]) {
			return len(idx[i]) < len(idx[j])
		}
		return idx[i] < idx[j]
	})
	return strings.Join(idx, ",")
}

func imageWorkDir(opts RenderOptions, id string) string {
	wd := strings.TrimSpace(opts.ImageWorkDirs[id])
	if !safeContainerDir.MatchString(wd) || strings.Contains(wd, "..") {
		return ""
	}
	return wd
}

// pinWorkDir cds into the image working directory before an overridden
// entrypoint. Podman can reset the process cwd to / when --entrypoint is set,
// and a relative module such as uvicorn's server:app is then not importable.
// working_dir is still written; the shell cd does not depend on the engine
// keeping it.
func pinWorkDir(out *Service) {
	if out.WorkingDir == "" || len(out.Entrypoint) == 0 {
		return
	}
	out.Command = append(append([]string{}, out.Entrypoint...), out.Command...)
	out.Entrypoint = []string{"/bin/sh", "-c", "cd " + out.WorkingDir + ` && exec "$@"`, "sh"}
}

func firstWorkDir(explicit, fromImage string) string {
	if strings.TrimSpace(explicit) != "" {
		return explicit
	}
	return fromImage
}

func resolveArgv(cmd []ir.ArgvElement, deploy map[string]string) ([]string, error) {
	if len(cmd) == 0 {
		return nil, nil
	}
	out := make([]string, len(cmd))
	for i, el := range cmd {
		if el.Secret != "" {
			out[i] = secrets.ComposePlaceholder(el.Secret)
			continue
		}
		if len(el.Parts) == 0 {
			out[i] = el.Literal
			continue
		}
		var b strings.Builder
		for _, p := range el.Parts {
			switch {
			case p.Secret != "":
				b.WriteString(secrets.ComposePlaceholder(p.Secret))
			case p.Parameter != "":
				v, ok := deploy[p.Parameter]
				if !ok {
					return nil, fmt.Errorf("unresolved parameter %q", p.Parameter)
				}
				b.WriteString(v)
			default:
				b.WriteString(p.Literal)
			}
		}
		out[i] = b.String()
	}
	return out, nil
}

func renderMounts(mounts []ir.Mount, opts RenderOptions) ([]VolumeMount, error) {
	var out []VolumeMount
	for _, m := range mounts {
		switch {
		case m.Volume != "":
			out = append(out, VolumeMount{
				Type: "volume", Source: m.Volume, Target: m.Target, ReadOnly: m.ReadOnly,
			})
		case m.Resource != "":
			src := "./payload/" + m.Resource
			if m.Source != "" {
				src = src + "/" + strings.TrimPrefix(m.Source, "./")
			}
			out = append(out, VolumeMount{
				Type: "bind", Source: src, Target: m.Target, ReadOnly: m.ReadOnly, Relabel: "z",
			})
		case m.Build != "":
			out = append(out, VolumeMount{
				Type: "bind", Source: "./payload/build-" + m.Build, Target: m.Target, ReadOnly: m.ReadOnly, Relabel: "z",
			})
		case m.HostLiteral != "" || m.HostParameter != "":
			src := m.HostLiteral
			if m.HostParameter != "" {
				v, ok := opts.DeployValues[m.HostParameter]
				if !ok {
					return nil, fmt.Errorf("unresolved host path parameter %q", m.HostParameter)
				}
				src = v
			}
			out = append(out, VolumeMount{
				Type: "bind", Source: src, Target: m.Target, ReadOnly: m.ReadOnly, Relabel: "z",
			})
		}
	}
	return out, nil
}

func resolveVS(vs ir.ValueSource, deploy map[string]string) (string, error) {
	if len(vs.Parts) > 0 {
		var b strings.Builder
		for _, p := range vs.Parts {
			s, err := resolveVS(ir.ValueSource{Literal: p.Literal, Parameter: p.Parameter, Secret: p.Secret}, deploy)
			if err != nil {
				return "", err
			}
			b.WriteString(s)
		}
		return b.String(), nil
	}
	if vs.Secret != "" {
		return secrets.ComposePlaceholder(vs.Secret), nil
	}
	if vs.Parameter != "" {
		v, ok := deploy[vs.Parameter]
		if !ok {
			return "", fmt.Errorf("unresolved parameter %q", vs.Parameter)
		}
		return v, nil
	}
	if vs.Literal == nil {
		return "", fmt.Errorf("empty value source")
	}
	switch t := vs.Literal.(type) {
	case string:
		return t, nil
	case bool:
		return strconv.FormatBool(t), nil
	case float64:
		return strconv.FormatFloat(t, 'f', -1, 64), nil
	case int:
		return strconv.Itoa(t), nil
	case int64:
		return strconv.FormatInt(t, 10), nil
	default:
		return fmt.Sprint(t), nil
	}
}

// mergedEnvSources copies environmentFrom (parent first) then overlays this service.
func mergedEnvSources(d ir.Deployment, id string, stack []string) (map[string]ir.ValueSource, error) {
	for _, s := range stack {
		if s == id {
			return nil, fmt.Errorf("environmentFrom cycle through %q", id)
		}
	}
	svc, ok := d.Services[id]
	if !ok {
		return nil, fmt.Errorf("environmentFrom references unknown service %q", id)
	}
	out := map[string]ir.ValueSource{}
	if svc.EnvironmentFrom != "" {
		parent, err := mergedEnvSources(d, svc.EnvironmentFrom, append(stack, id))
		if err != nil {
			return nil, err
		}
		for k, v := range parent {
			out[k] = v
		}
	}
	for k, v := range svc.Environment {
		out[k] = v
	}
	return out, nil
}

// acceptEnvParam decides whether an empty resolved parameter may appear in Compose env.
// Empty string/enum values are omitted. Empty integer/boolean/number values are errors.
func acceptEnvParam(d ir.Deployment, paramID, val string) (keep bool, err error) {
	if val != "" {
		return true, nil
	}
	p, ok := d.Parameters[paramID]
	if !ok {
		return false, nil
	}
	switch strings.ToLower(strings.TrimSpace(p.Type)) {
	case "integer", "number", "boolean":
		return false, fmt.Errorf("parameter %q type %s cannot be an empty string", paramID, p.Type)
	default:
		// string / enum / unknown: omit KEY rather than emit KEY: ""
		return false, nil
	}
}

func sortStrings(ss []string) {
	for i := 0; i < len(ss); i++ {
		for j := i + 1; j < len(ss); j++ {
			if ss[j] < ss[i] {
				ss[i], ss[j] = ss[j], ss[i]
			}
		}
	}
}
