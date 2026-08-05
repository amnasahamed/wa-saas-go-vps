// Command healthcheck is a minimal HTTP probe for container healthchecks.
//
// The api and bridge run on gcr.io/distroless/static, which has no shell, curl,
// or wget, so the usual `CMD-SHELL curl -f ...` healthcheck cannot work. This
// binary ships alongside them and gives Docker something to exec.
//
// Usage:
//
//	healthcheck http://127.0.0.1:8080/healthz
//
// Exits 0 when the endpoint returns 2xx, 1 otherwise.
package main

import (
	"fmt"
	"net/http"
	"os"
	"time"
)

func main() {
	if len(os.Args) != 2 {
		fmt.Fprintln(os.Stderr, "usage: healthcheck <url>")
		os.Exit(2)
	}

	client := &http.Client{Timeout: 3 * time.Second}
	resp, err := client.Get(os.Args[1])
	if err != nil {
		fmt.Fprintln(os.Stderr, "healthcheck:", err)
		os.Exit(1)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		fmt.Fprintf(os.Stderr, "healthcheck: status %d\n", resp.StatusCode)
		os.Exit(1)
	}
}
