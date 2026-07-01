package orchestrator

import (
	"strings"
	"time"

	"github.com/kjsst/sh-mvdos/internal/redisbus"
)

func l7ModeCMSHint(l7Mode string) string {
	switch strings.ToLower(strings.TrimSpace(l7Mode)) {
	case "catalog-search", "magento-search", "magento-cart":
		return "Magento"
	case "shopify-search":
		return "Shopify"
	case "wordpress", "wp-search":
		return "WordPress"
	case "drupal-search":
		return "Drupal"
	case "joomla-search":
		return "Joomla"
	case "woocommerce-search":
		return "WooCommerce"
	case "next-image":
		return "Next.js"
	default:
		return ""
	}
}

// PickScale treats the dashboard/policy value as the run ceiling.
func PickScale(phaseVal, policyVal int) int {
	if policyVal <= 0 {
		return phaseVal
	}
	if phaseVal <= 0 {
		return policyVal
	}
	if phaseVal < policyVal {
		return phaseVal
	}
	return policyVal
}

// PerPhaseBudget splits a run budget across concurrent phases so combos do not
// duplicate the full dashboard scale into each phase.
func PerPhaseBudget(total, phaseCount int) int {
	if total <= 0 || phaseCount <= 1 {
		return total
	}
	perPhase := total / phaseCount
	if perPhase < 1 {
		return 1
	}
	return perPhase
}

func copyPhaseParams(in map[string]string) map[string]string {
	if len(in) == 0 {
		return nil
	}
	out := make(map[string]string, len(in))
	for k, v := range in {
		out[k] = v
	}
	return out
}

// RequiredVectors returns unique vector ids referenced by phases.
func RequiredVectors(phases []Phase) []string {
	seen := make(map[string]struct{}, len(phases))
	var out []string
	for _, ph := range phases {
		if ph.Vector == "" {
			continue
		}
		if _, ok := seen[ph.Vector]; ok {
			continue
		}
		seen[ph.Vector] = struct{}{}
		out = append(out, ph.Vector)
	}
	return out
}

// BuildPhaseEvent maps a scheduled phase to a Redis event using policy/run scale.
func BuildPhaseEvent(ph Phase, runID, target string, workers, streams, batch int, base, expiresAt time.Time, l7Mode, proxyFile, wsPath string) redisbus.PhaseEvent {
	params := copyPhaseParams(ph.Params)
	if proxyFile != "" {
		if params == nil {
			params = map[string]string{}
		}
		if params["proxy_file"] == "" {
			params["proxy_file"] = proxyFile
		}
	}
	mode := strings.ToLower(strings.TrimSpace(l7Mode))
	if ph.Vector == "l7-abuser" && mode != "" && !IsStandaloneAttackMode(mode) {
		if params == nil {
			params = map[string]string{}
		}
		if params["mode"] == "" || params["mode"] == "baseline" {
			params["mode"] = mode
		}
		if params["cms"] == "" {
			if cms := l7ModeCMSHint(mode); cms != "" {
				params["cms"] = cms
			}
		}
	}
	if ph.Vector == "ws-flood" && strings.TrimSpace(wsPath) != "" {
		if params == nil {
			params = map[string]string{}
		}
		if params["ws_path"] == "" {
			params["ws_path"] = strings.TrimSpace(wsPath)
		}
	}
	delay := time.Duration(ph.AtSec) * time.Second
	return redisbus.PhaseEvent{
		RunID:     runID,
		PhaseID:   ph.ID,
		Vector:    ph.Vector,
		TargetURL: target,
		Workers:   PickScale(ph.Workers, workers),
		Streams:   PickScale(ph.Streams, streams),
		BatchSize: PickScale(ph.Batch, batch),
		Params:    params,
		At:        base.Add(delay),
		ExpiresAt: expiresAt,
	}
}
