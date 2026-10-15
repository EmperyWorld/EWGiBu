// Package ir defines the immutable compiled deployment model (separate from authoring DTOs).
package ir

// FormatVersion identifies the deployment document contract.
const FormatVersion = "iacpack.deployment/v1"

// Deployment is the redacted compiled model consumed by renderers/runtime.
// It contains no authoring source locations, local absolute paths, x-ui, or secret values.
type Deployment struct {
	FormatVersion string                  `json:"formatVersion"`
	Project       ProjectRef              `json:"project"`
	Mode          string                  `json:"mode"`
	Profile       string                  `json:"profile"`
	SemanticHash  string                  `json:"semanticHash"`
	Parameters    map[string]Parameter    `json:"parameters"`
	Secrets       map[string]Secret       `json:"secrets"`
	Images        map[string]Image        `json:"images"`
	Builds        map[string]PackageBuild `json:"builds,omitempty"`
	Resources     map[string]Resource     `json:"resources"`
	Networks      map[string]Network      `json:"networks"`
	Volumes       map[string]Volume       `json:"volumes"`
	Services      map[string]Service      `json:"services"`
	Tasks         map[string]Task         `json:"tasks"`
	Package       Package                 `json:"package"`
}

// ProjectRef is project identity.
type ProjectRef struct {
	Name        string `json:"name"`
	Version     string `json:"version"`
	Description string `json:"description,omitempty"`
}

// Parameter is a non-secret typed declaration (values may be unresolved deploy params).
type Parameter struct {
	Phase       string `json:"phase"`
	Type        string `json:"type"`
	Default     any    `json:"default,omitempty"`
	Description string `json:"description,omitempty"`
}

// Secret is a declaration only.
type Secret struct {
	Required    bool   `json:"required"`
	Delivery    string `json:"delivery"`
	Description string `json:"description,omitempty"`
}

// Image is pull or build metadata (no registry credentials).
type Image struct {
	Ref          string      `json:"ref,omitempty"`
	RefParameter string      `json:"refParameter,omitempty"`
	Delivery     string      `json:"delivery,omitempty"`
	Build        *ImageBuild `json:"build,omitempty"`
}

// ImageBuild is development-only build context metadata.
type ImageBuild struct {
	Context    string   `json:"context"`
	Dockerfile string   `json:"dockerfile"`
	Include    []string `json:"include"`
	Exclude    []string `json:"exclude,omitempty"`
}

// PackageBuild is the compiled frontend build declaration.
type PackageBuild struct {
	Context        string `json:"context"`
	PackageManager string `json:"packageManager"`
	Install        string `json:"install"`
	Script         string `json:"script"`
	Output         string `json:"output"`
	Entry          string `json:"entry"`
}

// Resource is a declared payload set (workspace-relative paths only).
type Resource struct {
	Source      string   `json:"source"`
	Include     []string `json:"include"`
	Exclude     []string `json:"exclude,omitempty"`
	Destination string   `json:"destination,omitempty"`
	Mode        string   `json:"mode,omitempty"`
	AllowEmpty  bool     `json:"allowEmpty"`
}

// Network is managed or external.
type Network struct {
	External *ExternalRef `json:"external,omitempty"`
}

// Volume is managed retention or external.
type Volume struct {
	Retention string       `json:"retention,omitempty"`
	External  *ExternalRef `json:"external,omitempty"`
}

// ExternalRef names a pre-existing provider resource.
type ExternalRef struct {
	Name string `json:"name"`
}

// ArgvPart is a literal fragment, deploy parameter, or secret id inside one argument.
type ArgvPart struct {
	Literal   string `json:"literal,omitempty"`
	Parameter string `json:"parameter,omitempty"`
	Secret    string `json:"secret,omitempty"`
}

// ArgvElement is one command argument. Parts are concatenated at render.
// Secret is the whole argument when set.
type ArgvElement struct {
	Literal string     `json:"literal,omitempty"`
	Secret  string     `json:"secret,omitempty"`
	Parts   []ArgvPart `json:"parts,omitempty"`
}

// LiteralArgs builds argv elements that are plain strings.
func LiteralArgs(items ...string) []ArgvElement {
	out := make([]ArgvElement, len(items))
	for i, s := range items {
		out[i] = ArgvElement{Literal: s}
	}
	return out
}

// Service is a long-running container in IR form.
type Service struct {
	Image           string                 `json:"image"`
	User            string                 `json:"user,omitempty"`
	Command         []ArgvElement          `json:"command,omitempty"`
	Entrypoint      []ArgvElement          `json:"entrypoint,omitempty"`
	Environment     map[string]ValueSource `json:"environment,omitempty"`
	EnvironmentFrom string                 `json:"environmentFrom,omitempty"`
	Secrets         []SecretMount          `json:"secrets,omitempty"`
	Ports           []Port                 `json:"ports,omitempty"`
	Mounts          []Mount                `json:"mounts,omitempty"`
	Networks        []string               `json:"networks,omitempty"`
	Healthcheck     *Healthcheck           `json:"healthcheck,omitempty"`
	Restart         string                 `json:"restart,omitempty"`
	Resources       *ResourceLimits        `json:"resources,omitempty"`
	DependsOn       []Dependency           `json:"dependsOn,omitempty"`
	Profiles        []string               `json:"profiles,omitempty"`
}

// Task is a one-shot lifecycle container.
type Task struct {
	Phase           string        `json:"phase"`
	Image           string        `json:"image"`
	Command         []ArgvElement `json:"command"`
	Entrypoint      []ArgvElement `json:"entrypoint,omitempty"`
	ResetEntrypoint bool          `json:"resetEntrypoint,omitempty"`
	User            string        `json:"user,omitempty"`
	Workdir         string        `json:"workdir,omitempty"`
	DependsOn       []Dependency  `json:"dependsOn,omitempty"`
	Timeout         string        `json:"timeout,omitempty"`
	Retry           *Retry        `json:"retry,omitempty"`
	Rerun           string        `json:"rerun,omitempty"`
	Mounts          []Mount       `json:"mounts,omitempty"`
	Networks        []string      `json:"networks,omitempty"`
	Secrets         []SecretMount          `json:"secrets,omitempty"`
	Environment     map[string]ValueSource `json:"environment,omitempty"`
}

// ValueSource is a literal, parameter, or secret reference, or concatenated Parts.
type ValueSource struct {
	Literal   any           `json:"literal,omitempty"`
	Parameter string        `json:"parameter,omitempty"`
	Secret    string        `json:"secret,omitempty"`
	Parts     []ValueSource `json:"parts,omitempty"`
}

// SecretMount mounts a declared secret as a file.
type SecretMount struct {
	Ref    string `json:"ref"`
	Target string `json:"target"`
}

// Port is a published port binding.
type Port struct {
	Container int       `json:"container"`
	Host      *HostPort `json:"host,omitempty"`
	Bind      string    `json:"bind,omitempty"`
	Protocol  string    `json:"protocol,omitempty"`
}

// HostPort is literal or parameter.
type HostPort struct {
	Literal   *int   `json:"literal,omitempty"`
	Parameter string `json:"parameter,omitempty"`
}

// Mount is a volume, bundled resource, or external host bind.
type Mount struct {
	Volume        string `json:"volume,omitempty"`
	Resource      string `json:"resource,omitempty"`
	Build         string `json:"build,omitempty"`
	Source        string `json:"source,omitempty"`
	HostLiteral   string `json:"hostLiteral,omitempty"`
	HostParameter string `json:"hostParameter,omitempty"`
	Target        string `json:"target"`
	ReadOnly      bool   `json:"readOnly"`
}

// Dependency is a readiness edge from authoring.
type Dependency struct {
	Target    string `json:"target"`
	Condition string `json:"condition"`
}

// Healthcheck is argv probe metadata.
type Healthcheck struct {
	Command     []string `json:"command"`
	Interval    string   `json:"interval"`
	Timeout     string   `json:"timeout"`
	StartPeriod string   `json:"startPeriod,omitempty"`
	Retries     int      `json:"retries"`
}

// ResourceLimits are CPU/memory ceilings.
type ResourceLimits struct {
	CPUs           string `json:"cpus,omitempty"`
	Memory         int64  `json:"memory,omitempty"`
	GPU            int    `json:"gpu,omitempty"`
	GPUAll         bool   `json:"gpuAll,omitempty"`
	GPUDevicesOnly bool   `json:"gpuDevicesOnly,omitempty"`
	LDLibraryPath  string `json:"ldLibraryPath,omitempty"`
}

// Retry configures bounded retries.
type Retry struct {
	Count          int     `json:"count"`
	InitialBackoff string  `json:"initialBackoff,omitempty"`
	Multiplier     float64 `json:"multiplier,omitempty"`
	MaxBackoff     string  `json:"maxBackoff,omitempty"`
}

// Package holds packaging options.
type Package struct {
	GenerateReadme     bool `json:"generateReadme"`
	GenerateEnvExample bool `json:"generateEnvExample"`
	IncludeRuntime     bool `json:"includeRuntime"`
	EmbedImages        bool `json:"embedImages"`
}
