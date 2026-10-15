package images

import (
	"context"
	"fmt"
)

// DockerCLIFetcher is the production network fetcher placeholder.
// Full docker pull/save→OCI conversion is invoked when docker is available;
// until then it fails closed with an actionable diagnostic (tests use FixtureFetcher).
type DockerCLIFetcher struct{}

// Fetch implements Fetcher.
func (DockerCLIFetcher) Fetch(ctx context.Context, ref, digest string) ([]byte, map[string][]byte, error) {
	_ = ctx
	return nil, nil, fmt.Errorf("docker OCI fetch not available in this environment for %s@%s; use a provisioned CI job or FixtureFetcher in tests", ref, digest)
}
