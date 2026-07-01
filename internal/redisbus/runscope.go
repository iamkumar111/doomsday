package redisbus

import "encoding/json"

// MatchesRun reports whether an event belongs to the active run.
// Run scoping is authoritative: empty event IDs never match.
func MatchesRun(eventRunID, activeRunID string) bool {
	return activeRunID != "" && eventRunID == activeRunID
}

// DecodeStop parses a stop channel payload.
func DecodeStop(payload string) (StopEvent, error) {
	var ev StopEvent
	if err := json.Unmarshal([]byte(payload), &ev); err != nil {
		return StopEvent{Reason: payload}, err
	}
	return ev, nil
}