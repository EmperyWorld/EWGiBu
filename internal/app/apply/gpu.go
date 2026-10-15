package apply

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"iacpack/internal/diagnostic"
	"iacpack/internal/domain/ir"
	"iacpack/internal/render/compose"
	"iacpack/internal/runtime/provider"
)

const codeApplyGPUUnavailable = "APPLY.GPU_UNAVAILABLE"

func podmanCountAllCDIMissing(p provider.ApplyOps, dep ir.Deployment) bool {
	if !deploymentHasGPUAll(dep) {
		return false
	}
	type probe interface {
		PodmanCountAllCDIMissing() bool
	}
	g, ok := p.(probe)
	if !ok {
		return false
	}
	return g.PodmanCountAllCDIMissing()
}

func sortedGPUServices(dep ir.Deployment) []string {
	var ids []string
	for id, svc := range dep.Services {
		if svc.Resources != nil && svc.Resources.GPUAll {
			ids = append(ids, id)
		}
	}
	sort.Strings(ids)
	return ids
}

func devicesOnlyWithoutNodes(dep ir.Deployment, nodes []string) []string {
	if len(nodes) > 0 {
		return nil
	}
	var ids []string
	for id, svc := range dep.Services {
		if svc.Resources != nil && svc.Resources.GPUDevicesOnly {
			ids = append(ids, id)
		}
	}
	sort.Strings(ids)
	return ids
}

// HostNVIDIADevices lists host /dev/nvidia* nodes that devices-only bind-mounts.
func HostNVIDIADevices() []string { return hostNVIDIADevices() }

func hostNVIDIADevices() []string {
	matches, err := filepath.Glob("/dev/nvidia*")
	if err != nil {
		return nil
	}
	var out []string
	for _, p := range matches {
		base := filepath.Base(p)
		if base == "nvidia-caps" {
			kids, _ := filepath.Glob(filepath.Join(p, "*"))
			for _, k := range kids {
				if nvidiaCapsNode(k) {
					out = append(out, k)
				}
			}
			continue
		}
		if nvidiaControlNode(base) {
			out = append(out, p)
		}
	}
	sort.Strings(out)
	return out
}

func nvidiaControlNode(base string) bool {
	switch base {
	case "nvidiactl", "nvidia-uvm", "nvidia-uvm-tools", "nvidia-modeset":
		return true
	default:
		return strings.HasPrefix(base, "nvidia") && isDigits(strings.TrimPrefix(base, "nvidia"))
	}
}

// nvidiaCapsNode is a CUDA 12+ capability device. The directory itself is not
// a device node; mounting only nvidia0 leaves cudaGetDeviceCount at zero on
// current drivers.
func nvidiaCapsNode(path string) bool {
	st, err := os.Lstat(path)
	if err != nil || st.IsDir() || st.Mode()&os.ModeSymlink != 0 {
		return false
	}
	return st.Mode()&os.ModeDevice != 0
}

func isDigits(s string) bool {
	if s == "" {
		return false
	}
	for _, r := range s {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}

func serviceHasGPUAll(dep ir.Deployment, id string) bool {
	svc, ok := dep.Services[id]
	return ok && svc.Resources != nil && svc.Resources.GPUAll
}

func deploymentHasGPUAll(dep ir.Deployment) bool {
	for _, svc := range dep.Services {
		if svc.Resources != nil && svc.Resources.GPUAll {
			return true
		}
	}
	return false
}

func cloneServiceResources(in map[string]ir.Service) map[string]ir.Service {
	out := make(map[string]ir.Service, len(in))
	for id, svc := range in {
		if svc.Resources != nil {
			rl := *svc.Resources
			svc.Resources = &rl
		}
		out[id] = svc
	}
	return out
}

// clearGPUAll clears resources.gpu all on the named services.
// An empty only list clears every service. The returned ids are sorted.
func clearGPUAll(dep *ir.Deployment, only ...string) []string {
	want := map[string]struct{}{}
	if len(only) == 0 {
		for id := range dep.Services {
			want[id] = struct{}{}
		}
	} else {
		for _, id := range only {
			want[id] = struct{}{}
		}
	}
	var cleared []string
	for id := range want {
		svc, ok := dep.Services[id]
		if !ok || svc.Resources == nil || !svc.Resources.GPUAll {
			continue
		}
		svc.Resources.GPUAll = false
		dep.Services[id] = svc
		cleared = append(cleared, id)
	}
	sort.Strings(cleared)
	return cleared
}

func gpuStartWarning(service string) diagnostic.Diagnostic {
	return diagnostic.Warning(codeApplyGPUUnavailable, gpuStartMessage(service, provider.NVIDIACountAllCDIDevice))
}

func gpuStartMessage(service, device string) string {
	return fmt.Sprintf("service %q: CDI device %q is not available; starting without device reservation", service, device)
}

func renderComposeBytes(dep ir.Deployment, opts compose.RenderOptions) ([]byte, error) {
	cf, err := compose.Render(dep, opts)
	if err != nil {
		return nil, err
	}
	return compose.MarshalYAML(cf)
}

func serviceStartDiagnostic(target string, err error) diagnostic.Diagnostic {
	var cdi *provider.CDIUnavailableError
	if errors.As(err, &cdi) {
		svc := cdi.Service
		if svc == "" {
			svc = target
		}
		dev := cdi.Device
		if dev == "" {
			dev = provider.NVIDIACountAllCDIDevice
		}
		return diagnostic.Error(codeApplyGPUUnavailable,
			fmt.Sprintf("service %q: CDI device %q is not available; devices without libcuda run on CPU", svc, dev))
	}
	var mro *provider.MountReadOnlyError
	if errors.As(err, &mro) {
		svc := mro.Service
		if svc == "" {
			svc = target
		}
		return diagnostic.Error("APPLY.ACTION",
			fmt.Sprintf("service %q: mount %q is read-only", svc, mro.Target))
	}
	msg := err.Error()
	if target != "" && !strings.Contains(msg, target) {
		msg = fmt.Sprintf("service %q: %s", target, msg)
	}
	return diagnostic.Error("APPLY.ACTION", msg)
}

func assertServiceWritableMounts(ctx context.Context, p provider.ApplyOps, project, service string, dep ir.Deployment) error {
	svc, ok := dep.Services[service]
	if !ok {
		return nil
	}
	var targets []string
	for _, m := range svc.Mounts {
		if m.ReadOnly || m.Target == "" {
			continue
		}
		targets = append(targets, m.Target)
	}
	if len(targets) == 0 {
		return nil
	}
	type checker interface {
		AssertWritableMounts(context.Context, string, string, []string) error
	}
	c, ok := p.(checker)
	if !ok {
		return nil
	}
	return c.AssertWritableMounts(ctx, project, service, targets)
}
