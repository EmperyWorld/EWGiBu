package importpropose

import "testing"

func TestResourceStubResultMapForAPIEmptySlices(t *testing.T) {
	m := (ResourceStubResult{}).MapForAPI()
	for _, key := range []string{"createdDirs", "createdFiles", "createdBuildContexts", "skipped"} {
		v, ok := m[key]
		if !ok {
			t.Fatalf("missing key %q", key)
		}
		sl, ok := v.([]string)
		if !ok {
			t.Fatalf("%q type %T", key, v)
		}
		if sl == nil || len(sl) != 0 {
			t.Fatalf("%q = %#v", key, sl)
		}
	}
}
