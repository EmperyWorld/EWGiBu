package secrets

import "strings"

// ComposeVar is the process environment name apply sets from a secret file.
// The rendered Compose file interpolates ${name}. The value is never a CLI argument.
func ComposeVar(id string) string {
	return "IACPACK_SECRET_" + strings.ToUpper(strings.ReplaceAll(id, "-", "_"))
}

// ComposePlaceholder is the Compose interpolation written for an environment secret reference.
func ComposePlaceholder(id string) string {
	return "${" + ComposeVar(id) + "}"
}
