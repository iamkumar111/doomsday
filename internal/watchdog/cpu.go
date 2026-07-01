package watchdog

import (
	"context"
	"log/slog"
	"os"
	"strconv"
	"strings"
	"time"
)

const clockTicksPerSec = 100.0

// Monitor cancels work when process CPU exceeds limitPercent (0 disables).
func Monitor(ctx context.Context, limitPercent int, onBreach func()) {
	if limitPercent <= 0 {
		return
	}
	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()

	var prevTicks uint64
	var prevAt time.Time

	for {
		select {
		case <-ctx.Done():
			return
		case now := <-ticker.C:
			ticks, err := procCPUTicks()
			if err != nil {
				continue
			}
			if prevAt.IsZero() {
				prevTicks, prevAt = ticks, now
				continue
			}
			elapsed := now.Sub(prevAt).Seconds()
			if elapsed <= 0 {
				continue
			}
			pct := (float64(ticks-prevTicks) / clockTicksPerSec / elapsed) * 100.0
			prevTicks, prevAt = ticks, now
			if pct > float64(limitPercent) {
				slog.Warn("watchdog: CPU limit breached", "pct", pct, "limit", limitPercent)
				if onBreach != nil {
					onBreach()
				}
				return
			}
		}
	}
}

func procCPUTicks() (uint64, error) {
	data, err := os.ReadFile("/proc/self/stat")
	if err != nil {
		return 0, err
	}
	fields := strings.Fields(string(data))
	if len(fields) < 15 {
		return 0, strconv.ErrSyntax
	}
	ut, err := strconv.ParseUint(fields[13], 10, 64)
	if err != nil {
		return 0, err
	}
	st, err := strconv.ParseUint(fields[14], 10, 64)
	if err != nil {
		return 0, err
	}
	return ut + st, nil
}