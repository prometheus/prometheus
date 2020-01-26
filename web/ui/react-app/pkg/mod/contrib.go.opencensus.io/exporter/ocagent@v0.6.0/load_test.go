// Copyright 2018, OpenCensus Authors
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

// +build go1.11

package ocagent_test

import (
	"context"
	"errors"
	"fmt"
	"io/ioutil"
	"net"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"google.golang.org/grpc"

	"contrib.go.opencensus.io/exporter/ocagent"
	agenttracepb "github.com/census-instrumentation/opencensus-proto/gen-go/agent/trace/v1"
	"go.opencensus.io/plugin/ochttp"
	"go.opencensus.io/trace"
)

// Issue #13. The agent exporter was crashing due to -1 that was being
// passed in as a size, into the bundler when exporting spans under load.
func TestExportsUnderLoad_issue13(t *testing.T) {
	// Create the agent's listener.
	agentLn, err := net.Listen("tcp", ":0")
	if err != nil {
		t.Fatalf("Failed to bind to address: %v", err)
	}
	defer agentLn.Close()
	agentPort, err := parsePort(agentLn.Addr())
	if err != nil {
		t.Fatalf("Could not parse port from agent address: %v", err)
	}
	agentAddr := fmt.Sprintf("localhost:%d", agentPort)

	// Then create the frontend HTTP server's listener which
	// will be used to generate tertiary spans that
	// the ocagent-exporter uploads to the agent.
	frontendLn, err := net.Listen("tcp", ":0")
	if err != nil {
		t.Fatalf("Failed to create frontend server listener: %v", err)
	}
	defer frontendLn.Close()

	frontendPort, err := parsePort(frontendLn.Addr())
	if err != nil {
		t.Fatalf("Failed to parse port from frontend server address: %v", err)
	}

	agentSrv := grpc.NewServer()
	agenttracepb.RegisterTraceServiceServer(agentSrv, new(discardAgent))
	go func() {
		_ = agentSrv.Serve(agentLn)
	}()

	exporter, err := ocagent.NewExporter(
		ocagent.WithInsecure(),
		ocagent.WithServiceName("go-app"),
		ocagent.WithReconnectionPeriod(50*time.Millisecond),
		ocagent.WithAddress(agentAddr))
	if err != nil {
		t.Fatalf("Failed to create the agent exporter: %v", err)
	}
	defer exporter.Stop()

	trace.RegisterExporter(exporter)

	body := "Hello, World!"
	frontend := &http.Server{Handler: &ochttp.Handler{
		Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Write([]byte(body))
		}),
	}}
	go func() {
		_ = frontend.Serve(frontendLn)
	}()

	// Creating our semaphore and load it up.
	// with 50 concurrent requests to generate many spans in short burts.
	// Using 50 because that's a high number for multiplicity of open file descriptors.
	n := 50
	reqSemaphore := make(chan bool, n)
	// Stock it up with concurrency tokens
	for i := 0; i < n; i++ {
		reqSemaphore <- true
	}

	frontendURL := fmt.Sprintf("http://localhost:%d", frontendPort)
	var wg sync.WaitGroup
	httpClient := &http.Client{Transport: &ochttp.Transport{Base: &http.Transport{MaxConnsPerHost: n}}}
	for i := 0; i < 10000; i++ {
		// Throttle to ensure we have a max of n concurrent requests
		<-reqSemaphore
		// Permitted to go on
		wg.Add(1)
		go func(id int) {
			defer func() {
				// Now declare that we are complete
				reqSemaphore <- true
				wg.Done()
			}()

			req, _ := http.NewRequest("GET", frontendURL, nil)
			ctx, span := trace.StartSpan(context.Background(), "TestSpan", trace.WithSampler(trace.AlwaysSample()))
			req = req.WithContext(ctx)
			res, err := httpClient.Do(req)
			span.End()
			if err != nil {
				t.Errorf("Request #%d: error: %v", id, err)
			} else {
				blob, err := ioutil.ReadAll(res.Body)
				if err != nil {
					t.Errorf("Request #%d: ReadAll error: %v", id, err)
				}
				_ = res.Body.Close()
				if g, w := string(blob), body; g != w {
					t.Errorf("Request #%d:\nGot:  %s\nWant: %s\n", id, g, w)
				}
			}
		}(i)
	}

	// Wait until every single request has completed.
	wg.Wait()
}

type discardAgent struct{}

var _ agenttracepb.TraceServiceServer = (*discardAgent)(nil)

func (da *discardAgent) Config(tscs agenttracepb.TraceService_ConfigServer) error {
	return nil
}

func (da *discardAgent) Export(tses agenttracepb.TraceService_ExportServer) error {
	for {
		req, err := tses.Recv()
		if err != nil {
			return err
		}
		if req == nil {
		}
	}
}

func parsePort(addr net.Addr) (uint16, error) {
	addrStr := addr.String()
	if i := strings.LastIndex(addrStr, ":"); i < 0 {
		return 0, errors.New("no `:` found")
	} else {
		port, err := strconv.Atoi(addrStr[i+1:])
		return uint16(port), err
	}
}
