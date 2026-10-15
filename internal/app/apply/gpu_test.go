package apply

import (
	"strings"
	"testing"

	"iacpack/internal/domain/ir"
	"iacpack/internal/render/compose"
	"iacpack/internal/runtime/provider"
)

func TestNVIDIAControlNodeIncludesCapsChildren(t *testing.T) {
	if !nvidiaControlNode("nvidia0") || !nvidiaControlNode("nvidiactl") || !nvidiaControlNode("nvidia-uvm") {
		t.Fatal("expected gpu nodes")
	}
	if nvidiaControlNode("nvidia-caps") || nvidiaControlNode("nvidia-modeset-extra") {
		t.Fatal("directory and unknown names are not device nodes")
	}
}

func TestClearGPUAllOmitsCountAll(t *testing.T) {
	orig := ir.Deployment{
		Project: ir.ProjectRef{Name: "example", Version: "0.1.0"},
		Images: map[string]ir.Image{
			"app": {Ref: "docker.io/library/nginx@sha256:0000000000000000000000000000000000000000000000000000000000000001"},
		},
		Services: map[string]ir.Service{
			"llama-chat": {
				Image:     "app",
				Resources: &ir.ResourceLimits{Memory: 1048576, GPUAll: true},
			},
			"app": {Image: "app"},
		},
	}
	renderDep := orig
	renderDep.Services = cloneServiceResources(orig.Services)
	cleared := clearGPUAll(&renderDep, "llama-chat")
	if len(cleared) != 1 || cleared[0] != "llama-chat" {
		t.Fatalf("cleared=%v", cleared)
	}
	if !orig.Services["llama-chat"].Resources.GPUAll {
		t.Fatal("source deployment must keep gpu all")
	}
	body, err := renderComposeBytes(renderDep, compose.RenderOptions{})
	if err != nil {
		t.Fatal(err)
	}
	text := string(body)
	if strings.Contains(text, "count: all") || strings.Contains(text, "devices:") {
		t.Fatalf("reservation still required:\n%s", text)
	}
	if !strings.Contains(text, "1048576") {
		t.Fatalf("memory limit dropped:\n%s", text)
	}
	msg := gpuStartMessage("llama-chat", provider.NVIDIACountAllCDIDevice)
	if !strings.Contains(msg, "llama-chat") || !strings.Contains(msg, provider.NVIDIACountAllCDIDevice) {
		t.Fatal(msg)
	}
}
