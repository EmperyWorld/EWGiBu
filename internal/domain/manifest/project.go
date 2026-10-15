// Package manifest holds the immutable authoring domain model for a Project.
package manifest

import "iacpack/internal/domain/path"

// Mode is the packaging/validation mode.
type Mode string

const (
	ModeDevelopment Mode = "development"
	ModeRelease     Mode = "release"
)

// Profile is the provider profile identifier.
type Profile string

const (
	ProfileComposeDockerV1 Profile = "compose-docker/v1"
)

// APIVersionV1Alpha1 is the initial authoring schema version.
const APIVersionV1Alpha1 = "iacpack.io/v1alpha1"

// KindProject is the only supported kind in v1.
const KindProject = "Project"

// Project is the immutable validated authoring model.
type Project struct {
	APIVersion string
	Kind       string
	Metadata   Metadata
	Spec       Spec
}

// Metadata identifies the packaged project.
type Metadata struct {
	Name        string
	Version     string
	Description string // optional; <= 4 KiB
}

// Spec holds the full v1alpha1 authoring surface.
type Spec struct {
	Target     Target
	Mode       Mode
	Parameters map[string]Parameter
	Secrets    map[string]Secret
	Images     map[string]Image
	Builds     map[string]PackageBuild
	Resources  map[string]Resource
	Networks   map[string]Network
	Volumes    map[string]Volume
	Services   map[string]Service
	Tasks      map[string]Task
	Databases  map[string]Database
	Package    Package
	XUI        *XUI // optional; excluded from semantic hash
}

// Target selects the provider profile.
type Target struct {
	Profile Profile
}

// ParameterPhase is build or deploy.
type ParameterPhase string

const (
	ParameterPhaseBuild  ParameterPhase = "build"
	ParameterPhaseDeploy ParameterPhase = "deploy"
)

// ParameterType is the typed parameter kind.
type ParameterType string

const (
	ParameterTypeString  ParameterType = "string"
	ParameterTypeInteger ParameterType = "integer"
	ParameterTypeNumber  ParameterType = "number"
	ParameterTypeBoolean ParameterType = "boolean"
	ParameterTypeEnum    ParameterType = "enum"
)

// Parameter is a non-secret typed value declaration.
type Parameter struct {
	Phase       ParameterPhase
	Type        ParameterType
	Default     *Scalar
	Minimum     *float64
	Maximum     *float64
	Pattern     string
	Choices     []Scalar
	Description string
}

// Scalar is a JSON-like scalar used for defaults and choices.
type Scalar struct {
	Kind ScalarKind
	Str  string
	Num  float64
	Bool bool
}

// ScalarKind classifies a Scalar.
type ScalarKind int

const (
	ScalarString ScalarKind = iota
	ScalarNumber
	ScalarBool
)

// Secret is a declaration only (never a value).
type Secret struct {
	Required    bool
	Delivery    string // "file"
	Description string
}

// Image is a discriminated union: pull ref or development build.
type Image struct {
	// Pull arm. Ref is a literal image reference. RefParameter is set instead
	// when ref is {parameter: id} and no default has been substituted.
	Ref          string
	RefParameter string
	Delivery     string // "pull"
	// Build arm (development only)
	Build *ImageBuild
}

// IsBuild reports whether this image uses a local build context.
func (i Image) IsBuild() bool { return i.Build != nil }

// ImageBuild is a development-only Dockerfile context.
type ImageBuild struct {
	Context         path.WorkspacePath
	Dockerfile      path.WorkspacePath
	Include         []string
	Exclude         []string
	Args            map[string]ValueSource
	ExpectedImageID string // optional sha256:<64-hex>
}

// ValueSource is a tagged literal, parameter, or secret reference, or a
// concatenation of those (Parts). A secret arm names a spec.secrets id.
// It never stores the secret value.
type ValueSource struct {
	Literal   *Scalar
	Parameter string // parameter id when set
	Secret    string // secret id when set
	Parts     []ValueSource
}

// IsParameter reports a parameter reference.
func (v ValueSource) IsParameter() bool { return v.Parameter != "" }

// IsSecret reports a secret reference.
func (v ValueSource) IsSecret() bool { return v.Secret != "" }

// PackageBuild is a frontend package-manager build executed by `iacpack build`.
// The result is copied into the bundle payload. Apply only mounts that payload.
type PackageBuild struct {
	Context        path.WorkspacePath
	PackageManager string // pnpm|npm|yarn|bun
	Install        string // frozen
	Script         string
	Output         string // relative to context
	Entry          string // file that must exist in output; default index.html
}

// Resource is an explicitly included file set.
type Resource struct {
	Source      path.WorkspacePath
	Include     []string
	Exclude     []string
	Destination path.BundlePath
	Mode        string // optional 0xxx
	AllowEmpty  bool
}

// Network is managed {} or external reference.
type Network struct {
	External *ExternalRef
}

// IsExternal reports an external network reference.
func (n Network) IsExternal() bool { return n.External != nil }

// Volume is managed retention or external reference.
type Volume struct {
	Retention string // retain|delete; empty means retain for managed
	External  *ExternalRef
}

// IsExternal reports an external volume reference.
func (v Volume) IsExternal() bool { return v.External != nil }

// ExternalRef names a pre-existing provider resource.
type ExternalRef struct {
	Name string
}

// ArgvPart is a literal fragment, parameter, or secret reference inside one argv element.
// A secret arm names a spec.secrets id. It never stores the secret value.
type ArgvPart struct {
	Literal   string
	Parameter string
	Secret    string
}

// ArgvElement is one command argument.
// Secret, when set, is the whole argument (rendered as a Compose interpolation).
// Parts, when set, are concatenated at render.
type ArgvElement struct {
	Literal string
	Secret  string
	Parts   []ArgvPart
}

// Service is a long-running container declaration.
type Service struct {
	Image           string
	User            string // uid:gid or user name; empty leaves the image USER
	Command         []ArgvElement
	Entrypoint      []ArgvElement
	Environment     map[string]ValueSource
	EnvironmentFrom string // service id whose env is copied, then overridden
	Secrets         []SecretMount
	Ports           []Port
	Mounts          []Mount
	Networks        []string
	Healthcheck     *Healthcheck
	Restart         string
	Resources       *ResourceLimits
	DependsOn       []Dependency
	Profiles        []string
}

// Task is a one-shot lifecycle container.
type Task struct {
	Phase           TaskPhase
	Image           string
	Command         []ArgvElement
	Entrypoint      []ArgvElement
	ResetEntrypoint bool
	User            string
	Workdir         path.ContainerPath
	DependsOn       []Dependency
	Timeout         string
	Retry           *Retry
	Rerun           string
	Mounts          []Mount
	Networks        []string
	Secrets         []SecretMount
}

// Database is a second database on an existing server service.
// The password secret is not stored here.
type Database struct {
	On           string // service id that serves the database
	Name         string // database name
	OwnerFrom    string // service id whose account is reused
	Migrations   string // workspace directory of SQL files
	URLParameter string // deploy parameter id expanded to the DSN
}

// TaskPhase is the lifecycle phase.
type TaskPhase string

const (
	TaskPhasePreStart   TaskPhase = "preStart"
	TaskPhaseMigration  TaskPhase = "migration"
	TaskPhasePostDeploy TaskPhase = "postDeploy"
)

// SecretMount mounts a declared secret as a file.
type SecretMount struct {
	Ref    string
	Target path.ContainerPath
}

// Port maps a container port to an optional host binding.
type Port struct {
	Container int
	Host      *HostPort // optional
	Bind      string    // IP literal; empty means unspecified
	Protocol  string    // tcp|udp
}

// HostPort is a literal integer or parameter reference.
type HostPort struct {
	Literal   *int
	Parameter string
}

// Mount is a volume, a bundled resource, or an external host bind.
// HostLiteral / HostParameter name a host directory that is not copied into the bundle.
type Mount struct {
	Volume        string
	Resource      string
	Build         string // spec.builds id; payload is produced by iacpack build
	Source        string // optional resource subpath
	HostLiteral   string
	HostParameter string
	Target        path.ContainerPath
	ReadOnly      bool
}

// Dependency is a readiness edge.
type Dependency struct {
	Target    string
	Condition string // started|healthy|completedSuccessfully
}

// Healthcheck is an executable argv probe (no shell).
type Healthcheck struct {
	Command     []string
	Interval    string
	Timeout     string
	StartPeriod string
	Retries     int
}

// ResourceLimits are supported Compose subset ceilings.
type ResourceLimits struct {
	CPUs   string // decimal string
	Memory int64  // bytes; 0 means unset
	GPU    int    // NVIDIA GPU count 1..8; 0 means unset (ADR-021)
	GPUAll bool   // count: all
	// GPUDevicesOnly passes host /dev/nvidia* and does not require CDI.
	// LDLibraryPath is the container path of libcuda from the image (for example /usr/local/cuda/compat).
	GPUDevicesOnly bool
	LDLibraryPath  string
}

// Retry configures task retries.
type Retry struct {
	Count          int
	InitialBackoff string
	Multiplier     float64
	MaxBackoff     string
}

// Package holds packaging options with schema defaults applied.
type Package struct {
	GenerateReadme     bool
	GenerateEnvExample bool
	IncludeRuntime     bool
	EmbedImages        bool
	Set                bool // true if package block was present
}

// XUI is UI-only metadata excluded from semantic hashing.
type XUI struct {
	Nodes    map[string]XUINode
	Viewport *XUIViewport
}

// XUINode is a canvas node position.
type XUINode struct {
	X        float64
	Y        float64
	Category string // optional
}

// XUIViewport is the optional canvas viewport.
type XUIViewport struct {
	X    float64
	Y    float64
	Zoom float64
}
