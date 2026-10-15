package compile

import (
	"encoding/json"
	"fmt"

	"iacpack/internal/domain/ir"
)

// UnmarshalDeployment parses a deployment.json document.
func UnmarshalDeployment(raw []byte) (ir.Deployment, error) {
	var d ir.Deployment
	if err := json.Unmarshal(raw, &d); err != nil {
		return ir.Deployment{}, err
	}
	if d.FormatVersion != ir.FormatVersion {
		return ir.Deployment{}, fmt.Errorf("unsupported deployment formatVersion %q", d.FormatVersion)
	}
	if d.Project.Name == "" {
		return ir.Deployment{}, fmt.Errorf("deployment missing project.name")
	}
	return d, nil
}
