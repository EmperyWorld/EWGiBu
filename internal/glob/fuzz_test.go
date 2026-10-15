package glob_test

import (
	"testing"

	"iacpack/internal/glob"
)

func FuzzMatch(f *testing.F) {
	f.Add("**/*.yaml", "a/b.yaml")
	f.Add("*", "x")
	f.Add("a/**/c", "a/b/c")
	f.Fuzz(func(t *testing.T, pattern, name string) {
		_, _ = glob.Match(pattern, name)
	})
}
