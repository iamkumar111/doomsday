package guard

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/url"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/kjsst/sh-mvdos/internal/labpolicy"
)

const EthicsAckValue = "I_OWN_THIS_LAB"

const (
	ExitUsage   = 2
	ExitRefused = 3
)

var ErrRefused = errors.New("guard: execution refused")

type Config struct {
	PolicyPath string
	TargetURL  string
	Vector     string
}

func MustAuthorize(cfg Config) (context.Context, error) {
	return authorize(cfg, true, true)
}

// MustAuthorizeControlPlane authorizes dashboard/conductor even when the policy
// target is not yet on allowed_hosts, so operators can fix the allowlist via UI.
// Unlike MustAuthorize, it does not apply MaxDurationSec as a process lifetime cap.
func MustAuthorizeControlPlane(cfg Config) (context.Context, error) {
	return authorize(cfg, false, false)
}

func authorize(cfg Config, requireTarget, applyRunDeadline bool) (context.Context, error) {
	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	policy, err := labpolicy.Load(cfg.PolicyPath)
	if err != nil {
		cancel()
		return nil, fmt.Errorf("%w: %v", ErrRefused, err)
	}
	if policy.EthicsAck != EthicsAckValue {
		cancel()
		return nil, fmt.Errorf("%w: set ethics_ack=%q in lab-policy", ErrRefused, EthicsAckValue)
	}
	if policy.LabMode != "isolated" {
		cancel()
		return nil, fmt.Errorf("%w: lab_mode must be isolated", ErrRefused)
	}
	target := cfg.TargetURL
	if target == "" {
		target = policy.TargetURL
	}
	if requireTarget {
		if err := validateTarget(target, policy); err != nil {
			cancel()
			return nil, err
		}
	} else if target != "" {
		if err := validateTarget(target, policy); err != nil {
			slog.Warn("guard: policy target not allowlisted yet; control plane starting anyway",
				"vector", cfg.Vector,
				"target", target,
				"err", err,
			)
		}
	}
	if applyRunDeadline && policy.MaxDurationSec > 0 {
		deadline := time.Now().Add(time.Duration(policy.MaxDurationSec) * time.Second)
		dctx, dcancel := context.WithDeadline(ctx, deadline)
		go func() {
			<-dctx.Done()
			dcancel()
		}()
		ctx = dctx
	}
	slog.Info("guard: authorized",
		"vector", cfg.Vector,
		"target", target,
		"run_deadline", applyRunDeadline,
		"max_duration_sec", policy.MaxDurationSec,
	)
	return ctx, nil
}

// ValidateTarget performs the scheme + allowlist checks used by MustAuthorize.
func ValidateTarget(raw string, policy *labpolicy.Policy) error {
	return validateTarget(raw, policy)
}

// MustValidatePolicyTarget loads the policy and performs full target authorization checks
// (ethics, lab_mode, scheme, allowlist). Returns ErrRefused wrapped error on failure.
func MustValidatePolicyTarget(policyPath, target string) error {
	p, err := labpolicy.Load(policyPath)
	if err != nil {
		return fmt.Errorf("%w: %v", ErrRefused, err)
	}
	if p.EthicsAck != EthicsAckValue {
		return fmt.Errorf("%w: set ethics_ack=%q in lab-policy", ErrRefused, EthicsAckValue)
	}
	if p.LabMode != "isolated" {
		return fmt.Errorf("%w: lab_mode must be isolated", ErrRefused)
	}
	if err := ValidateTarget(target, p); err != nil {
		return err
	}
	return nil
}

func validateTarget(raw string, policy *labpolicy.Policy) error {
	u, err := url.Parse(raw)
	if err != nil {
		return fmt.Errorf("%w: invalid target url: %v", ErrRefused, err)
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return fmt.Errorf("%w: target scheme must be http or https", ErrRefused)
	}
	host := strings.ToLower(u.Hostname())
	if host == "" {
		return fmt.Errorf("%w: empty target host", ErrRefused)
	}
	if !policy.IsHostAllowed(host) {
		return fmt.Errorf("%w: host %q not in allowed_hosts — add via dashboard allowlist", ErrRefused, host)
	}
	return nil
}

func ExitCode(err error) int {
	if err == nil {
		return 0
	}
	if errors.Is(err, ErrRefused) {
		return ExitRefused
	}
	return ExitUsage
}
