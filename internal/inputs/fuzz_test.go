package inputs_test

import (
	"testing"

	"iacpack/internal/inputs"
)

func FuzzValidateRelPath(f *testing.F) {
	f.Add("config/app.yaml")
	f.Add("../etc/passwd")
	f.Add("a\\b")
	f.Add("/abs")
	f.Fuzz(func(t *testing.T, p string) {
		_ = inputs.ValidateRelPath(p)
	})
}
