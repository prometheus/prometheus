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
	"encoding/json"
	"errors"
	"net"
	"reflect"
	"sync"
	"testing"
	"time"

	"github.com/golang/protobuf/ptypes/timestamp"
	"google.golang.org/grpc"

	"go.opencensus.io/stats"
	"go.opencensus.io/stats/view"

	agentmetricspb "github.com/census-instrumentation/opencensus-proto/gen-go/agent/metrics/v1"
	metricspb "github.com/census-instrumentation/opencensus-proto/gen-go/metrics/v1"
)

type metricsAgent struct {
	mu      sync.RWMutex
	metrics []*agentmetricspb.ExportMetricsServiceRequest
}

func TestExportMetrics_conversionFromViewData(t *testing.T) {
	ln, err := net.Listen("tcp", ":0")
	if err != nil {
		t.Fatalf("Failed to get an available TCP address: %v", err)
	}
	defer ln.Close()

	_, agentPortStr, _ := net.SplitHostPort(ln.Addr().String())
	ma := new(metricsAgent)
	srv := grpc.NewServer()
	agentmetricspb.RegisterMetricsServiceServer(srv, ma)
	defer srv.Stop()
	go func() {
		_ = srv.Serve(ln)
	}()

	reconnectionPeriod := 2 * time.Millisecond
	ocexp, err := NewExporter(
		WithInsecure(),
		WithAddress(":"+agentPortStr),
		WithReconnectionPeriod(reconnectionPeriod),
	)
	if err != nil {
		t.Fatalf("Failed to create the ocagent exporter: %v", err)
	}
	<-time.After(5 * reconnectionPeriod)
	ocexp.Flush()

	startTime := time.Date(2018, 11, 25, 15, 38, 18, 997, time.UTC)
	endTime := startTime.Add(100 * time.Millisecond)

	mLatencyMs := stats.Float64("latency", "The latency for various methods", "ms")

	ocexp.ExportView(&view.Data{
		Start: startTime,
		End:   endTime,
		View: &view.View{
			Name:        "ocagent.io/latency",
			Description: "The latency of the various methods",
			Aggregation: view.Count(),
			Measure:     mLatencyMs,
		},
		Rows: []*view.Row{
			{
				Data: &view.CountData{Value: 4},
			},
		},
	})

	for i := 0; i < 5; i++ {
		ocexp.Flush()
	}

	<-time.After(100 * time.Millisecond)

	var received []*agentmetricspb.ExportMetricsServiceRequest
	ma.forEachRequest(func(req *agentmetricspb.ExportMetricsServiceRequest) {
		received = append(received, req)
	})

	// Now compare them with what we expect
	want := []*agentmetricspb.ExportMetricsServiceRequest{
		{
			Node:     NodeWithStartTime(""), // The first message identifying this application.
			Metrics:  nil,
			Resource: resourceProtoFromEnv(),
		},
		{
			Metrics: []*metricspb.Metric{
				{
					MetricDescriptor: &metricspb.MetricDescriptor{
						Name:        "ocagent.io/latency",
						Description: "The latency of the various methods",
						Unit:        "ms", // Derived from the measure
						Type:        metricspb.MetricDescriptor_CUMULATIVE_INT64,
						LabelKeys:   nil,
					},
					Timeseries: []*metricspb.TimeSeries{
						{
							StartTimestamp: &timestamp.Timestamp{
								Seconds: 1543160298,
								Nanos:   997,
							},
							LabelValues: nil,
							Points: []*metricspb.Point{
								{
									Timestamp: &timestamp.Timestamp{
										Seconds: 1543160298,
										Nanos:   100000997,
									},
									Value: &metricspb.Point_Int64Value{Int64Value: 4},
								},
							},
						},
					},
				},
			},
			Resource: resourceProtoFromEnv(),
		},
	}

	if !reflect.DeepEqual(received, want) {
		gj, _ := json.MarshalIndent(received, "", "  ")
		wj, _ := json.MarshalIndent(want, "", "  ")
		if string(gj) != string(wj) {
			t.Errorf("Got:\n%s\nWant:\n%s", gj, wj)
		}
	}
}

func (ma *metricsAgent) Export(mes agentmetricspb.MetricsService_ExportServer) error {
	// Expecting the first message to contain the Node information
	firstMetric, err := mes.Recv()
	if err != nil {
		return err
	}

	if firstMetric == nil || firstMetric.Node == nil {
		return errors.New("Expecting a non-nil Node in the first message")
	}

	ma.addMetric(firstMetric)

	for {
		msg, err := mes.Recv()
		if err != nil {
			return err
		}
		ma.addMetric(msg)
	}
}

func (ma *metricsAgent) addMetric(metric *agentmetricspb.ExportMetricsServiceRequest) {
	ma.mu.Lock()
	ma.metrics = append(ma.metrics, metric)
	ma.mu.Unlock()
}

func (ma *metricsAgent) forEachRequest(fn func(*agentmetricspb.ExportMetricsServiceRequest)) {
	ma.mu.RLock()
	defer ma.mu.RUnlock()

	for _, req := range ma.metrics {
		fn(req)
	}
}
