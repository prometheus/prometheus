package main

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"time"

	"github.com/prometheus/client_golang/api"
)

func DebugPprof(url *url.URL) int {
	config := api.Config{
		Address: url.String(),
	}

	c, err := api.NewClient(config)
	if err != nil {
		fmt.Fprintln(os.Stderr, "error creating API client:", err)
		return 1
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	req, err := http.NewRequest(http.MethodGet, url.String()+"/debug/pprof", nil)
	if err != nil {
		fmt.Fprintln(os.Stderr, "debug pprof error:", err)
		return 1
	}

	_, body, err := c.Do(ctx, req)
	if err != nil {
		fmt.Fprintln(os.Stderr, "debug pprof error:", err)
		return 1
	}
	cancel()
	if err != nil {
		fmt.Fprintln(os.Stderr, "debug pprof error:", err)
		return 1
	}

	fmt.Println(string(body))
	return 0
}
