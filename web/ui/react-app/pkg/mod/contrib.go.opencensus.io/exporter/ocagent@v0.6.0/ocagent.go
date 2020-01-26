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

package ocagent

import (
	"context"
	"errors"
	"fmt"
	"io"
	"sync"
	"time"
	"unsafe"

	"google.golang.org/api/support/bundler"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/metadata"

	"go.opencensus.io/plugin/ocgrpc"
	"go.opencensus.io/resource"
	"go.opencensus.io/stats/view"
	"go.opencensus.io/trace"

	commonpb "github.com/census-instrumentation/opencensus-proto/gen-go/agent/common/v1"
	agentmetricspb "github.com/census-instrumentation/opencensus-proto/gen-go/agent/metrics/v1"
	agenttracepb "github.com/census-instrumentation/opencensus-proto/gen-go/agent/trace/v1"
	metricspb "github.com/census-instrumentation/opencensus-proto/gen-go/metrics/v1"
	resourcepb "github.com/census-instrumentation/opencensus-proto/gen-go/resource/v1"
	tracepb "github.com/census-instrumentation/opencensus-proto/gen-go/trace/v1"
)

var startupMu sync.Mutex
var startTime time.Time

func init() {
	startupMu.Lock()
	startTime = time.Now()
	startupMu.Unlock()
}

var _ trace.Exporter = (*Exporter)(nil)
var _ view.Exporter = (*Exporter)(nil)

type Exporter struct {
	// mu protects the non-atomic and non-channel variables
	mu sync.RWMutex
	// senderMu protects the concurrent unsafe send on traceExporter client
	senderMu sync.Mutex
	// recvMu protects the concurrent unsafe recv on traceExporter client
	recvMu             sync.Mutex
	started            bool
	stopped            bool
	agentAddress       string
	serviceName        string
	canDialInsecure    bool
	traceExporter      agenttracepb.TraceService_ExportClient
	metricsExporter    agentmetricspb.MetricsService_ExportClient
	nodeInfo           *commonpb.Node
	grpcClientConn     *grpc.ClientConn
	reconnectionPeriod time.Duration
	resourceDetector   resource.Detector
	resource           *resourcepb.Resource
	compressor         string
	headers            map[string]string
	lastConnectErrPtr  unsafe.Pointer

	startOnce      sync.Once
	stopCh         chan bool
	disconnectedCh chan bool

	backgroundConnectionDoneCh chan bool

	traceBundler *bundler.Bundler

	// viewDataBundler is the bundler to enable conversion
	// from OpenCensus-Go view.Data to metricspb.Metric.
	// Please do not confuse it with metricsBundler!
	viewDataBundler *bundler.Bundler

	clientTransportCredentials credentials.TransportCredentials

	grpcDialOptions []grpc.DialOption
}

func NewExporter(opts ...ExporterOption) (*Exporter, error) {
	exp, err := NewUnstartedExporter(opts...)
	if err != nil {
		return nil, err
	}
	if err := exp.Start(); err != nil {
		return nil, err
	}
	return exp, nil
}

const spanDataBufferSize = 300

func NewUnstartedExporter(opts ...ExporterOption) (*Exporter, error) {
	e := new(Exporter)
	for _, opt := range opts {
		opt.withExporter(e)
	}
	traceBundler := bundler.NewBundler((*trace.SpanData)(nil), func(bundle interface{}) {
		e.uploadTraces(bundle.([]*trace.SpanData))
	})
	traceBundler.DelayThreshold = 2 * time.Second
	traceBundler.BundleCountThreshold = spanDataBufferSize
	e.traceBundler = traceBundler

	viewDataBundler := bundler.NewBundler((*view.Data)(nil), func(bundle interface{}) {
		e.uploadViewData(bundle.([]*view.Data))
	})
	viewDataBundler.DelayThreshold = 2 * time.Second
	viewDataBundler.BundleCountThreshold = 500 // TODO: (@odeke-em) make this configurable.
	e.viewDataBundler = viewDataBundler
	e.nodeInfo = NodeWithStartTime(e.serviceName)
	if e.resourceDetector != nil {
		res, err := e.resourceDetector(context.Background())
		if err != nil {
			panic(fmt.Sprintf("Error detecting resource. err:%v\n", err))
		}
		if res != nil {
			e.resource = resourceToResourcePb(res)
		}
	} else {
		e.resource = resourceProtoFromEnv()
	}

	return e, nil
}

const (
	maxInitialConfigRetries = 10
	maxInitialTracesRetries = 10
)

var (
	errAlreadyStarted = errors.New("already started")
	errNotStarted     = errors.New("not started")
	errStopped        = errors.New("stopped")
)

// Start dials to the agent, establishing a connection to it. It also
// initiates the Config and Trace services by sending over the initial
// messages that consist of the node identifier. Start invokes a background
// connector that will reattempt connections to the agent periodically
// if the connection dies.
func (ae *Exporter) Start() error {
	var err = errAlreadyStarted
	ae.startOnce.Do(func() {
		ae.mu.Lock()
		ae.started = true
		ae.disconnectedCh = make(chan bool, 1)
		ae.stopCh = make(chan bool)
		ae.backgroundConnectionDoneCh = make(chan bool)
		ae.mu.Unlock()

		// An optimistic first connection attempt to ensure that
		// applications under heavy load can immediately process
		// data. See https://github.com/census-ecosystem/opencensus-go-exporter-ocagent/pull/63
		if err := ae.connect(); err == nil {
			ae.setStateConnected()
		} else {
			ae.setStateDisconnected(err)
		}
		go ae.indefiniteBackgroundConnection()

		err = nil
	})

	return err
}

func (ae *Exporter) prepareAgentAddress() string {
	if ae.agentAddress != "" {
		return ae.agentAddress
	}
	return fmt.Sprintf("%s:%d", DefaultAgentHost, DefaultAgentPort)
}

func (ae *Exporter) enableConnectionStreams(cc *grpc.ClientConn) error {
	ae.mu.RLock()
	started := ae.started
	nodeInfo := ae.nodeInfo
	ae.mu.RUnlock()

	if !started {
		return errNotStarted
	}

	ae.mu.Lock()
	// If the previous clientConn was non-nil, close it
	if ae.grpcClientConn != nil {
		_ = ae.grpcClientConn.Close()
	}
	ae.grpcClientConn = cc
	ae.mu.Unlock()

	if err := ae.createTraceServiceConnection(ae.grpcClientConn, nodeInfo); err != nil {
		return err
	}

	return ae.createMetricsServiceConnection(ae.grpcClientConn, nodeInfo)
}

func (ae *Exporter) createTraceServiceConnection(cc *grpc.ClientConn, node *commonpb.Node) error {
	// Initiate the trace service by sending over node identifier info.
	traceSvcClient := agenttracepb.NewTraceServiceClient(cc)
	ctx := context.Background()
	if len(ae.headers) > 0 {
		ctx = metadata.NewOutgoingContext(ctx, metadata.New(ae.headers))
	}
	traceExporter, err := traceSvcClient.Export(ctx)
	if err != nil {
		return fmt.Errorf("Exporter.Start:: TraceServiceClient: %v", err)
	}

	firstTraceMessage := &agenttracepb.ExportTraceServiceRequest{
		Node:     node,
		Resource: ae.resource,
	}
	if err := traceExporter.Send(firstTraceMessage); err != nil {
		return fmt.Errorf("Exporter.Start:: Failed to initiate the Config service: %v", err)
	}

	ae.mu.Lock()
	ae.traceExporter = traceExporter
	ae.mu.Unlock()

	// Initiate the config service by sending over node identifier info.
	configStream, err := traceSvcClient.Config(context.Background())
	if err != nil {
		return fmt.Errorf("Exporter.Start:: ConfigStream: %v", err)
	}
	firstCfgMessage := &agenttracepb.CurrentLibraryConfig{Node: node}
	if err := configStream.Send(firstCfgMessage); err != nil {
		return fmt.Errorf("Exporter.Start:: Failed to initiate the Config service: %v", err)
	}

	// In the background, handle trace configurations that are beamed down
	// by the agent, but also reply to it with the applied configuration.
	go ae.handleConfigStreaming(configStream)

	return nil
}

func (ae *Exporter) createMetricsServiceConnection(cc *grpc.ClientConn, node *commonpb.Node) error {
	metricsSvcClient := agentmetricspb.NewMetricsServiceClient(cc)
	metricsExporter, err := metricsSvcClient.Export(context.Background())
	if err != nil {
		return fmt.Errorf("MetricsExporter: failed to start the service client: %v", err)
	}
	// Initiate the metrics service by sending over the first message just containing the Node and Resource.
	firstMetricsMessage := &agentmetricspb.ExportMetricsServiceRequest{
		Node:     node,
		Resource: ae.resource,
	}
	if err := metricsExporter.Send(firstMetricsMessage); err != nil {
		return fmt.Errorf("MetricsExporter:: failed to send the first message: %v", err)
	}

	ae.mu.Lock()
	ae.metricsExporter = metricsExporter
	ae.mu.Unlock()

	// With that we are good to go and can start sending metrics
	return nil
}

func (ae *Exporter) dialToAgent() (*grpc.ClientConn, error) {
	addr := ae.prepareAgentAddress()
	var dialOpts []grpc.DialOption
	if ae.clientTransportCredentials != nil {
		dialOpts = append(dialOpts, grpc.WithTransportCredentials(ae.clientTransportCredentials))
	} else if ae.canDialInsecure {
		dialOpts = append(dialOpts, grpc.WithInsecure())
	}
	if ae.compressor != "" {
		dialOpts = append(dialOpts, grpc.WithDefaultCallOptions(grpc.UseCompressor(ae.compressor)))
	}
	dialOpts = append(dialOpts, grpc.WithStatsHandler(&ocgrpc.ClientHandler{}))
	if len(ae.grpcDialOptions) != 0 {
		dialOpts = append(dialOpts, ae.grpcDialOptions...)
	}

	ctx := context.Background()
	if len(ae.headers) > 0 {
		ctx = metadata.NewOutgoingContext(ctx, metadata.New(ae.headers))
	}
	return grpc.DialContext(ctx, addr, dialOpts...)
}

func (ae *Exporter) handleConfigStreaming(configStream agenttracepb.TraceService_ConfigClient) error {
	// Note: We haven't yet implemented configuration sending so we
	// should NOT be changing connection states within this function for now.
	for {
		recv, err := configStream.Recv()
		if err != nil {
			// TODO: Check if this is a transient error or exponential backoff-able.
			return err
		}
		cfg := recv.Config
		if cfg == nil {
			continue
		}

		// Otherwise now apply the trace configuration sent down from the agent
		if psamp := cfg.GetProbabilitySampler(); psamp != nil {
			trace.ApplyConfig(trace.Config{DefaultSampler: trace.ProbabilitySampler(psamp.SamplingProbability)})
		} else if csamp := cfg.GetConstantSampler(); csamp != nil {
			alwaysSample := csamp.Decision == tracepb.ConstantSampler_ALWAYS_ON
			if alwaysSample {
				trace.ApplyConfig(trace.Config{DefaultSampler: trace.AlwaysSample()})
			} else {
				trace.ApplyConfig(trace.Config{DefaultSampler: trace.NeverSample()})
			}
		} else { // TODO: Add the rate limiting sampler here
		}

		// Then finally send back to upstream the newly applied configuration
		err = configStream.Send(&agenttracepb.CurrentLibraryConfig{Config: &tracepb.TraceConfig{Sampler: cfg.Sampler}})
		if err != nil {
			return err
		}
	}
}

// Stop shuts down all the connections and resources
// related to the exporter.
func (ae *Exporter) Stop() error {
	ae.mu.RLock()
	cc := ae.grpcClientConn
	started := ae.started
	stopped := ae.stopped
	ae.mu.RUnlock()

	if !started {
		return errNotStarted
	}
	if stopped {
		// TODO: tell the user that we've already stopped, so perhaps a sentinel error?
		return nil
	}

	ae.Flush()

	// Now close the underlying gRPC connection.
	var err error
	if cc != nil {
		err = cc.Close()
	}

	// At this point we can change the state variables: started and stopped
	ae.mu.Lock()
	ae.started = false
	ae.stopped = true
	ae.mu.Unlock()
	close(ae.stopCh)

	// Ensure that the backgroundConnector returns
	<-ae.backgroundConnectionDoneCh

	return err
}

func (ae *Exporter) ExportSpan(sd *trace.SpanData) {
	if sd == nil {
		return
	}
	_ = ae.traceBundler.Add(sd, 1)
}

func (ae *Exporter) ExportTraceServiceRequest(batch *agenttracepb.ExportTraceServiceRequest) error {
	if batch == nil || len(batch.Spans) == 0 {
		return nil
	}

	select {
	case <-ae.stopCh:
		return errStopped

	default:
		if lastConnectErr := ae.lastConnectError(); lastConnectErr != nil {
			return fmt.Errorf("ExportTraceServiceRequest: no active connection, last connection error: %v", lastConnectErr)
		}

		ae.senderMu.Lock()
		err := ae.traceExporter.Send(batch)
		ae.senderMu.Unlock()
		if err != nil {
			if err == io.EOF {
				ae.recvMu.Lock()
				// Perform a .Recv to try to find out why the RPC actually ended.
				// See:
				//   * https://github.com/grpc/grpc-go/blob/d389f9fac68eea0dcc49957d0b4cca5b3a0a7171/stream.go#L98-L100
				//   * https://groups.google.com/forum/#!msg/grpc-io/XcN4hA9HonI/F_UDiejTAwAJ
				for {
					_, err = ae.traceExporter.Recv()
					if err != nil {
						break
					}
				}
				ae.recvMu.Unlock()
			}

			ae.setStateDisconnected(err)
			if err != io.EOF {
				return err
			}
		}
		return nil
	}
}

func (ae *Exporter) ExportView(vd *view.Data) {
	if vd == nil {
		return
	}
	_ = ae.viewDataBundler.Add(vd, 1)
}

// ExportMetricsServiceRequest sends proto metrics with the metrics service client.
func (ae *Exporter) ExportMetricsServiceRequest(batch *agentmetricspb.ExportMetricsServiceRequest) error {
	if batch == nil || len(batch.Metrics) == 0 {
		return nil
	}

	select {
	case <-ae.stopCh:
		return errStopped

	default:
		if lastConnectErr := ae.lastConnectError(); lastConnectErr != nil {
			return fmt.Errorf("ExportMetricsServiceRequest: no active connection, last connection error: %v", lastConnectErr)
		}

		ae.senderMu.Lock()
		err := ae.metricsExporter.Send(batch)
		ae.senderMu.Unlock()
		if err != nil {
			if err == io.EOF {
				ae.recvMu.Lock()
				// Perform a .Recv to try to find out why the RPC actually ended.
				// See:
				//   * https://github.com/grpc/grpc-go/blob/d389f9fac68eea0dcc49957d0b4cca5b3a0a7171/stream.go#L98-L100
				//   * https://groups.google.com/forum/#!msg/grpc-io/XcN4hA9HonI/F_UDiejTAwAJ
				for {
					_, err = ae.metricsExporter.Recv()
					if err != nil {
						break
					}
				}
				ae.recvMu.Unlock()
			}

			ae.setStateDisconnected(err)
			if err != io.EOF {
				return err
			}
		}
		return nil
	}
}

func ocSpanDataToPbSpans(sdl []*trace.SpanData) []*tracepb.Span {
	if len(sdl) == 0 {
		return nil
	}
	protoSpans := make([]*tracepb.Span, 0, len(sdl))
	for _, sd := range sdl {
		if sd != nil {
			protoSpans = append(protoSpans, ocSpanToProtoSpan(sd))
		}
	}
	return protoSpans
}

func (ae *Exporter) uploadTraces(sdl []*trace.SpanData) {
	select {
	case <-ae.stopCh:
		return

	default:
		if !ae.connected() {
			return
		}

		protoSpans := ocSpanDataToPbSpans(sdl)
		if len(protoSpans) == 0 {
			return
		}
		ae.senderMu.Lock()
		err := ae.traceExporter.Send(&agenttracepb.ExportTraceServiceRequest{
			Spans:    protoSpans,
			Resource: resourceProtoFromEnv(),
		})
		ae.senderMu.Unlock()
		if err != nil {
			ae.setStateDisconnected(err)
		}
	}
}

func ocViewDataToPbMetrics(vdl []*view.Data) []*metricspb.Metric {
	if len(vdl) == 0 {
		return nil
	}
	metrics := make([]*metricspb.Metric, 0, len(vdl))
	for _, vd := range vdl {
		if vd != nil {
			vmetric, err := viewDataToMetric(vd)
			// TODO: (@odeke-em) somehow report this error, if it is non-nil.
			if err == nil && vmetric != nil {
				metrics = append(metrics, vmetric)
			}
		}
	}
	return metrics
}

func (ae *Exporter) uploadViewData(vdl []*view.Data) {
	protoMetrics := ocViewDataToPbMetrics(vdl)
	if len(protoMetrics) == 0 {
		return
	}
	req := &agentmetricspb.ExportMetricsServiceRequest{
		Metrics:  protoMetrics,
		Resource: resourceProtoFromEnv(),
		// TODO:(@odeke-em)
		// a) Figure out how to derive a Node from the environment
		// or better letting users of the exporter configure it.
	}
	ae.ExportMetricsServiceRequest(req)
}

func (ae *Exporter) Flush() {
	ae.traceBundler.Flush()
	ae.viewDataBundler.Flush()
}

func resourceProtoFromEnv() *resourcepb.Resource {
	rs, _ := resource.FromEnv(context.Background())
	if rs == nil {
		return nil
	}
	return resourceToResourcePb(rs)
}

func resourceToResourcePb(rs *resource.Resource) *resourcepb.Resource {
	rprs := &resourcepb.Resource{
		Type: rs.Type,
	}
	if rs.Labels != nil {
		rprs.Labels = make(map[string]string)
		for k, v := range rs.Labels {
			rprs.Labels[k] = v
		}
	}
	return rprs
}
