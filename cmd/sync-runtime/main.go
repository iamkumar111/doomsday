package main

import (
	"fmt"
	"os"
	"strings"
)

// sync-runtime copies key=value lines from data/runtime.env into stdout for lab scripts.
func main() {
	path := "data/runtime.env"
	if len(os.Args) > 1 {
		path = os.Args[1]
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		fmt.Fprintf(os.Stderr, "sync-runtime: %v\n", err)
		os.Exit(1)
	}
	for _, line := range strings.Split(string(raw), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		fmt.Println(line)
	}
}
