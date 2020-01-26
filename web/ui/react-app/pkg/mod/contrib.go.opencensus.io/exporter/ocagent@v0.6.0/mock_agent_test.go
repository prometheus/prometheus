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

package ocagent_test

import (
	"fmt"
	"net"
	"sync"
	"testing"
	"time"

	"google.golang.org/grpc"

	commonpb "github.com/census-instrumentation/opencensus-proto/gen-go/agent/common/v1"
	agenttracepb "github.com/census-instrumentation/opencensus-proto/gen-go/agent/trace/v1"
	tracepb "github.com/census-instrumentation/opencensus-proto/gen-go/trace/v1"
)

func makeMockAgent(t *testing.T) *mockAgent {
	return &mockAgent{configsToSend: make(chan *agenttracepb.UpdatedLibraryConfig), t: t, wg: new(sync.WaitGroup)}
}

type mockAgent struct {
	t *testing.T

	spans []*tracepb.Span
	mu    sync.Mutex
	wg    *sync.WaitGroup

	traceNodes      []*commonpb.Node
	receivedConfigs []*agenttracepb.CurrentLibraryConfig

	configsToSend          chan *agenttracepb.UpdatedLibraryConfig
	closeConfigsToSendOnce sync.Once

	address  string
	stopFunc func() error
	stopOnce sync.Once
}

var _ agenttracepb.TraceServiceServer = (*mockAgent)(nil)

func (ma *mockAgent) Config(tscs agenttracepb.TraceService_ConfigServer) error {
	ma.mu.Lock()
	ma.wg.Add(1)
	ma.mu.Unlock()
	defer func() {
		ma.mu.Lock()
		ma.wg.Done()
		ma.mu.Unlock()
	}()

	in, err := tscs.Recv()
	if err != nil {
		return err
	}
	if in == nil || in.Node == nil {
		return fmt.Errorf("the first message must contain the node identifier")
	}
	ma.receivedConfigs = append(ma.receivedConfigs, in)

	// Push down all the configs
	for cfg := range ma.configsToSend {
		// Push down configs
		if err := tscs.Send(cfg); err != nil {
			return err
		}

		// And then get back the config sent back by the client library
		back, err := tscs.Recv()
		if err != nil {
			return err
		}
		ma.receivedConfigs = append(ma.receivedConfigs, back)
	}

	// Just for the sake of draining any configs
	// that the client-side exporter is still sending.
	for {
		back, err := tscs.Recv()
		if err != nil {
			return err
		}
		ma.receivedConfigs = append(ma.receivedConfigs, back)
	}
}

func (ma *mockAgent) Export(tses agenttracepb.TraceService_ExportServer) error {
	in, err := tses.Recv()
	if err != nil {
		return err
	}

	ma.mu.Lock()
	ma.wg.Add(1)
	ma.mu.Unlock()
	defer func() {
		ma.mu.Lock()
		ma.wg.Done()
		ma.mu.Unlock()
	}()

	// The first trace message should contain the node identifier.
	if in == nil || in.Node == nil {
		return fmt.Errorf("the first message must contain the node identifier")
	}
	ma.traceNodes = append(ma.traceNodes, in.Node)

	// Now that we have the node identifier, let's start receiving spans.
	for {
		req, err := tses.Recv()
		if err != nil {
			return err
		}
		ma.mu.Lock()
		ma.spans = append(ma.spans, req.Spans...)
		ma.traceNodes = append(ma.traceNodes, req.Node)
		ma.mu.Unlock()
	}
}

func (ma *mockAgent) transitionToReceivingClientConfigs() {
	// Since we are done sending all the configs, close the configsChannel
	// so that the state can transition to receiving all the client configs.
	ma.closeConfigsToSendOnce.Do(func() {
		close(ma.configsToSend)
	})
}

var errAlreadyStopped = fmt.Errorf("already stopped")

func (ma *mockAgent) stop() error {
	var err = errAlreadyStopped
	ma.stopOnce.Do(func() {
		ma.transitionToReceivingClientConfigs()

		if ma.stopFunc != nil {
			err = ma.stopFunc()
		}
	})
	// Give it sometime to shutdown.
	<-time.After(160 * time.Millisecond)
	ma.mu.Lock()
	ma.wg.Wait()
	ma.mu.Unlock()
	return err
}

// runMockAgent is a helper function to create a mockAgent
func runMockAgent(t *testing.T) *mockAgent {
	return runMockAgentAtAddr(t, ":0")
}

func runMockAgentAtAddr(t *testing.T, addr string) *mockAgent {
	var deferFuncs []func() error
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		t.Fatalf("Failed to get an address: %v", err)
	}
	deferFuncs = append(deferFuncs, ln.Close)

	srv := grpc.NewServer()
	ma := makeMockAgent(t)
	agenttracepb.RegisterTraceServiceServer(srv, ma)
	go func() {
		_ = srv.Serve(ln)
	}()

	deferFunc := func() error {
		srv.Stop()
		return ln.Close()
	}

	_, agentPortStr, _ := net.SplitHostPort(ln.Addr().String())

	ma.address = "localhost:" + agentPortStr
	ma.stopFunc = deferFunc

	return ma
}

func (ma *mockAgent) getSpans() []*tracepb.Span {
	ma.mu.Lock()
	spans := append([]*tracepb.Span{}, ma.spans...)
	ma.mu.Unlock()

	return spans
}

func (ma *mockAgent) getReceivedConfigs() []*agenttracepb.CurrentLibraryConfig {
	ma.mu.Lock()
	receivedConfigs := append([]*agenttracepb.CurrentLibraryConfig{}, ma.receivedConfigs...)
	ma.mu.Unlock()

	return receivedConfigs
}

func (ma *mockAgent) getTraceNodes() []*commonpb.Node {
	ma.mu.Lock()
	traceNodes := append([]*commonpb.Node{}, ma.traceNodes...)
	ma.mu.Unlock()

	return traceNodes
}
