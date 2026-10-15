package canonicaljson_test

import (
	"math"
	"strings"
	"testing"

	"iacpack/internal/canonicaljson"
)

func TestMarshalSortedKeysNoWhitespace(t *testing.T) {
	got, err := canonicaljson.Marshal(map[string]any{
		"b": 2,
		"a": map[string]any{"z": 1, "y": true},
	})
	if err != nil {
		t.Fatal(err)
	}
	want := `{"a":{"y":true,"z":1},"b":2}`
	if string(got) != want {
		t.Fatalf("got %s want %s", got, want)
	}
}

func TestMarshalRejectsNaN(t *testing.T) {
	nan := math.NaN()
	_, err := canonicaljson.Marshal(map[string]any{"x": nan})
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestMarshalScalarsAndArray(t *testing.T) {
	got, err := canonicaljson.Marshal([]any{nil, false, true, 1.5, "a"})
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != `[null,false,true,1.5,"a"]` {
		t.Fatalf("got %s", got)
	}
	if _, err := canonicaljson.Marshal(math.Inf(1)); err == nil {
		t.Fatal("expected Inf reject")
	}
}

func TestMarshalUTF16KeyOrder(t *testing.T) {
	got, err := canonicaljson.Marshal(map[string]any{"\u00e9": 1, "e\u0301": 2, "a": 0})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(got), `"a":0`) {
		t.Fatalf("got %s", got)
	}
}
