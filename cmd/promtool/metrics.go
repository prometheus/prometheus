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

	remoteapi "github.com/prometheus/client_golang/exp/api/remote"

	"github.com/prometheus/prometheus/util/fmtutil"
)

// PushMetrics to a prometheus remote write (for testing purpose only).
func PushMetrics(url *url.URL, roundTripper http.RoundTripper, headers map[string]string, timeout time.Duration, protoMsg string, labels map[string]string, files ...string) int {
	addressURL, err := url.Parse(url.String())
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return failureExitCode
	}

	// Build HTTP client with custom transport for headers.
	httpClient := &http.Client{
		Transport: &setHeadersTransport{
			RoundTripper: roundTripper,
			headers:      headers,
		},
		Timeout: timeout,
	}

	// Create remote write API client.
	writeAPI, err := remoteapi.NewAPI(addressURL.String(), remoteapi.WithAPIHTTPClient(httpClient))
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return failureExitCode
	}

	// Determine message type based on protobuf_message flag.
	var messageType remoteapi.WriteMessageType
	switch protoMsg {
	case "io.prometheus.write.v2.Request":
		messageType = remoteapi.WriteV2MessageType
	case "prometheus.WriteRequest":
		messageType = remoteapi.WriteV1MessageType
	default:
		fmt.Fprintf(os.Stderr, "  FAILED: invalid protobuf message %q, must be prometheus.WriteRequest or io.prometheus.write.v2.Request\n", protoMsg)
		return failureExitCode
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
		if parseAndPushMetrics(writeAPI, messageType, data, labels) {
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
		if parseAndPushMetrics(writeAPI, messageType, data, labels) {
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

func parseAndPushMetrics(writeAPI *remoteapi.API, messageType remoteapi.WriteMessageType, data []byte, labels map[string]string) bool {
	metricsData, err := fmtutil.MetricTextToWriteRequest(bytes.NewReader(data), labels)
	if err != nil {
		fmt.Fprintln(os.Stderr, "  FAILED:", err)
		return false
	}

	// Use remoteapi.Write which handles marshaling and compression internally.
	_, err = writeAPI.Write(context.Background(), messageType, metricsData)
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
