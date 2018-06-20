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

type debugWriterConfig struct {
	serverURL      string
	tarballName    string
	pathToFileName map[string]string
	postProcess    func(b []byte) ([]byte, error)
}

type debugWriter struct {
	archiver
	httpClient
	requestToFile map[*http.Request]string
	postProcess   func(b []byte) ([]byte, error)
}

func newDebugWriter(cfg debugWriterConfig) (*debugWriter, error) {
	client, err := newPrometheusHTTPClient(cfg.serverURL)
	if err != nil {
		return nil, err
	}
	archiver, err := newTarGzFileWriter(cfg.tarballName)
	if err != nil {
		return nil, err
	}
	reqs := make(map[*http.Request]string)
	for path, filename := range cfg.pathToFileName {
		req, err := http.NewRequest(http.MethodGet, client.urlJoin(path), nil)
		if err != nil {
			return nil, err
		}
		reqs[req] = filename
	}
	return &debugWriter{
		archiver,
		client,
		reqs,
		cfg.postProcess,
	}, nil
}

func (w *debugWriter) Write() int {
	for req, filename := range w.requestToFile {
		_, body, err := w.do(req)
		if err != nil {
			fmt.Fprintln(os.Stderr, "error executing HTTP request:", err)
			return 1
		}

		buf, err := w.postProcess(body)
		if err != nil {
			fmt.Fprintln(os.Stderr, "error post-processing HTTP response body:", err)
			return 1
		}

		if err := w.archiver.write(filename, buf); err != nil {
			fmt.Fprintln(os.Stderr, "error writing into archive:", err)
			return 1
		}
	}

	if err := w.close(); err != nil {
		fmt.Fprintln(os.Stderr, "error closing archiver:", err)
		return 1
	}

	fmt.Printf("Compiling debug information complete, all files written in %q.\n", w.filename())
	return 0
}

func validate(b []byte) (*profile.Profile, error) {
	p, err := profile.Parse(bytes.NewReader(b))
	if err != nil {
		return nil, err
	}
	return p, nil
}

var pprofPostProcess = func(b []byte) ([]byte, error) {
	p, err := validate(b)
	if err != nil {
		return nil, err
	}
	var buf bytes.Buffer
	if err := p.WriteUncompressed(&buf); err != nil {
		return nil, err
	}
	fmt.Println(p.String())
	return buf.Bytes(), nil
}

var metricsPostProcess = func(b []byte) ([]byte, error) {
	fmt.Println(string(b))
	return b, nil
}

var allPostProcess = func(b []byte) ([]byte, error) {
	_, err := validate(b)
	if err != nil {
		return metricsPostProcess(b)
	}
	return pprofPostProcess(b)
}
