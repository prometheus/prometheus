// Copyright 2023 The Prometheus Authors
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
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"time"

	"github.com/golang/snappy"
	config_util "github.com/prometheus/common/config"
	"github.com/prometheus/common/model"

	"github.com/prometheus/prometheus/storage/remote"
	"github.com/prometheus/prometheus/util/fmtutil"
)

// Push metrics to a prometheus remote write (for testing purpose only).
func PushMetrics(url *url.URL, roundTripper http.RoundTripper, headers map[string]string, timeout time.Duration, labels map[string]string, files ...string) int {
	addressURL, err := url.Parse(url.String())
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return failureExitCode
	}

	// build remote write client
	writeClient, err := remote.NewWriteClient("remote-write", &remote.ClientConfig{
		URL:     &config_util.URL{URL: addressURL},
		Timeout: model.Duration(timeout),
	})
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return failureExitCode
	}

	// set custom tls config from httpConfigFilePath
	// set custom headers to every request
	client, ok := writeClient.(*remote.Client)
	if !ok {
		fmt.Fprintln(os.Stderr, fmt.Errorf("unexpected type %T", writeClient))
		return failureExitCode
	}
	client.Client.Transport = &setHeadersTransport{
		RoundTripper: roundTripper,
		headers:      headers,
	}

	var data []byte
	var failed bool

	if len(files) == 0 {
		data, err = io.ReadAll(os.Stdin)
		if err != nil {
			fmt.Fprintln(os.Stderr, "  FAILED:", err)
			return failureExitCode
		}
		fmt.Printf("Parsing standard input\n")
		if parseAndPushMetrics(client, data, labels) {
			fmt.Printf("  SUCCESS: metrics pushed to remote write.\n")
			return successExitCode
		}
		return failureExitCode
	}

	for _, file := range files {
		data, err = os.ReadFile(file)
		if err != nil {
			fmt.Fprintln(os.Stderr, "  FAILED:", err)
			failed = true
			continue
		}

		fmt.Printf("Parsing metrics file %s\n", file)
		if parseAndPushMetrics(client, data, labels) {
			fmt.Printf("  SUCCESS: metrics file %s pushed to remote write.\n", file)
			continue
		}
		failed = true
	}

	if failed {
		return failureExitCode
	}

	return successExitCode
}

func parseAndPushMetrics(client *remote.Client, data []byte, labels map[string]string) bool {
	metricsData, err := fmtutil.MetricTextToWriteRequest(bytes.NewReader(data), labels)
	if err != nil {
		fmt.Fprintln(os.Stderr, "  FAILED:", err)
		return false
	}

	raw, err := metricsData.Marshal()
	if err != nil {
		fmt.Fprintln(os.Stderr, "  FAILED:", err)
		return false
	}

	// Encode the request body into snappy encoding.
	compressed := snappy.Encode(nil, raw)
	err = client.Store(context.Background(), compressed)
	if err != nil {
		fmt.Fprintln(os.Stderr, "  FAILED:", err)
		return false
	}

	return true
}

type setHeadersTransport struct {
	http.RoundTripper
	headers map[string]string
}

func (s *setHeadersTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	for key, value := range s.headers {
		req.Header.Set(key, value)
	}
	return s.RoundTripper.RoundTrip(req)
}
