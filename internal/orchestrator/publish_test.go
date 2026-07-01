package orchestrator

import "testing"

func TestPickScale(t *testing.T) {
	tests := []struct {
		phase, policy, want int
	}{
		{0, 64, 64},
		{4, 64, 64},
		{128, 64, 128},
		{8, 0, 8},
		{0, 0, 0},
	}
	for _, tc := range tests {
		if got := PickScale(tc.phase, tc.policy); got != tc.want {
			t.Fatalf("PickScale(%d,%d)=%d want %d", tc.phase, tc.policy, got, tc.want)
		}
	}
}