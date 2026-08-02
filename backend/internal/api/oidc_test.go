package api

import "testing"

func TestClaimTruthy(t *testing.T) {
	cases := []struct {
		in   any
		want bool
	}{
		{true, true},
		{false, false},
		{"true", true},
		{"1", true},
		{"yes", true},
		{"on", true},
		{"TRUE", true},
		{"false", false},
		{"0", false},
		{"", false},
		{1.0, true},
		{0.0, false},
		{[]any{"a", "true"}, true},
		{[]any{"a", "b"}, false},
		{nil, false},
	}
	for _, c := range cases {
		if got := claimTruthy(c.in); got != c.want {
			t.Errorf("claimTruthy(%v) = %v, want %v", c.in, got, c.want)
		}
	}
}
