package v1alpha1

import "testing"

func TestBindsOverlap(t *testing.T) {
	cases := []struct {
		a, b string
		want bool
	}{
		{"0.0.0.0", "127.0.0.1", true},
		{"127.0.0.1", "0.0.0.0", true},
		{"0.0.0.0", "::1", false}, // IPv4 wildcard does not overlap pure IPv6
		{"::", "127.0.0.1", true},
		{"::", "::1", true},
		{"::0", "127.0.0.1", true}, // equivalent IPv6 unspecified
		{"127.0.0.1", "127.0.0.1", true},
		{"127.0.0.1", "::ffff:127.0.0.1", true},
		{"127.0.0.1", "10.0.0.1", false},
		{"127.0.0.1", "::1", false},
	}
	for _, tc := range cases {
		got := bindsOverlap(tc.a, tc.b)
		if got != tc.want {
			t.Fatalf("bindsOverlap(%q,%q)=%v want %v", tc.a, tc.b, got, tc.want)
		}
		gotRev := bindsOverlap(tc.b, tc.a)
		if gotRev != tc.want {
			t.Fatalf("bindsOverlap(%q,%q)=%v want %v", tc.b, tc.a, gotRev, tc.want)
		}
	}
}
