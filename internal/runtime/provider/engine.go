package provider

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"iacpack/internal/runtime/envsanitize"
	"iacpack/internal/runtime/process"
)

// ContainerEngine is the local CLI used for compose-compatible operations.
type ContainerEngine string

const (
	EnginePodman ContainerEngine = "podman"
	EngineDocker ContainerEngine = "docker"
)

// EnvContainerEngine selects podman|docker when set (IACPACK_CONTAINER_ENGINE).
const EnvContainerEngine = "IACPACK_CONTAINER_ENGINE"

// ResolveContainerEngine picks podman|docker. Empty explicit uses env, then auto
// (Podman first, Docker second).
func ResolveContainerEngine(explicit string) (ContainerEngine, error) {
	explicit = strings.ToLower(strings.TrimSpace(explicit))
	if explicit == "" {
		explicit = strings.ToLower(strings.TrimSpace(os.Getenv(EnvContainerEngine)))
	}
	switch explicit {
	case "":
		return autoContainerEngine(context.Background(), process.OSRunner{})
	case string(EnginePodman):
		return EnginePodman, nil
	case string(EngineDocker):
		return EngineDocker, nil
	default:
		return "", fmt.Errorf("unsupported container engine %q (use podman or docker)", explicit)
	}
}

func autoContainerEngine(ctx context.Context, runner process.Runner) (ContainerEngine, error) {
	ctx, cancel := context.WithTimeout(ctx, 8*time.Second)
	defer cancel()
	if engineUsable(ctx, runner, EnginePodman) {
		return EnginePodman, nil
	}
	if engineUsable(ctx, runner, EngineDocker) {
		return EngineDocker, nil
	}
	return "", fmt.Errorf("no usable Podman or Docker engine (need compose subcommand)")
}

func engineUsable(ctx context.Context, runner process.Runner, engine ContainerEngine) bool {
	bin := string(engine)
	env := envsanitize.CleanEnviron(os.Environ())
	res, err := runner.Run(ctx, process.Spec{Path: bin, Args: []string{"info"}, Env: env})
	if err != nil || res.ExitCode != 0 {
		return false
	}
	res, err = runner.Run(ctx, process.Spec{Path: bin, Args: []string{"compose", "version"}, Env: env})
	return err == nil && res.ExitCode == 0
}

// SelectContainerSocket returns the Unix socket for the given engine.
// override may come from --docker-socket / IACPACK_DOCKER_SOCKET.
func SelectContainerSocket(engine ContainerEngine, override string) (string, error) {
	if o := strings.TrimSpace(override); o != "" {
		return envsanitize.SelectDockerSocket(o)
	}
	if env := strings.TrimSpace(os.Getenv("IACPACK_DOCKER_SOCKET")); env != "" {
		return envsanitize.SelectDockerSocket(env)
	}
	if engine == EnginePodman {
		if s := defaultPodmanSocket(); s != "" {
			return envsanitize.SelectDockerSocket(s)
		}
	}
	return envsanitize.SelectDockerSocket("")
}

func defaultPodmanSocket() string {
	if s := strings.TrimSpace(os.Getenv("IACPACK_CONTAINER_SOCKET")); s != "" {
		return s
	}
	base := strings.TrimSpace(os.Getenv("XDG_RUNTIME_DIR"))
	if base == "" {
		base = filepath.Join("/run/user", strconv.Itoa(os.Getuid()))
	}
	return filepath.Join(base, "podman", "podman.sock")
}

func (h Host) resolvedEngine() (ContainerEngine, error) {
	if h.Engine != "" {
		e := ContainerEngine(strings.ToLower(strings.TrimSpace(h.Engine)))
		switch e {
		case EnginePodman, EngineDocker:
			return e, nil
		default:
			return "", fmt.Errorf("unsupported container engine %q", h.Engine)
		}
	}
	return ResolveContainerEngine("")
}

func (h Host) cliBin() (string, error) {
	e, err := h.resolvedEngine()
	if err != nil {
		return "", err
	}
	return string(e), nil
}

func (h Host) runtimeEnv() ([]string, error) {
	e, err := h.resolvedEngine()
	if err != nil {
		return nil, err
	}
	socket, err := SelectContainerSocket(e, h.Socket)
	if err != nil {
		return nil, err
	}
	env := envsanitize.CleanEnviron(os.Environ())
	return append(env, "DOCKER_HOST=unix://"+socket, "COMPOSE_IGNORE_ORPHANS=true"), nil
}
