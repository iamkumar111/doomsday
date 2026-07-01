package worker

import (
	"crypto/tls"
	"net"
	"net/http"
	"time"
)

const (
	dialTimeout           = 5 * time.Second
	responseHeaderTimeout = 10 * time.Second
	clientTimeout         = 15 * time.Second
	keepAliveInterval     = 30 * time.Second
	idleConnTimeout       = 90 * time.Second
)

func newLabTransport() *http.Transport {
	return &http.Transport{
		DialContext: (&net.Dialer{
			Timeout:   dialTimeout,
			KeepAlive: keepAliveInterval,
		}).DialContext,
		TLSClientConfig:       &tls.Config{InsecureSkipVerify: true, MinVersion: tls.VersionTLS12},
		TLSHandshakeTimeout:   dialTimeout,
		MaxIdleConns:          4096,
		MaxIdleConnsPerHost:   2048,
		MaxConnsPerHost:       0,
		IdleConnTimeout:       idleConnTimeout,
		ResponseHeaderTimeout: responseHeaderTimeout,
		DisableCompression:    true,
		ForceAttemptHTTP2:     false,
	}
}

func newLabClient() *http.Client {
	return &http.Client{
		Transport: newLabTransport(),
		Timeout:   clientTimeout,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if len(via) >= 3 {
				return http.ErrUseLastResponse
			}
			return nil
		},
	}
}