package proxy

import (
	"bufio"
	"context"
	"encoding/base64"
	"fmt"
	"math/rand"
	"net"
	"net/http"
	"net/url"
	"strings"
)

// DialTCP connects to addr directly or via HTTP CONNECT when proxies are set.
func DialTCP(ctx context.Context, addr string, proxies []string) (net.Conn, error) {
	if len(proxies) == 0 {
		d := net.Dialer{Timeout: DialTimeout}
		return d.DialContext(ctx, "tcp", addr)
	}
	raw := proxies[rand.Intn(len(proxies))]
	pURL, err := url.Parse(raw)
	if err != nil {
		return nil, err
	}
	proxyAddr := pURL.Host
	if _, _, err := net.SplitHostPort(proxyAddr); err != nil {
		proxyAddr = net.JoinHostPort(proxyAddr, "80")
	}
	d := net.Dialer{Timeout: DialTimeout}
	rawConn, err := d.DialContext(ctx, "tcp", proxyAddr)
	if err != nil {
		return nil, err
	}
	connectReq := "CONNECT " + addr + " HTTP/1.1\r\nHost: " + addr + "\r\n"
	if pURL.User != nil {
		user := pURL.User.Username()
		pass, _ := pURL.User.Password()
		cred := base64.StdEncoding.EncodeToString([]byte(user + ":" + pass))
		connectReq += "Proxy-Authorization: Basic " + cred + "\r\n"
	}
	connectReq += "\r\n"
	if _, err := rawConn.Write([]byte(connectReq)); err != nil {
		rawConn.Close()
		return nil, err
	}
	br := bufio.NewReader(rawConn)
	resp, err := http.ReadResponse(br, nil)
	if err != nil {
		rawConn.Close()
		return nil, fmt.Errorf("CONNECT failed: %w", err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		rawConn.Close()
		return nil, fmt.Errorf("CONNECT returned %d", resp.StatusCode)
	}
	return rawConn, nil
}

// LoadOrEmpty returns proxy URLs or nil when file is empty/missing.
func LoadOrEmpty(path string) ([]string, error) {
	path = strings.TrimSpace(path)
	if path == "" {
		return nil, nil
	}
	return LoadProxies(path)
}

// EffectivePath picks the first non-empty proxy file path.
func EffectivePath(values ...string) string {
	for _, v := range values {
		if v = strings.TrimSpace(v); v != "" {
			return v
		}
	}
	return ""
}