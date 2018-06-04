package main

import (
	"bytes"
	"context"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"time"

	"github.com/google/pprof/profile"
	"github.com/prometheus/client_golang/api"
)

var profNames = []string{
	"block",
	"goroutine",
	"heap",
	"mutex",
	"threadcreate",
}

func DebugPprof(url *url.URL) int {
	config := api.Config{
		Address: url.String(),
	}

	c, err := api.NewClient(config)
	if err != nil {
		fmt.Fprintln(os.Stderr, "error creating API client:", err)
		return 1
	}

	for _, profName := range profNames {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
		req, err := http.NewRequest(http.MethodGet, url.String()+"/debug/pprof/"+profName, nil)
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

		p, err := profile.Parse(bytes.NewReader(body))
		if err != nil {
			panic(err)
		}

		// open output file
		profFileName := profName + ".pb"
		fo, err := os.Create(profFileName)
		if err != nil {
			panic(err)
		}
		// close fo on exit and check for its returned error
		defer func() {
			if err := fo.Close(); err != nil {
				panic(err)
			}
		}()
		if err := p.WriteUncompressed(fo); err != nil {
			panic(err)
		}

		fmt.Println(p.String())
	}
	return 0
}
