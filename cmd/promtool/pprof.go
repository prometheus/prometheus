package main

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"fmt"
	"io"
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
		if err := p.WriteUncompressed(fo); err != nil {
			panic(err)
		}
		if err := fo.Close(); err != nil {
			panic(err)
		}
		fi, err := os.Open(profFileName)
		if err != nil {
			panic(err)
		}
		defer fi.Close()
		stat, err := fi.Stat()
		if err != nil {
			panic(err)
		}
		header := &tar.Header{
			Name: profFileName,
			Mode: int64(stat.Mode()),
			Size: stat.Size(),
		}
		if err := tw.WriteHeader(header); err != nil {
			panic(err)
		}
		// copy the file data to the tarball
		if _, err := io.Copy(tw, fi); err != nil {
			panic(err)
		}
		if err := os.Remove(profName + ".pb"); err != nil {
			panic(err)
		}
		fmt.Println(p.String())
	}

	return 0
}
