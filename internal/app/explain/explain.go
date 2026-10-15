// Package explain prints the runtime facts for one service in a verified bundle.
package explain

import (
	"fmt"
	"sort"
	"strconv"
	"strings"

	"iacpack/internal/app/apply"
	"iacpack/internal/app/verify"
	"iacpack/internal/bundle"
	"iacpack/internal/compile"
	"iacpack/internal/diagnostic"
	"iacpack/internal/domain/ir"
)

// Result is the explain outcome. No secret values.
type Result struct {
	OK          bool
	Diagnostics []diagnostic.Diagnostic
	Model       map[string]any
	Text        string
}

// Run explains one service from a bundle: image, gpu, ldLibraryPath, and devices.
func Run(bundlePath, service string) Result {
	var res Result
	if bundlePath == "" || service == "" {
		res.Diagnostics = []diagnostic.Diagnostic{diagnostic.Error(diagnostic.CodeCLIInvalidUsage, "explain requires a bundle and a service id")}
		return finish(res)
	}
	raw, _, diags := verify.ReadVerifiedFile(bundlePath, "deployment.json")
	res.Diagnostics = append(res.Diagnostics, diags...)
	if diagnostic.HasError(res.Diagnostics) {
		return finish(res)
	}
	dep, err := compile.UnmarshalDeployment(raw)
	if err != nil {
		res.Diagnostics = append(res.Diagnostics, diagnostic.Error(bundle.CodeBundleMeta, err.Error()))
		return finish(res)
	}
	svc, ok := dep.Services[service]
	if !ok {
		res.Diagnostics = append(res.Diagnostics, diagnostic.Error(diagnostic.CodeManifestInvalidReference,
			fmt.Sprintf("unknown service %q", service)))
		return finish(res)
	}
	img := dep.Images[svc.Image]
	model := map[string]any{
		"service":       service,
		"image":         svc.Image,
		"imageRef":      imageRef(img),
		"user":          svc.User,
		"gpu":           gpuMode(svc),
		"ldLibraryPath": ldPath(svc),
		"devices":       devicesFor(svc),
		"envKeys":       envKeys(dep, service),
	}
	if svc.EnvironmentFrom != "" {
		model["environmentFrom"] = svc.EnvironmentFrom
	}
	res.Model = model
	res.Text = formatText(model)
	res.OK = true
	return finish(res)
}

func imageRef(img ir.Image) string {
	if img.Ref != "" {
		return img.Ref
	}
	if img.RefParameter != "" {
		return "parameter:" + img.RefParameter
	}
	if img.Build != nil {
		return "build"
	}
	return ""
}

func gpuMode(svc ir.Service) string {
	if svc.Resources == nil {
		return "none"
	}
	switch {
	case svc.Resources.GPUDevicesOnly:
		return "devices-only"
	case svc.Resources.GPUAll:
		return "all"
	case svc.Resources.GPU > 0:
		return strconv.Itoa(svc.Resources.GPU)
	default:
		return "none"
	}
}

func ldPath(svc ir.Service) string {
	if svc.Resources == nil {
		return ""
	}
	return svc.Resources.LDLibraryPath
}

func devicesFor(svc ir.Service) []string {
	if svc.Resources == nil {
		return []string{}
	}
	switch {
	case svc.Resources.GPUDevicesOnly:
		nodes := apply.HostNVIDIADevices()
		if nodes == nil {
			return []string{}
		}
		return nodes
	case svc.Resources.GPUAll:
		return []string{"nvidia.com/gpu=all"}
	default:
		return []string{}
	}
}

func envKeys(dep ir.Deployment, id string) []string {
	seen := map[string]struct{}{}
	var walk func(string, []string)
	walk = func(cur string, stack []string) {
		for _, s := range stack {
			if s == cur {
				return
			}
		}
		svc, ok := dep.Services[cur]
		if !ok {
			return
		}
		if svc.EnvironmentFrom != "" {
			walk(svc.EnvironmentFrom, append(stack, cur))
		}
		for k := range svc.Environment {
			seen[k] = struct{}{}
		}
	}
	walk(id, nil)
	keys := make([]string, 0, len(seen))
	for k := range seen {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

func formatText(model map[string]any) string {
	var b strings.Builder
	fmt.Fprintf(&b, "service: %s\n", model["service"])
	fmt.Fprintf(&b, "image: %s\n", model["imageRef"])
	fmt.Fprintf(&b, "gpu: %s\n", model["gpu"])
	fmt.Fprintf(&b, "ldLibraryPath: %s\n", model["ldLibraryPath"])
	devs, _ := model["devices"].([]string)
	fmt.Fprintf(&b, "devices: %s\n", strings.Join(devs, ", "))
	if from, ok := model["environmentFrom"].(string); ok && from != "" {
		fmt.Fprintf(&b, "environmentFrom: %s\n", from)
	}
	if user, _ := model["user"].(string); user != "" {
		fmt.Fprintf(&b, "user: %s\n", user)
	}
	keys, _ := model["envKeys"].([]string)
	fmt.Fprintf(&b, "envKeys: %s\n", strings.Join(keys, ", "))
	return b.String()
}

func finish(res Result) Result {
	diagnostic.SortDeterministic(res.Diagnostics)
	res.OK = res.OK && !diagnostic.HasError(res.Diagnostics)
	return res
}
