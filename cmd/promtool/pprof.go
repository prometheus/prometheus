package main

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"fmt"
	"net/http"
	"os"

	"github.com/google/pprof/profile"
)

var profNames = []string{
	"block",
	"goroutine",
	"heap",
	"mutex",
	"threadcreate",
}

var debugPprofProfile DebugPprofProfile

type DebugPprofProfile struct {
	Request *http.Request
}

func initDebugPprofProfile(path string) {
	req, err := http.NewRequest(http.MethodGet, promClient.Server.String()+"/debug/pprof/"+path, nil)
	if err != nil {
		panic(err)
	}
	debugPprofProfile = DebugPprofProfile{
		Request: req,
	}
}

func (c *DebugPprofProfile) Get() (*profile.Profile, error) {
	_, body, err := promClient.Do(c.Request)
	if err != nil {
		return nil, err
	}
	p, err := profile.Parse(bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	return p, nil
}

func DebugPprof() int {
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
		initDebugPprofProfile(profName)
		p, err := debugPprofProfile.Get()
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
