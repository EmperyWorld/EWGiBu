// Package projectinit generates the minimal Project manifest for `iacpack init`.
package projectinit

// MinimalYAML is the deterministic minimal release manifest template.
// The digest is an intentional documentation placeholder, not a real image.
const MinimalYAML = `apiVersion: iacpack.io/v1alpha1
kind: Project
metadata:
  name: example
  version: 0.1.0
spec:
  target:
    profile: compose-docker/v1
  mode: release
  images:
    app:
      ref: docker.io/library/nginx@sha256:0000000000000000000000000000000000000000000000000000000000000001
      delivery: pull
  services:
    app:
      image: app
`

// DefaultOutputName is the conventional authoring filename.
const DefaultOutputName = "iacpack.yaml"

// Content returns the exact bytes written by init (LF, no BOM).
func Content() []byte {
	return []byte(MinimalYAML)
}
