package orchestrator

import "testing"

func TestPickScale(t *testing.T) {
	tests := []struct {
		phase, policy, want int
	}{
		{0, 64, 64},
		{4, 64, 4},
		{128, 64, 64},
		{8, 0, 8},
		{0, 0, 0},
	}
	for _, tc := range tests {
		if got := PickScale(tc.phase, tc.policy); got != tc.want {
			t.Fatalf("PickScale(%d,%d)=%d want %d", tc.phase, tc.policy, got, tc.want)
		}
	}
}

func TestPerPhaseBudget(t *testing.T) {
	tests := []struct {
		total, phases, want int
	}{
		{64, 1, 64},
		{64, 3, 21},
		{2, 4, 1},
		{0, 3, 0},
	}
	for _, tc := range tests {
		if got := PerPhaseBudget(tc.total, tc.phases); got != tc.want {
			t.Fatalf("PerPhaseBudget(%d,%d)=%d want %d", tc.total, tc.phases, got, tc.want)
		}
	}
}
