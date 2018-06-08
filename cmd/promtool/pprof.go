package main

import (
	"bytes"
	"fmt"
	"net/http"
	"os"
	"path"

	"github.com/google/pprof/profile"
)

const PPROF_PATH = "/debug/pprof"

type DebugPprofConfig struct {
	server       string
	tarballName  string
	profileNames []string
	fileExt      string
}

type DebugPprof struct {
	writer           *TarGzFileWriter
	httpClient       *PrometheusHttpClient
	profileToRequest map[string]*http.Request
	fileExt          string
}

func NewDebugPprof(cfg DebugPprofConfig) *DebugPprof {
	client, err := NewPrometheusHttpClient(PrometheusHttpClientConfig{ServerURL: cfg.server})
	if err != nil {
		panic(err)
	}
	tw := NewTarGzFileFileWriter(TarGzFileWriterConfig{FileName: cfg.tarballName})
	m := make(map[string]*http.Request)
	for _, prof := range cfg.profileNames {
		req, err := http.NewRequest(http.MethodGet, client.HTTPClient.URL(path.Join(PPROF_PATH, prof), nil).String(), nil)
		if err != nil {
			panic(err)
		}
		m[prof] = req
	}
	return &DebugPprof{
		writer:           tw,
		httpClient:       client,
		profileToRequest: m,
		fileExt:          cfg.fileExt,
	}
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

func (c *DebugPprof) Exec() int {
	for prof, req := range c.profileToRequest {
		_, body, err := c.httpClient.Do(req)
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			return 1
		}

		p := validate(body)
		buf := buffer(p)

		if err := c.writer.AddFile(prof+c.fileExt, buf); err != nil {
			fmt.Fprintln(os.Stderr, err)
			return 1
		}
		fmt.Println(p.String())
	}

	if err := c.writer.Close(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	return 0
}
