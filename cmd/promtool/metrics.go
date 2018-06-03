package main

import (
	"context"
	"fmt"
	"io/ioutil"
	"net/http"
	"net/url"
	"os"
	"time"

	"github.com/prometheus/client_golang/api"
)

func DebugMetrics(url *url.URL) int {
	config := api.Config{
		Address: url.String(),
	}

	c, err := api.NewClient(config)
	if err != nil {
		fmt.Fprintln(os.Stderr, "error creating API client:", err)
		return 1
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	req, err := http.NewRequest(http.MethodGet, url.String()+"/metrics", nil)
	if err != nil {
		fmt.Fprintln(os.Stderr, "debug metrics error:", err)
		return 1
	}

	_, body, err := c.Do(ctx, req)
	if err != nil {
		fmt.Fprintln(os.Stderr, "debug metrics error:", err)
		return 1
	}
	cancel()

	if err := ioutil.WriteFile("metrics.txt", body, 0644); err != nil {
		panic(err)
	}
	fmt.Println(string(body))
	return 0
}
