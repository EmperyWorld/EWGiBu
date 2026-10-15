package provider

import (
	"context"
	"fmt"
	"net"
	"runtime"
	"strconv"
	"strings"
	"time"

	"iacpack/internal/runtime/process"
)

// Host is a read-only provider using local Podman/Docker CLI argv.
type Host struct {
	Runner process.Runner
	Socket string
	// Engine is podman|docker; empty resolves Podman first, then Docker.
	Engine string
}

func (h Host) runner() process.Runner {
	if h.Runner != nil {
		return h.Runner
	}
	return process.OSRunner{}
}

// Platform implements ReadOnly.
func (h Host) Platform(ctx context.Context) (PlatformInfo, error) {
	_ = ctx
	return PlatformInfo{OS: runtime.GOOS, Arch: runtime.GOARCH}, nil
}

// DockerVersion implements ReadOnly.
func (h Host) DockerVersion(ctx context.Context) (DockerInfo, error) {
	env, err := h.runtimeEnv()
	if err != nil {
		return DockerInfo{}, err
	}
	bin, err := h.cliBin()
	if err != nil {
		return DockerInfo{}, err
	}
	res, err := h.runner().Run(ctx, process.Spec{
		Path: bin,
		Args: []string{"version", "--format", "{{.Server.Version}}"},
		Env:  env,
	})
	if err != nil {
		return DockerInfo{}, err
	}
	if res.ExitCode != 0 {
		return DockerInfo{}, fmt.Errorf("%s version failed: %s", bin, strings.TrimSpace(string(res.Stderr)))
	}
	return DockerInfo{Version: strings.TrimSpace(string(res.Stdout))}, nil
}

// ComposeVersion implements ReadOnly.
func (h Host) ComposeVersion(ctx context.Context) (string, error) {
	env, err := h.runtimeEnv()
	if err != nil {
		return "", err
	}
	bin, err := h.cliBin()
	if err != nil {
		return "", err
	}
	res, err := h.runner().Run(ctx, process.Spec{
		Path: bin,
		Args: []string{"compose", "version", "--short"},
		Env:  env,
	})
	if err != nil {
		return "", err
	}
	if res.ExitCode != 0 {
		return "", fmt.Errorf("%s compose version failed: %s", bin, strings.TrimSpace(string(res.Stderr)))
	}
	return composeVersionLine(string(res.Stdout)), nil
}

func composeVersionLine(s string) string {
	lines := strings.Split(s, "\n")
	for i := len(lines) - 1; i >= 0; i-- {
		t := strings.TrimSpace(lines[i])
		if t == "" {
			continue
		}
		if t[0] >= '0' && t[0] <= '9' {
			return t
		}
	}
	return strings.TrimSpace(s)
}

// DaemonReachable implements ReadOnly.
func (h Host) DaemonReachable(ctx context.Context) error {
	env, err := h.runtimeEnv()
	if err != nil {
		return err
	}
	bin, err := h.cliBin()
	if err != nil {
		return err
	}
	format := "{{.ServerVersion}}"
	if e, err := h.resolvedEngine(); err == nil && e == EnginePodman {
		format = "{{.Version.Version}}"
	}
	res, err := h.runner().Run(ctx, process.Spec{
		Path: bin,
		Args: []string{"info", "--format", format},
		Env:  env,
	})
	if err != nil {
		return err
	}
	if res.ExitCode != 0 {
		return fmt.Errorf("%s engine not reachable", bin)
	}
	return nil
}

// HostPortInUse implements ReadOnly with a short dial probe (race-aware).
func (h Host) HostPortInUse(ctx context.Context, bind string, port int) (bool, error) {
	_ = h
	if bind == "0.0.0.0" || bind == "::" {
		bind = "127.0.0.1"
	}
	addr := net.JoinHostPort(bind, strconv.Itoa(port))
	d := net.Dialer{Timeout: 200 * time.Millisecond}
	conn, err := d.DialContext(ctx, "tcp", addr)
	if err != nil {
		return false, nil // cannot connect => likely free (or filtered)
	}
	_ = conn.Close()
	return true, nil
}
