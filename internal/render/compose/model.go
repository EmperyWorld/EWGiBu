// Package compose renders a typed Docker Compose document for compose-docker/v1.
package compose

// File is the root Compose document (Compose Specification subset).
type File struct {
	Name     string
	Services map[string]Service
	Networks map[string]Network
	Volumes  map[string]Volume
	Secrets  map[string]Secret
}

// Service is a Compose service.
type Service struct {
	Image      string
	User       string
	Build      *Build
	Command    []string
	Entrypoint []string
	// EntrypointClear emits `entrypoint: []` so the image ENTRYPOINT is not used.
	EntrypointClear bool
	Environment     map[string]string // resolved non-secret only
	Ports           []Port
	Networks        []string
	Volumes         []VolumeMount
	Secrets         []SecretRef
	Devices         []string
	SecurityOpt     []string
	DependsOn       map[string]DependsOn
	Healthcheck     *Healthcheck
	Restart         string
	WorkingDir      string
	Profiles        []string // e.g. iacpack-task — excluded from default compose up
	Deploy          *Deploy
}

// Build is a development image build declaration.
type Build struct {
	Context    string
	Dockerfile string
	Args       map[string]string
}

// Port is host:container publishing.
type Port struct {
	Target    int
	Published string // may be literal or empty
	HostIP    string
	Protocol  string
}

// VolumeMount binds a host-relative or named volume into the container.
type VolumeMount struct {
	Type     string // bind|volume
	Source   string
	Target   string
	ReadOnly bool
	// Relabel is a Compose bind SELinux option, "z" or "Z". Empty leaves the label unchanged.
	Relabel string
}

// SecretRef mounts a compose secret into the container.
type SecretRef struct {
	Source string
	Target string
}

// DependsOn is a Compose condition dependency.
type DependsOn struct {
	Condition string // service_started|service_healthy|service_completed_successfully
}

// Healthcheck is an argv probe.
type Healthcheck struct {
	Test        []string
	Interval    string
	Timeout     string
	StartPeriod string
	Retries     int
}

// Deploy holds resource limits (Compose deploy.resources.limits).
type Deploy struct {
	CPUs           string
	Memory         string
	GPU            int
	GPUAll         bool
	GPUDevicesOnly bool
}

// Network is a Compose network.
type Network struct {
	External bool
	Name     string // external name
}

// Volume is a Compose named volume.
type Volume struct {
	External bool
	Name     string
}

// Secret is a Compose secret (file path into artifact tree for preview).
type Secret struct {
	File string
}
