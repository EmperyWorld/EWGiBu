package importpropose

import "testing"

func TestParseComposeMemoryBytes(t *testing.T) {
	got, err := parseComposeMemoryBytes("1536M")
	if err != nil {
		t.Fatal(err)
	}
	if got != 1536*(1<<20) {
		t.Fatalf("got %d", got)
	}
}
