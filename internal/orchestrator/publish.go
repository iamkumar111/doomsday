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

// PickScale chooses the higher of phase and policy scale so policy intensity is not capped by YAML defaults.
func PickScale(phaseVal, policyVal int) int {
	if phaseVal <= 0 {
		return policyVal
	}
	if policyVal <= 0 {
		return phaseVal
	}
	if phaseVal > policyVal {
		return phaseVal
	}
	return policyVal
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

// BuildPhaseEvent maps a scheduled phase to a Redis event using policy/run scale.
func BuildPhaseEvent(ph Phase, runID, target string, workers, streams, batch int, base time.Time, l7Mode string) redisbus.PhaseEvent {
	params := copyPhaseParams(ph.Params)
	if ph.Vector == "l7-abuser" && l7Mode != "" {
		if params == nil {
			params = map[string]string{}
		}
		if params["mode"] == "" || params["mode"] == "baseline" {
			params["mode"] = l7Mode
		}
		if params["cms"] == "" {
			if cms := l7ModeCMSHint(l7Mode); cms != "" {
				params["cms"] = cms
			}
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
	}
}