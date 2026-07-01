package proxy

import (
	"bufio"
	"crypto/tls"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"
)

const (
	DialTimeout           = 5 * time.Second
	ResponseHeaderTimeout = 10 * time.Second
	ClientTimeout         = 10 * time.Second
	KeepAliveInterval     = 30 * time.Second
	IdleConnTimeout       = 90 * time.Second
	MaxIdleConns          = 500
	MaxIdleConnsPerHost   = 250
	MaxConnsPerHost       = 250
	MaxClientsPerProxy    = 64
	MaxDirectPool         = 256
)

// LoadProxies reads one proxy URL per line (http://, https://, socks5://).
func LoadProxies(path string) ([]string, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("open proxy file: %w", err)
	}
	defer f.Close()

	var lines []string
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		if line := strings.TrimSpace(sc.Text()); line != "" && !strings.HasPrefix(line, "#") {
			lines = append(lines, line)
		}
	}
	if err := sc.Err(); err != nil {
		return nil, fmt.Errorf("read proxy file: %w", err)
	}
	if len(lines) == 0 {
		return nil, fmt.Errorf("no proxies found in %s", path)
	}
	return lines, nil
}

func NewTransport(proxyURL *url.URL) *http.Transport {
	t := &http.Transport{
		DialContext: (&net.Dialer{
			Timeout:   DialTimeout,
			KeepAlive: KeepAliveInterval,
		}).DialContext,
		TLSClientConfig:       &tls.Config{InsecureSkipVerify: true, MinVersion: tls.VersionTLS12},
		TLSHandshakeTimeout:   DialTimeout,
		MaxIdleConns:          MaxIdleConns,
		MaxIdleConnsPerHost:   MaxIdleConnsPerHost,
		MaxConnsPerHost:       MaxConnsPerHost,
		IdleConnTimeout:       IdleConnTimeout,
		ResponseHeaderTimeout: ResponseHeaderTimeout,
		DisableCompression:    true,
		ForceAttemptHTTP2:     false,
	}
	if proxyURL != nil {
		t.Proxy = http.ProxyURL(proxyURL)
	}
	return t
}

func newClient(transport *http.Transport) *http.Client {
	return &http.Client{
		Transport: transport,
		Timeout:   ClientTimeout,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if len(via) >= 3 {
				return http.ErrUseLastResponse
			}
			return nil
		},
	}
}

// BuildClientPool creates HTTP clients scaled per unique proxy (Slayer model).
func BuildClientPool(proxies []string, workers int) ([]*http.Client, error) {
	seen := make(map[string]bool)
	var unique []string
	for _, p := range proxies {
		if !seen[p] {
			seen[p] = true
			unique = append(unique, p)
		}
	}

	clientsPerProxy := workers / len(unique)
	if clientsPerProxy < 1 {
		clientsPerProxy = 1
	}
	if clientsPerProxy > MaxClientsPerProxy {
		clientsPerProxy = MaxClientsPerProxy
	}

	clients := make([]*http.Client, 0, len(unique)*clientsPerProxy)
	for _, raw := range unique {
		proxyURL, err := url.Parse(raw)
		if err != nil {
			continue
		}
		for i := 0; i < clientsPerProxy; i++ {
			clients = append(clients, newClient(NewTransport(proxyURL)))
		}
	}
	if len(clients) == 0 {
		return nil, fmt.Errorf("no valid proxy clients built")
	}
	return clients, nil
}

// BuildDirectPool creates a pool of direct (no-proxy) clients.
func BuildDirectPool(count int) []*http.Client {
	if count < 1 {
		count = 4
	}
	clients := make([]*http.Client, 0, count)
	for i := 0; i < count; i++ {
		clients = append(clients, newClient(NewTransport(nil)))
	}
	return clients
}

// ResolvePool returns a client pool for direct or proxied mode.
func ResolvePool(proxyFile string, workers int) ([]*http.Client, error) {
	proxyFile = strings.TrimSpace(proxyFile)
	if proxyFile == "" {
		poolSize := workers / 8
		if poolSize < 4 {
			poolSize = 4
		}
		if poolSize > MaxDirectPool {
			poolSize = MaxDirectPool
		}
		return BuildDirectPool(poolSize), nil
	}
	proxies, err := LoadProxies(proxyFile)
	if err != nil {
		return nil, err
	}
	return BuildClientPool(proxies, workers)
}

// Pick returns client for worker id using round-robin over pool.
func Pick(clients []*http.Client, workerID int) *http.Client {
	if len(clients) == 0 {
		return newClient(NewTransport(nil))
	}
	return clients[workerID%len(clients)]
}