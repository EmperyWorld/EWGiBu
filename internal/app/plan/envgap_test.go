package plan

import (
	"testing"

	"iacpack/internal/domain/ir"
)

func TestMissingEnvKeysClearedByEnvironmentFrom(t *testing.T) {
	dep := ir.Deployment{
		Services: map[string]ir.Service{
			"api": {
				Image: "app",
				Environment: map[string]ir.ValueSource{
					"LLAMA_MODEL": {Literal: "m.gguf"},
					"PORT":        {Literal: "8080"},
				},
			},
			"beat": {
				Image: "app",
				Environment: map[string]ir.ValueSource{
					"PORT": {Literal: "8090"},
				},
			},
		},
	}
	gaps := missingEnvKeys(dep)
	found := false
	for _, k := range gaps["beat"] {
		if k == "LLAMA_MODEL" {
			found = true
		}
	}
	if !found {
		t.Fatalf("gaps=%v", gaps)
	}
	beat := dep.Services["beat"]
	beat.EnvironmentFrom = "api"
	dep.Services["beat"] = beat
	gaps = missingEnvKeys(dep)
	if len(gaps["beat"]) != 0 {
		t.Fatalf("inherited service still missing %v", gaps["beat"])
	}
}
