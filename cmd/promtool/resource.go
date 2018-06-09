// Copyright 2015 The Prometheus Authors
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
// http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package main

import (
	"bytes"
	"fmt"
	"net/http"
	"os"

	"github.com/google/pprof/profile"
)

type ResourceSaverConfig struct {
	server         string
	tarballName    string
	pathToFileName map[string]string
	postProcess    func(b []byte) (*bytes.Buffer, error)
}

type ResourceSaver struct {
	writer            ArchiveWriter
	httpClient        HTTPClient
	fileNameToRequest map[string]*http.Request
	postProcess       func(b []byte) (*bytes.Buffer, error)
}

func NewResourceSaver(cfg ResourceSaverConfig) *ResourceSaver {
	client, err := NewHTTPClient(HTTPClientConfig{ServerURL: cfg.server})
	if err != nil {
		panic(err)
	}
	tw, err := NewArchiveWriter(ArchiveWriterConfig{ArchiveName: cfg.tarballName})
	if err != nil {
		panic(err)
	}
	m := make(map[string]*http.Request)
	for path, filename := range cfg.pathToFileName {
		req, err := http.NewRequest(http.MethodGet, client.URLJoin(path), nil)
		if err != nil {
			panic(err)
		}
		m[filename] = req
	}
	return &ResourceSaver{
		writer:            tw,
		httpClient:        client,
		fileNameToRequest: m,
		postProcess:       cfg.postProcess,
	}
}

func (c *ResourceSaver) Exec() int {
	for filename, req := range c.fileNameToRequest {
		_, body, err := c.httpClient.Do(req)
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			return 1
		}

		buf, err := c.postProcess(body)
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			return 1
		}

		if err := c.writer.Write(filename, buf); err != nil {
			fmt.Fprintln(os.Stderr, err)
			return 1
		}
	}

	if err := c.writer.Close(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	return 0
}

func validate(b []byte) (*profile.Profile, error) {
	p, err := profile.Parse(bytes.NewReader(b))
	if err != nil {
		return nil, err
	}
	return p, nil
}

func buffer(p *profile.Profile) (*bytes.Buffer, error) {
	buf := new(bytes.Buffer)
	if err := p.WriteUncompressed(buf); err != nil {
		return nil, err
	}
	return buf, nil
}

var pprofPostProcess = func(b []byte) (*bytes.Buffer, error) {
	p, err := validate(b)
	if err != nil {
		return nil, err
	}
	fmt.Println(p.String())
	return buffer(p)
}

var metricsPostProcess = func(b []byte) (*bytes.Buffer, error) {
	buf := new(bytes.Buffer)
	if _, err := buf.Write(b); err != nil {
		return nil, err
	}
	fmt.Println(buf.String())
	return buf, nil
}

var allPostProcess = func(b []byte) (*bytes.Buffer, error) {
	_, err := validate(b)
	if err != nil {
		return metricsPostProcess(b)
	}
	return pprofPostProcess(b)
}
