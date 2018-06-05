package main

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
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

func GetPprofProfile(url string, name string) (*profile.Profile, error) {
	config := api.Config{
		Address: url,
	}

	c, err := api.NewClient(config)
	if err != nil {
		return nil, err
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	req, err := http.NewRequest(http.MethodGet, url+"/debug/pprof/"+name, nil)
	if err != nil {
		return nil, err
	}

	_, body, err := c.Do(ctx, req)
	if err != nil {
		fmt.Fprintln(os.Stderr, "debug pprof error:", err)
		return nil, err
	}
	cancel()

	p, err := profile.Parse(bytes.NewReader(body))
	if err != nil {
		panic(err)
	}
	return p, nil
}

func DebugPprof(url *url.URL) int {
	tarfile, err := os.Create("debug.tar.gz")
	if err != nil {
		panic(err)
	}
	// close fo on exit and check for its returned error
	defer func() {
		if err := tarfile.Close(); err != nil {
			panic(err)
		}
	}()
	gw := gzip.NewWriter(tarfile)
	defer gw.Close()
	tw := tar.NewWriter(gw)
	defer tw.Close()

	for _, profName := range profNames {
		p, err := GetPprofProfile(url.String(), profName)
		if err != nil {
			fmt.Fprintln(os.Stderr, "error creating API client:", err)
		}
		var buf bytes.Buffer
		if err := p.WriteUncompressed(&buf); err != nil {
			panic(err)
		}

		header := &tar.Header{
			Name: profName + ".pb",
			Mode: 0644,
			Size: int64(buf.Len()),
		}
		if err := tw.WriteHeader(header); err != nil {
			panic(err)
		}
		if _, err := tw.Write(buf.Bytes()); err != nil {
			panic(err)
		}
		fmt.Println(p.String())
	}

	return 0
}
