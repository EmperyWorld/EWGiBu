// Package identity derives stable project/instance and provider slugs.
package identity

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"regexp"
	"strings"
)

// DefaultInstance is the CLI default for --instance.
const DefaultInstance = "default"

var instanceRe = regexp.MustCompile(`^[a-z][a-z0-9-]{0,30}$`)

// ValidateInstance checks the TZ.md instance grammar.
func ValidateInstance(id string) error {
	if id == "" {
		return fmt.Errorf("instance id is empty")
	}
	if !instanceRe.MatchString(id) {
		return fmt.Errorf("instance id %q must match ^[a-z][a-z0-9-]{0,30}$", id)
	}
	return nil
}

// ValidateProjectName requires a non-empty project identifier from a verified bundle.
func ValidateProjectName(name string) error {
	if strings.TrimSpace(name) == "" {
		return fmt.Errorf("project name is empty")
	}
	return nil
}

// ProviderSlug returns the deterministic Compose project name.
// iacp- + first 40 ASCII of "<project>-<instance>" + "-" + first 12 hex of SHA-256(project+NUL+instance).
func ProviderSlug(project, instance string) string {
	base := project + "-" + instance
	ascii := make([]byte, 0, len(base))
	for i := 0; i < len(base); i++ {
		c := base[i]
		if c <= 0x7f {
			ascii = append(ascii, c)
		}
	}
	prefix := string(ascii)
	if len(prefix) > 40 {
		prefix = prefix[:40]
	}
	sum := sha256.Sum256([]byte(project + "\x00" + instance))
	return "iacp-" + prefix + "-" + hex.EncodeToString(sum[:])[:12]
}
