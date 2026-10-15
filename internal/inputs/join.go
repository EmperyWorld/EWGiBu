package inputs

import (
	"fmt"
	"strings"
)

// JoinRel joins base and elem as slash-separated relative paths.
func JoinRel(base, elem string) (string, error) {
	elem = strings.TrimPrefix(elem, "./")
	if elem == "" {
		return "", fmt.Errorf("empty path element")
	}
	if err := ValidateRelPath(elem); err != nil {
		return "", err
	}
	if base == "" || base == "." {
		return elem, nil
	}
	if err := ValidateRelPath(base); err != nil {
		return "", err
	}
	joined := base + "/" + elem
	if err := ValidateRelPath(joined); err != nil {
		return "", err
	}
	return joined, nil
}
