package glob_test

import (
	"testing"

	"iacpack/internal/glob"
)

func TestMatch(t *testing.T) {
	cases := []struct {
		pat, name string
		want      bool
	}{
		{"**", "a/b/c", true},
		{"*.yaml", "app.yaml", true},
		{"*.yaml", "dir/app.yaml", false},
		{"**/*.yaml", "dir/app.yaml", true},
		{"a/**/c", "a/b/c", true},
		{"a/**/c", "a/c", true},
		{"a/*/c", "a/b/c", true},
		{"a/*/c", "a/b/x/c", false},
		{"file.?", "file.c", true},
		{".git/**", ".git/config", true},
	}
	for _, tc := range cases {
		got, err := glob.Match(tc.pat, tc.name)
		if err != nil {
			t.Fatalf("%s vs %s: %v", tc.pat, tc.name, err)
		}
		if got != tc.want {
			t.Fatalf("%s vs %s: got %v want %v", tc.pat, tc.name, got, tc.want)
		}
	}
}

func TestReaches(t *testing.T) {
	cases := []struct {
		pat, dir string
		want     bool
	}{
		{"nginx.prod.conf", "node_modules", false},
		{"*", "node_modules", false},
		{"**", "node_modules", true},
		{"foo/**", "foo", true},
		{"foo/**", "bar", false},
		{"foo/*.conf", "foo/bar", false},
		{"**/*.yaml", "node_modules", true},
	}
	for _, tc := range cases {
		got, err := glob.Reaches(tc.pat, tc.dir)
		if err != nil {
			t.Fatalf("%s under %s: %v", tc.pat, tc.dir, err)
		}
		if got != tc.want {
			t.Fatalf("%s under %s: got %v want %v", tc.pat, tc.dir, got, tc.want)
		}
	}
}

func TestRejectUnsupported(t *testing.T) {
	if _, err := glob.Match("a[b]", "ab"); err == nil {
		t.Fatal("expected error")
	}
	if _, err := glob.Match("a**b", "ab"); err == nil {
		t.Fatal("expected error for inline **")
	}
}
