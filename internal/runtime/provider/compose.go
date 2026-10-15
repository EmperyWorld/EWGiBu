// Package provider defines the Docker Compose runtime interface (mutation methods deferred to P08).
package provider

import (
	"context"

	"iacpack/internal/runtime/process"
)

// DockerSocketDefault is the v1 local socket.
const DockerSocketDefault = "/var/run/docker.sock"

// ReadOnly probes are allowed in P07 doctor/plan.
type ReadOnly interface {
	Platform(ctx context.Context) (PlatformInfo, error)
	DockerVersion(ctx context.Context) (DockerInfo, error)
	ComposeVersion(ctx context.Context) (string, error)
	DaemonReachable(ctx context.Context) error
	DiskAvailableBytes(ctx context.Context, path string) (uint64, error)
	HostPortInUse(ctx context.Context, bind string, port int) (bool, error)
}

// PlatformInfo is host platform metadata.
type PlatformInfo struct {
	OS   string
	Arch string
}

// DockerInfo is engine metadata.
type DockerInfo struct {
	Version    string
	APIVersion string
}

// Mutator will be implemented in P08; defined here so recording fakes can assert zero calls.
type Mutator interface {
	Up(ctx context.Context, project string, composeFile string) error
	Down(ctx context.Context, project string) error
}

// Recording is a fake provider for plan/doctor tests.
type Recording struct {
	Runner        *process.RecordingRunner
	PlatformValue PlatformInfo
	DockerValue   DockerInfo
	ComposeVer    string
	DaemonErr     error
	DiskBytes     uint64
	PortsInUse    map[string]bool // "bind:port"
	Mutations     int
	ReadCalls     int
}

// Platform implements ReadOnly.
func (r *Recording) Platform(ctx context.Context) (PlatformInfo, error) {
	r.ReadCalls++
	_ = ctx
	if r.PlatformValue.OS == "" {
		return PlatformInfo{OS: "linux", Arch: "amd64"}, nil
	}
	return r.PlatformValue, nil
}

// DockerVersion implements ReadOnly.
func (r *Recording) DockerVersion(ctx context.Context) (DockerInfo, error) {
	r.ReadCalls++
	_ = ctx
	if r.DockerValue.Version == "" {
		return DockerInfo{Version: "24.0.0", APIVersion: "1.43"}, nil
	}
	return r.DockerValue, nil
}

// ComposeVersion implements ReadOnly.
func (r *Recording) ComposeVersion(ctx context.Context) (string, error) {
	r.ReadCalls++
	_ = ctx
	if r.ComposeVer == "" {
		return "2.24.0", nil
	}
	return r.ComposeVer, nil
}

// DaemonReachable implements ReadOnly.
func (r *Recording) DaemonReachable(ctx context.Context) error {
	r.ReadCalls++
	_ = ctx
	return r.DaemonErr
}

// DiskAvailableBytes implements ReadOnly.
func (r *Recording) DiskAvailableBytes(ctx context.Context, path string) (uint64, error) {
	r.ReadCalls++
	_ = ctx
	_ = path
	if r.DiskBytes == 0 {
		return 50 << 30, nil
	}
	return r.DiskBytes, nil
}

// HostPortInUse implements ReadOnly.
func (r *Recording) HostPortInUse(ctx context.Context, bind string, port int) (bool, error) {
	r.ReadCalls++
	_ = ctx
	key := bind + ":" + itoa(port)
	return r.PortsInUse[key], nil
}

// Up records a forbidden mutation in P07 tests.
func (r *Recording) Up(ctx context.Context, project string, composeFile string) error {
	_ = ctx
	_ = project
	_ = composeFile
	r.Mutations++
	return nil
}

// Down records a forbidden mutation in P07 tests.
func (r *Recording) Down(ctx context.Context, project string) error {
	_ = ctx
	_ = project
	r.Mutations++
	return nil
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var buf [20]byte
	i := len(buf)
	for n > 0 {
		i--
		buf[i] = byte('0' + n%10)
		n /= 10
	}
	return string(buf[i:])
}
