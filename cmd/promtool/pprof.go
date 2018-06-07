package main

import (
	"bytes"
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
	tw := NewTarGzFileFileWriter(TarGzFileWriterConfig{FileName: "debug.tar.gz"})
	defer func() {
		if err := tw.Close(); err != nil {
			panic(err)
		}
	}()
	for _, profName := range profNames {
		body, err := NewBodyGetter(BodyGetterConfig{Path: "/debug/pprof/" + profName}).Get()
		if err != nil {
			fmt.Fprintln(os.Stderr, "error creating API client:", err)
		}

		p := validate(body)
		buf := buffer(p)

		if err := tw.AddFile(profName+".pb", buf); err != nil {
			fmt.Fprintln(os.Stderr, err)
		}
		fmt.Println(p.String())
	}

	return 0
}
