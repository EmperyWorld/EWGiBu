package canonicaljson_test

import (
	"encoding/json"
	"testing"

	"iacpack/internal/canonicaljson"
)

func FuzzMarshal(f *testing.F) {
	f.Add([]byte(`{"b":1,"a":[true,null,"x"]}`))
	f.Add([]byte(`[]`))
	f.Add([]byte(`{"Z":1,"a":2}`))
	f.Fuzz(func(t *testing.T, raw []byte) {
		var v any
		if err := json.Unmarshal(raw, &v); err != nil {
			return
		}
		_, _ = canonicaljson.Marshal(v)
	})
}
