package redisbus

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"
)

const (
	ChannelPhase   = "shmv:phase"
	ChannelStop    = "shmv:stop"
	ChannelMetrics = "shmv:metrics"
)

type Client struct {
	rdb *redis.Client
}

func New(addr string) *Client {
	return &Client{rdb: redis.NewClient(&redis.Options{Addr: addr})}
}

func (c *Client) Ping(ctx context.Context) error {
	return c.rdb.Ping(ctx).Err()
}

func (c *Client) Publish(ctx context.Context, channel string, payload any) error {
	raw, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	return c.rdb.Publish(ctx, channel, raw).Err()
}

func (c *Client) Subscribe(ctx context.Context, channels ...string) *redis.PubSub {
	return c.rdb.Subscribe(ctx, channels...)
}

type PhaseEvent struct {
	RunID     string            `json:"run_id,omitempty"`
	PhaseID   string            `json:"phase_id"`
	Vector    string            `json:"vector"`
	TargetURL string            `json:"target_url"`
	Workers   int               `json:"workers"`
	Streams   int               `json:"streams"`
	BatchSize int               `json:"batch_size"`
	Params    map[string]string `json:"params,omitempty"`
	At        time.Time         `json:"at"`
}

type MetricsEvent struct {
	RunID     string  `json:"run_id,omitempty"`
	Vector    string  `json:"vector"`
	Requests  uint64  `json:"requests"`
	Errors    uint64  `json:"errors"`
	RPS       float64 `json:"rps"`
	Timestamp int64   `json:"timestamp"`
}

type StopEvent struct {
	RunID  string `json:"run_id,omitempty"`
	Reason string `json:"reason,omitempty"`
}

// MetricsFromPhase builds a metrics event tied to the originating run.
func MetricsFromPhase(ev PhaseEvent, reqs, errs uint64, rps float64) MetricsEvent {
	return MetricsEvent{
		RunID:     ev.RunID,
		Vector:    ev.Vector,
		Requests:  reqs,
		Errors:    errs,
		RPS:       rps,
		Timestamp: time.Now().Unix(),
	}
}

func Decode[T any](payload string) (T, error) {
	var out T
	if err := json.Unmarshal([]byte(payload), &out); err != nil {
		return out, fmt.Errorf("redisbus: decode: %w", err)
	}
	return out, nil
}
