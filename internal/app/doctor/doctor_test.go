package doctor_test

import (
	"context"
	"fmt"
	"testing"

	"iacpack/internal/app/doctor"
	"iacpack/internal/runtime/provider"
)

func TestDoctorMatrix(t *testing.T) {
	ctx := context.Background()
	cases := []struct {
		name string
		mod  func(*provider.Recording)
		ok   bool
	}{
		{"success", func(r *provider.Recording) {}, true},
		{"bad-os", func(r *provider.Recording) { r.PlatformValue = provider.PlatformInfo{OS: "windows", Arch: "amd64"} }, false},
		{"daemon", func(r *provider.Recording) { r.DaemonErr = fmt.Errorf("denied") }, false},
		{"low-disk", func(r *provider.Recording) { r.DiskBytes = 100 }, false},
		{"old-compose", func(r *provider.Recording) { r.ComposeVer = "2.10.0" }, false},
		{"port", func(r *provider.Recording) { r.PortsInUse = map[string]bool{"0.0.0.0:8080": true} }, true}, // no bundle => ports skipped
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rec := &provider.Recording{}
			tc.mod(rec)
			res := doctor.Run(ctx, doctor.Options{Provider: rec, MinDiskBytes: 1 << 20, ContainerEngine: "docker"})
			if res.OK != tc.ok {
				t.Fatalf("ok=%v want %v diags=%v", res.OK, tc.ok, res.Diagnostics)
			}
			if rec.Mutations != 0 {
				t.Fatal("mutations")
			}
		})
	}
}

func TestDoctorPodmanComposeDoesNotRequireDockerPlugin(t *testing.T) {
	rec := &provider.Recording{ComposeVer: "1.6.0"}
	res := doctor.Run(context.Background(), doctor.Options{
		Provider: rec, ContainerEngine: "podman", MinDiskBytes: 1 << 20,
	})
	if !res.OK {
		t.Fatalf("podman-compose 1.6 must pass: %v", res.Diagnostics)
	}
}

func TestDoctorOccupiedPortWithBundleSkippedWithoutBundle(t *testing.T) {
	rec := &provider.Recording{PortsInUse: map[string]bool{"0.0.0.0:80": true}}
	res := doctor.Run(context.Background(), doctor.Options{Provider: rec})
	if !res.OK {
		t.Fatalf("without bundle should succeed: %v", res.Diagnostics)
	}
}
