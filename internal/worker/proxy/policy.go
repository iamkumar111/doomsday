package proxy

import (
	"os"
	"strings"

	"github.com/kjsst/sh-mvdos/internal/labpolicy"
)

// ResolveFile returns the proxy file for a worker run (phase → env → policy).
func ResolveFile(policyPath, phaseProxy, envProxy string) string {
	if p := strings.TrimSpace(phaseProxy); p != "" {
		return p
	}
	if p := strings.TrimSpace(envProxy); p != "" {
		return p
	}
	if p := strings.TrimSpace(os.Getenv("PROXY_FILE")); p != "" {
		return p
	}
	if policyPath != "" {
		if pol, err := labpolicy.Load(policyPath); err == nil {
			return strings.TrimSpace(pol.ProxyFile)
		}
	}
	return ""
}