package redisbus

import "testing"

func TestMatchesRun(t *testing.T) {
	tests := []struct {
		event, active string
		want          bool
	}{
		{"run-1", "run-1", true},
		{"run-1", "run-2", false},
		{"", "run-1", false},
		{"run-1", "", false},
		{"", "", false},
	}
	for _, tc := range tests {
		if got := MatchesRun(tc.event, tc.active); got != tc.want {
			t.Fatalf("MatchesRun(%q, %q) = %v, want %v", tc.event, tc.active, got, tc.want)
		}
	}
}