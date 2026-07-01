package orchestrator

import "testing"

func TestSelectAttackPhasesStandalone(t *testing.T) {
	all := []Phase{
		{ID: "l7-get", Vector: "l7-abuser"},
		{ID: "h2-rapid", Vector: "h2-rapid-reset"},
		{ID: "ws-stress", Vector: "ws-flood"},
	}
	combo := Combo{ID: "slayer-mix", Phases: []string{"l7-get", "h2-rapid", "ws-stress"}}

	got := SelectAttackPhases(all, combo, "h2-rapid-reset")
	if len(got) != 1 || got[0].Vector != "h2-rapid-reset" {
		t.Fatalf("expected single h2 phase, got %+v", got)
	}

	got = SelectAttackPhases(all, combo, "ws-flood")
	if len(got) != 1 || got[0].Vector != "ws-flood" {
		t.Fatalf("expected single ws phase, got %+v", got)
	}

	got = SelectAttackPhases(all, combo, "httpget")
	if len(got) != 3 {
		t.Fatalf("expected combo phases for l7 override, got %d", len(got))
	}
}