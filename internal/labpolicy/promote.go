package labpolicy

import "strings"

// PromoteDraftToPolicy copies runnable fields from a recon draft into policy.
// Ethics, lab_mode, and allowed_hosts remain authoritative on policy.
func PromoteDraftToPolicy(policy *Policy, draft *ReconDraft) {
	if policy == nil || draft == nil {
		return
	}
	if draft.TargetURL != "" {
		policy.TargetURL = draft.TargetURL
	}
	if draft.Combo != "" {
		policy.Combo = draft.Combo
	}
	if draft.Workers > 0 {
		policy.Workers = draft.Workers
	}
	if draft.Streams > 0 {
		policy.Streams = draft.Streams
	}
	if draft.BatchSize > 0 {
		policy.BatchSize = draft.BatchSize
	}
	if draft.L7Mode != "" {
		policy.L7Mode = draft.L7Mode
	}
}

// DraftHostAllowlisted reports whether the draft target host is on the policy allowlist.
func DraftHostAllowlisted(policy *Policy, draft *ReconDraft) bool {
	if policy == nil || draft == nil {
		return false
	}
	host := strings.ToLower(strings.TrimSpace(draft.Host))
	if host == "" && draft.TargetURL != "" {
		scratch := &Policy{TargetURL: draft.TargetURL}
		if h, err := scratch.TargetHost(); err == nil {
			host = h
		}
	}
	if host == "" {
		return false
	}
	return policy.IsHostAllowed(host)
}