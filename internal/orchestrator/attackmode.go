package orchestrator

import "strings"

// Standalone attack modes map directly to a single phase/vector (not l7-abuser).
var standaloneAttackModes = map[string]Phase{
	"h2-rapid-reset": {ID: "h2-rapid", Vector: "h2-rapid-reset", AtSec: 0},
	"ws-flood":       {ID: "ws-stress", Vector: "ws-flood", AtSec: 0},
	"slowloris":      {ID: "slowloris-hold", Vector: "slowloris", AtSec: 0},
	"quic-burner":    {ID: "quic-stress", Vector: "quic-burner", AtSec: 0},
}

// IsStandaloneAttackMode reports whether mode selects a dedicated vector worker.
func IsStandaloneAttackMode(mode string) bool {
	_, ok := standaloneAttackModes[strings.ToLower(strings.TrimSpace(mode))]
	return ok
}

// SelectAttackPhases applies combo phases unless attack mode overrides with a standalone vector.
func SelectAttackPhases(all []Phase, combo Combo, attackMode string) []Phase {
	mode := strings.ToLower(strings.TrimSpace(attackMode))
	if ph, ok := standaloneAttackModes[mode]; ok {
		return []Phase{ph}
	}
	return SelectPhases(all, combo)
}

// L7AttackModes are modes executed by the l7-abuser worker.
func L7AttackModes() []string {
	return []string{
		"", "baseline", "httpget", "httppost", "apiflood", "rudy",
		"catalog-search", "wordpress", "shopify-search", "cms-rotate", "graphql",
	}
}