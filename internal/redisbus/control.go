package redisbus

import (
	"context"
	"encoding/json"
	"fmt"
	"time"
)

const (
	activeRunsKey = "shmv:active_runs"
	workerKeyFmt  = "shmv:worker:%s"
	runPhasesFmt  = "shmv:run:%s:phases"
	runMetaFmt    = "shmv:run:%s:meta"
	runStoppedFmt = "shmv:run:%s:stopped"
	workerTTL     = 45 * time.Second
)

type WorkerHeartbeat struct {
	Vector string `json:"vector"`
	Host   string `json:"host"`
	TS     int64  `json:"ts"`
}

// PublishPhase publishes to Pub/Sub and records the event for offline worker replay.
func (c *Client) PublishPhase(ctx context.Context, ev PhaseEvent) error {
	raw, err := json.Marshal(ev)
	if err != nil {
		return err
	}
	runKey := fmt.Sprintf(runPhasesFmt, ev.RunID)
	metaKey := fmt.Sprintf(runMetaFmt, ev.RunID)

	pipe := c.rdb.Pipeline()
	pipe.Publish(ctx, ChannelPhase, raw)
	pipe.RPush(ctx, runKey, raw)
	pipe.SAdd(ctx, activeRunsKey, ev.RunID)
	if !ev.ExpiresAt.IsZero() {
		pipe.HSet(ctx, metaKey, map[string]interface{}{
			"expires_at": ev.ExpiresAt.Unix(),
			"target":     ev.TargetURL,
		})
		ttl := time.Until(ev.ExpiresAt) + time.Hour
		if ttl > 0 {
			pipe.Expire(ctx, runKey, ttl)
			pipe.Expire(ctx, metaKey, ttl)
		}
	}
	_, err = pipe.Exec(ctx)
	return err
}

// PublishStop publishes stop and marks the run stopped in durable storage.
func (c *Client) PublishStop(ctx context.Context, ev StopEvent) error {
	raw, err := json.Marshal(ev)
	if err != nil {
		return err
	}
	stoppedKey := fmt.Sprintf(runStoppedFmt, ev.RunID)
	pipe := c.rdb.Pipeline()
	pipe.Publish(ctx, ChannelStop, raw)
	if ev.RunID != "" {
		reason := ev.Reason
		if reason == "" {
			reason = "stop"
		}
		pipe.Set(ctx, stoppedKey, reason, 24*time.Hour)
		pipe.SRem(ctx, activeRunsKey, ev.RunID)
	}
	_, err = pipe.Exec(ctx)
	return err
}

func (c *Client) IsRunStopped(ctx context.Context, runID string) bool {
	if runID == "" {
		return false
	}
	n, err := c.rdb.Exists(ctx, fmt.Sprintf(runStoppedFmt, runID)).Result()
	return err == nil && n > 0
}

func (c *Client) runExpiresAt(ctx context.Context, runID string) (time.Time, bool) {
	val, err := c.rdb.HGet(ctx, fmt.Sprintf(runMetaFmt, runID), "expires_at").Int64()
	if err != nil {
		return time.Time{}, false
	}
	return time.Unix(val, 0), true
}

// ReplayDuePhases invokes handle for past-due phases of active runs matching vector.
func (c *Client) ReplayDuePhases(ctx context.Context, vector string, handle func(PhaseEvent)) error {
	runIDs, err := c.rdb.SMembers(ctx, activeRunsKey).Result()
	if err != nil {
		return err
	}
	now := time.Now()
	for _, runID := range runIDs {
		if c.IsRunStopped(ctx, runID) {
			continue
		}
		if exp, ok := c.runExpiresAt(ctx, runID); ok && now.After(exp) {
			continue
		}
		raws, err := c.rdb.LRange(ctx, fmt.Sprintf(runPhasesFmt, runID), 0, -1).Result()
		if err != nil {
			continue
		}
		for _, raw := range raws {
			ev, err := Decode[PhaseEvent](raw)
			if err != nil || ev.Vector != vector {
				continue
			}
			if !ev.At.IsZero() && ev.At.After(now) {
				continue
			}
			handle(ev)
		}
	}
	return nil
}

// TouchWorker records a heartbeat for vector (TTL-based liveness).
func (c *Client) TouchWorker(ctx context.Context, hb WorkerHeartbeat) error {
	if hb.Vector == "" {
		return fmt.Errorf("redisbus: empty worker vector")
	}
	if hb.TS == 0 {
		hb.TS = time.Now().Unix()
	}
	raw, err := json.Marshal(hb)
	if err != nil {
		return err
	}
	key := fmt.Sprintf(workerKeyFmt, hb.Vector)
	return c.rdb.Set(ctx, key, raw, workerTTL).Err()
}

// WorkersReady returns vectors with no recent heartbeat.
func (c *Client) WorkersReady(ctx context.Context, vectors []string) (missing []string, err error) {
	for _, v := range vectors {
		if v == "" {
			continue
		}
		n, err := c.rdb.Exists(ctx, fmt.Sprintf(workerKeyFmt, v)).Result()
		if err != nil {
			return nil, err
		}
		if n == 0 {
			missing = append(missing, v)
		}
	}
	return missing, nil
}

// ListWorkers returns vectors that have sent a recent heartbeat.
func (c *Client) ListWorkers(ctx context.Context) ([]WorkerHeartbeat, error) {
	var out []WorkerHeartbeat
	iter := c.rdb.Scan(ctx, 0, "shmv:worker:*", 100).Iterator()
	for iter.Next(ctx) {
		raw, err := c.rdb.Get(ctx, iter.Val()).Result()
		if err != nil {
			continue
		}
		hb, err := Decode[WorkerHeartbeat](raw)
		if err == nil && hb.Vector != "" {
			out = append(out, hb)
		}
	}
	return out, iter.Err()
}