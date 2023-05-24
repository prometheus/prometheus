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
	failed := false

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

	// add empty string to avoid matching filename
	if len(files) == 0 {
		files = append(files, "")
	}

	for _, file := range files {
		var data []byte
		var err error

		// if file is an empty string it is a stdin
		if file == "" {
			data, err = io.ReadAll(os.Stdin)
			if err != nil {
				fmt.Fprintln(os.Stderr, "  FAILED:", err)
				failed = true
				break
			}

			fmt.Printf("Parsing input from stdin\n")
		} else {
			data, err = os.ReadFile(file)
			if err != nil {
				fmt.Fprintln(os.Stderr, "  FAILED:", err)
				failed = true
				continue
			}

			fmt.Printf("Parsing input from metric file %s\n", file)
		}
		metricsData, err := fmtutil.ParseMetricsTextAndFormat(bytes.NewReader(data), labels)
		if err != nil {
			fmt.Fprintln(os.Stderr, "  FAILED:", err)
			failed = true
			continue
		}

		raw, err := metricsData.Marshal()
		if err != nil {
			fmt.Fprintln(os.Stderr, "  FAILED:", err)
			failed = true
			continue
		}

		// Encode the request body into snappy encoding.
		compressed := snappy.Encode(nil, raw)
		err = client.Store(context.Background(), compressed)
		if err != nil {
			fmt.Fprintln(os.Stderr, "  FAILED:", err)
			failed = true
			continue
		}

		if file == "" {
			fmt.Printf("  SUCCESS: metric pushed to remote write.\n")
		} else {
			fmt.Printf("  SUCCESS: metric file %s pushed to remote write.\n", file)
		}
	}

	if failed {
		return failureExitCode
	}

	return successExitCode
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
