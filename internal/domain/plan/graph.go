// Package plan defines the deterministic lifecycle action graph.
package plan

// ActionKind classifies a planned action.
type ActionKind string

const (
	KindVerify            ActionKind = "verify"
	KindProvisionNetwork  ActionKind = "provision.network"
	KindProvisionVolume   ActionKind = "provision.volume"
	KindProvisionResource ActionKind = "provision.resource"
	KindImageAvailable    ActionKind = "image.available"
	KindTaskRun           ActionKind = "task.run"
	KindServiceStart      ActionKind = "service.start"
	KindServiceReady      ActionKind = "service.ready"
	KindFinalize          ActionKind = "finalize"
)

// Action is one node in the lifecycle DAG.
type Action struct {
	ID     string     `json:"id"`
	Kind   ActionKind `json:"kind"`
	Target string     `json:"target,omitempty"`
	Phase  string     `json:"phase,omitempty"`
	Rerun  string     `json:"rerun,omitempty"` // task rerun policy; empty for non-tasks
	Label  string     `json:"label"`
}

// Edge is a prerequisite → dependent edge.
type Edge struct {
	From      string `json:"from"`
	To        string `json:"to"`
	Condition string `json:"condition,omitempty"`
}

// Graph is the deterministic action DAG plus a topological order.
type Graph struct {
	Actions        []Action `json:"actions"`
	Edges          []Edge   `json:"edges"`
	Order          []string `json:"order"`
	DeploymentHash string   `json:"deploymentHash"`
}
