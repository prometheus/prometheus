package main

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"fmt"
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

func validate(b []byte) *profile.Profile {
	p, err := profile.Parse(bytes.NewReader(b))
	if err != nil {
		panic(err)
	}
	return p
}

func buffer(p *profile.Profile) bytes.Buffer {
	var buf bytes.Buffer
	if err := p.WriteUncompressed(&buf); err != nil {
		panic(err)
	}
	return buf
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
		body, err := NewBodyGetter(BodyGetterConfig{Path: "/debug/pprof/" + profName}).Get()
		if err != nil {
			fmt.Fprintln(os.Stderr, "error creating API client:", err)
		}

		p := validate(body)
		buf := buffer(p)

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
