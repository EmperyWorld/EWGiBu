package compose_test

import (
	"strings"
	"testing"

	"iacpack/internal/domain/ir"
	"iacpack/internal/lockfile"
	"iacpack/internal/render/compose"
)

func TestRenderDevicesOnlySkipsCDIReservation(t *testing.T) {
	ref := "docker.io/library/nginx@sha256:cccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccc"
	d := ir.Deployment{
		Project: ir.ProjectRef{Name: "demo"},
		Images:  map[string]ir.Image{"llama": {Ref: ref}},
		Services: map[string]ir.Service{
			"llama-chat": {
				Image: "llama",
				Resources: &ir.ResourceLimits{
					GPUDevicesOnly: true,
					LDLibraryPath:  "/usr/local/cuda/compat",
				},
			},
		},
	}
	f, err := compose.Render(d, compose.RenderOptions{
		Lock:              lockfile.Document{Images: map[string]lockfile.ImageLock{"llama": {Ref: ref}}},
		HostNVIDIADevices: []string{"/dev/nvidia0", "/dev/nvidiactl"},
		SELinuxEnforcing:  true,
	})
	if err != nil {
		t.Fatal(err)
	}
	raw, err := compose.MarshalYAML(f)
	if err != nil {
		t.Fatal(err)
	}
	text := string(raw)
	if strings.Contains(text, "count: all") {
		t.Fatalf("CDI reservation rendered:\n%s", text)
	}
	if !strings.Contains(text, "/dev/nvidia0") || !strings.Contains(text, "LD_LIBRARY_PATH") || !strings.Contains(text, "/usr/local/cuda/compat") {
		t.Fatalf("missing devices or library path:\n%s", text)
	}
	if !strings.Contains(text, `NVIDIA_VISIBLE_DEVICES: "0"`) {
		t.Fatalf("devices-only must name the mounted GPU index:\n%s", text)
	}
	if strings.Contains(text, "NVIDIA_VISIBLE_DEVICES: all") || strings.Contains(text, "NVIDIA_VISIBLE_DEVICES: void") || strings.Contains(text, "NVIDIA_DRIVER_CAPABILITIES") {
		t.Fatalf("CDI-triggering nvidia env rendered:\n%s", text)
	}
	if !strings.Contains(text, "label=disable") {
		t.Fatalf("SELinux still hides /dev/nvidia*:\n%s", text)
	}
}

func TestRenderDevicesOnlyOverridesAuthorVisibleDevices(t *testing.T) {
	ref := "docker.io/library/nginx@sha256:cccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccc"
	d := ir.Deployment{
		Project: ir.ProjectRef{Name: "demo"},
		Images:  map[string]ir.Image{"llama": {Ref: ref}},
		Services: map[string]ir.Service{
			"llama-chat": {
				Image: "llama",
				Environment: map[string]ir.ValueSource{
					"NVIDIA_VISIBLE_DEVICES":     {Literal: "all"},
					"NVIDIA_DRIVER_CAPABILITIES": {Literal: "compute,utility"},
					"LD_LIBRARY_PATH":            {Literal: "/usr/local/cuda/lib64"},
				},
				Resources: &ir.ResourceLimits{
					GPUDevicesOnly: true,
					LDLibraryPath:  "/usr/local/cuda/compat",
				},
			},
		},
	}
	f, err := compose.Render(d, compose.RenderOptions{
		Lock:              lockfile.Document{Images: map[string]lockfile.ImageLock{"llama": {Ref: ref}}},
		HostNVIDIADevices: []string{"/dev/nvidia0"},
	})
	if err != nil {
		t.Fatal(err)
	}
	raw, err := compose.MarshalYAML(f)
	if err != nil {
		t.Fatal(err)
	}
	text := string(raw)
	if strings.Contains(text, "NVIDIA_VISIBLE_DEVICES: all") || strings.Contains(text, "count: all") {
		t.Fatalf("author all still requests CDI:\n%s", text)
	}
	if !strings.Contains(text, `NVIDIA_VISIBLE_DEVICES: "0"`) || !strings.Contains(text, "/usr/local/cuda/compat") {
		t.Fatalf("missing GPU index or library path:\n%s", text)
	}
}

func TestVisibleNVIDIADevicesListsCardIndexes(t *testing.T) {
	got := compose.VisibleNVIDIADevices([]string{"/dev/nvidiactl", "/dev/nvidia1", "/dev/nvidia0", "/dev/nvidia-caps/nvidia-cap1"})
	if got != "0,1" {
		t.Fatalf("got %q", got)
	}
}
