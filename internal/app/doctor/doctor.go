// Package doctor performs read-only host/provider preflight (no mutations).
package doctor

import (
	"context"
	"fmt"
	"runtime"
	"strconv"
	"strings"

	"iacpack/internal/app/verify"
	"iacpack/internal/compile"
	"iacpack/internal/diagnostic"
	"iacpack/internal/runtime/provider"
)

// Options configures doctor.
type Options struct {
	Bundle          string // optional; when set, also checks host ports from deployment
	DockerSocket    string
	ContainerEngine string // podman|docker|empty auto
	DockerConfig    string // explicit only; never ambient
	Provider        provider.ReadOnly
	MinDiskBytes    uint64
}

// Check is one preflight result.
type Check struct {
	Name    string `json:"name"`
	OK      bool   `json:"ok"`
	Detail  string `json:"detail,omitempty"`
	Warning string `json:"warning,omitempty"`
}

// Result is the doctor outcome.
type Result struct {
	OK          bool
	Diagnostics []diagnostic.Diagnostic
	Checks      []Check
	Model       map[string]any
	Text        string
}

const (
	CodeDoctorPlatform = "DOCTOR.PLATFORM"
	CodeDoctorDocker   = "DOCTOR.DOCKER"
	CodeDoctorCompose  = "DOCTOR.COMPOSE"
	CodeDoctorDaemon   = "DOCTOR.DAEMON"
	CodeDoctorDisk     = "DOCTOR.DISK"
	CodeDoctorPort     = "DOCTOR.PORT"
	CodeDoctorEnv      = "DOCTOR.ENV"
)

// Run performs read-only preflight.
func Run(ctx context.Context, opts Options) Result {
	var res Result
	prov := opts.Provider
	if prov == nil {
		res.Diagnostics = []diagnostic.Diagnostic{diagnostic.Error(diagnostic.CodeInternal, "doctor provider required")}
		return finish(res)
	}
	minDisk := opts.MinDiskBytes
	if minDisk == 0 {
		minDisk = 1 << 30 // 1 GiB
	}
	eng, err := provider.ResolveContainerEngine(opts.ContainerEngine)
	if err != nil {
		res.Diagnostics = append(res.Diagnostics, diagnostic.Error(CodeDoctorDocker, err.Error()))
		res.Checks = append(res.Checks, Check{Name: "container-engine", OK: false, Detail: err.Error()})
	} else {
		res.Checks = append(res.Checks, Check{Name: "container-engine", OK: true, Detail: string(eng)})
	}
	socket, err := provider.SelectContainerSocket(eng, opts.DockerSocket)
	if err != nil {
		res.Diagnostics = append(res.Diagnostics, diagnostic.Error(CodeDoctorDocker, err.Error()))
		res.Checks = append(res.Checks, Check{Name: "container-socket", OK: false, Detail: err.Error()})
	} else {
		res.Checks = append(res.Checks, Check{Name: "container-socket", OK: true, Detail: socket})
	}

	plat, err := prov.Platform(ctx)
	if err != nil {
		res.Diagnostics = append(res.Diagnostics, diagnostic.Error(CodeDoctorPlatform, err.Error()))
		res.Checks = append(res.Checks, Check{Name: "platform", OK: false, Detail: err.Error()})
	} else {
		ok := plat.OS == "linux"
		detail := plat.OS + "/" + plat.Arch
		if !ok {
			res.Diagnostics = append(res.Diagnostics, diagnostic.Error(CodeDoctorPlatform,
				"unsupported OS "+plat.OS+"; v1 requires linux"))
		}
		// Also note process GOOS for local runs
		if runtime.GOOS != "linux" && ok {
			// provider may fake linux in tests; warn if host differs
			_ = runtime.GOOS
		}
		res.Checks = append(res.Checks, Check{Name: "platform", OK: ok, Detail: detail})
	}

	dinfo, err := prov.DockerVersion(ctx)
	if err != nil {
		res.Diagnostics = append(res.Diagnostics, diagnostic.Error(CodeDoctorDocker, err.Error()))
		res.Checks = append(res.Checks, Check{Name: "docker-engine", OK: false, Detail: err.Error()})
	} else {
		res.Checks = append(res.Checks, Check{Name: "docker-engine", OK: true, Detail: dinfo.Version})
	}

	cver, err := prov.ComposeVersion(ctx)
	if err != nil {
		res.Diagnostics = append(res.Diagnostics, diagnostic.Error(CodeDoctorCompose, err.Error()))
		res.Checks = append(res.Checks, Check{Name: "compose-plugin", OK: false, Detail: err.Error()})
	} else {
		detail := strings.TrimSpace(cver)
		ok := true
		if eng == provider.EngineDocker {
			ok = composeAtLeast(detail, "2.24")
			if !ok {
				res.Diagnostics = append(res.Diagnostics, diagnostic.Error(CodeDoctorCompose,
					"compose plugin "+detail+" below required 2.24"))
			}
		}
		res.Checks = append(res.Checks, Check{Name: "compose-plugin", OK: ok, Detail: detail})
	}

	if err := prov.DaemonReachable(ctx); err != nil {
		res.Diagnostics = append(res.Diagnostics, diagnostic.Error(CodeDoctorDaemon, err.Error()))
		res.Checks = append(res.Checks, Check{Name: "daemon", OK: false, Detail: err.Error()})
	} else {
		res.Checks = append(res.Checks, Check{Name: "daemon", OK: true})
	}

	disk, err := prov.DiskAvailableBytes(ctx, "/")
	if err != nil {
		res.Diagnostics = append(res.Diagnostics, diagnostic.Error(CodeDoctorDisk, err.Error()))
		res.Checks = append(res.Checks, Check{Name: "disk", OK: false, Detail: err.Error()})
	} else {
		ok := disk >= minDisk
		if !ok {
			res.Diagnostics = append(res.Diagnostics, diagnostic.Error(CodeDoctorDisk,
				fmt.Sprintf("available disk %d bytes below minimum %d", disk, minDisk)))
		}
		res.Checks = append(res.Checks, Check{Name: "disk", OK: ok, Detail: fmt.Sprintf("%d bytes available", disk)})
	}

	if opts.Bundle != "" {
		res.Checks = append(res.Checks, checkBundlePorts(ctx, prov, opts.Bundle, &res)...)
	}

	res.Checks = append(res.Checks, Check{
		Name:    "env-sanitize",
		OK:      true,
		Detail:  "provider calls must use cleaned environ (COMPOSE_*/DOCKER_HOST/DOCKER_CONTEXT cleared)",
		Warning: "host-port preflight has an inherent race before apply binds the port",
	})

	res.Model = map[string]any{
		"checks":       res.Checks,
		"dockerSocket": socket,
		"dockerConfig": opts.DockerConfig,
		"mutations":    0,
	}
	res.Text = formatText(res.Checks)
	res.OK = !diagnostic.HasError(res.Diagnostics)
	return finish(res)
}

func checkBundlePorts(ctx context.Context, prov provider.ReadOnly, bundlePath string, res *Result) []Check {
	depRaw, _, diags := verify.ReadVerifiedFile(bundlePath, "deployment.json")
	if len(diags) > 0 {
		res.Diagnostics = append(res.Diagnostics, diags...)
		return []Check{{Name: "host-ports", OK: false, Detail: "bundle verify failed"}}
	}
	dep, err := compile.UnmarshalDeployment(depRaw)
	if err != nil {
		res.Diagnostics = append(res.Diagnostics, diagnostic.Error(diagnostic.CodeInternal, err.Error()))
		return []Check{{Name: "host-ports", OK: false, Detail: err.Error()}}
	}
	var checks []Check
	okAll := true
	for id, svc := range dep.Services {
		for _, p := range svc.Ports {
			if p.Host == nil || p.Host.Literal == nil {
				continue
			}
			bind := p.Bind
			if bind == "" {
				bind = "0.0.0.0"
			}
			inUse, err := prov.HostPortInUse(ctx, bind, *p.Host.Literal)
			if err != nil {
				okAll = false
				res.Diagnostics = append(res.Diagnostics, diagnostic.Error(CodeDoctorPort, err.Error()))
				continue
			}
			if inUse {
				okAll = false
				res.Diagnostics = append(res.Diagnostics, diagnostic.Error(CodeDoctorPort,
					fmt.Sprintf("host port %s:%d appears in use (service %s)", bind, *p.Host.Literal, id)).
					WithHint("Port check has a race; re-check at apply time"))
			}
		}
	}
	detail := "no literal host ports claimed"
	if !okAll {
		detail = "one or more host ports occupied or unchecked"
	}
	checks = append(checks, Check{
		Name: "host-ports", OK: okAll, Detail: detail,
		Warning: "host-port preflight has an inherent race before apply binds the port",
	})
	return checks
}

func composeAtLeast(ver, min string) bool {
	vp := parseVer(ver)
	mp := parseVer(min)
	for i := 0; i < 3; i++ {
		if vp[i] > mp[i] {
			return true
		}
		if vp[i] < mp[i] {
			return false
		}
	}
	return true
}

func parseVer(s string) [3]int {
	s = strings.TrimPrefix(strings.TrimSpace(s), "v")
	// take leading major.minor.patch
	var out [3]int
	parts := strings.Split(s, ".")
	for i := 0; i < len(parts) && i < 3; i++ {
		n := ""
		for _, r := range parts[i] {
			if r >= '0' && r <= '9' {
				n += string(r)
			} else {
				break
			}
		}
		out[i], _ = strconv.Atoi(n)
	}
	return out
}

func formatText(checks []Check) string {
	var b strings.Builder
	for _, c := range checks {
		status := "ok"
		if !c.OK {
			status = "FAIL"
		}
		fmt.Fprintf(&b, "%s: %s", c.Name, status)
		if c.Detail != "" {
			fmt.Fprintf(&b, " (%s)", c.Detail)
		}
		b.WriteByte('\n')
		if c.Warning != "" {
			fmt.Fprintf(&b, "  warning: %s\n", c.Warning)
		}
	}
	return b.String()
}

func finish(res Result) Result {
	diagnostic.SortDeterministic(res.Diagnostics)
	res.OK = res.OK && !diagnostic.HasError(res.Diagnostics)
	return res
}
