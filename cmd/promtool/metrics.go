package main

import (
	"bytes"
	"fmt"
	"net/http"
	"os"
)

type DebugMetricsConfig struct {
	server      string
	tarballName string
	path        string
	fileName    string
}

type DebugMetrics struct {
	writer     ArchiveWriter
	httpClient HTTPClient
	request    *http.Request
	fileName   string
}

func NewDebugMetrics(cfg DebugMetricsConfig) *DebugMetrics {
	client, err := NewHTTPClient(HTTPClientConfig{ServerURL: cfg.server})
	if err != nil {
		panic(err)
	}
	tw := NewArchiveWriter(ArchiveWriterConfig{ArchiveName: cfg.tarballName})
	req, err := http.NewRequest(http.MethodGet, client.URLJoin(cfg.path), nil)
	if err != nil {
		panic(err)
	}
	return &DebugMetrics{
		writer:     tw,
		httpClient: client,
		request:    req,
		fileName:   cfg.fileName,
	}
}

func (c *DebugMetrics) Exec() int {
	_, body, err := c.httpClient.Do(c.request)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}

	var buf bytes.Buffer
	buf.Write(body)
	if err := c.writer.Write(c.fileName, buf); err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	fmt.Println(buf.String())

	if err := c.writer.Close(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	return 0
}
