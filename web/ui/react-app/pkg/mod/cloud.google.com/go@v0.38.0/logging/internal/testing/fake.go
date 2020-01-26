/*
Copyright 2016 Google LLC

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

// Package testing provides support for testing the logging client.
package testing

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"

	"cloud.google.com/go/internal/testutil"
	emptypb "github.com/golang/protobuf/ptypes/empty"
	tspb "github.com/golang/protobuf/ptypes/timestamp"
	lpb "google.golang.org/genproto/googleapis/api/label"
	mrpb "google.golang.org/genproto/googleapis/api/monitoredres"
	logpb "google.golang.org/genproto/googleapis/logging/v2"
)

type loggingHandler struct {
	logpb.LoggingServiceV2Server

	mu   sync.Mutex
	logs map[string][]*logpb.LogEntry // indexed by log name
}

type configHandler struct {
	logpb.ConfigServiceV2Server

	mu    sync.Mutex
	sinks map[string]*logpb.LogSink // indexed by (full) sink name
}

type metricHandler struct {
	logpb.MetricsServiceV2Server

	mu      sync.Mutex
	metrics map[string]*logpb.LogMetric // indexed by (full) metric name
}

// NewServer creates a new in-memory fake server implementing the logging service.
// It returns the address of the server.
func NewServer() (string, error) {
	srv, err := testutil.NewServer()
	if err != nil {
		return "", err
	}
	logpb.RegisterLoggingServiceV2Server(srv.Gsrv, &loggingHandler{
		logs: make(map[string][]*logpb.LogEntry),
	})
	logpb.RegisterConfigServiceV2Server(srv.Gsrv, &configHandler{
		sinks: make(map[string]*logpb.LogSink),
	})
	logpb.RegisterMetricsServiceV2Server(srv.Gsrv, &metricHandler{
		metrics: make(map[string]*logpb.LogMetric),
	})
	srv.Start()
	return srv.Addr, nil
}

// DeleteLog deletes a log and all its log entries. The log will reappear if it
// receives new entries.
func (h *loggingHandler) DeleteLog(_ context.Context, req *logpb.DeleteLogRequest) (*emptypb.Empty, error) {
	// TODO(jba): return NotFound if log isn't there?
	h.mu.Lock()
	defer h.mu.Unlock()
	delete(h.logs, req.LogName)
	return &emptypb.Empty{}, nil
}

// The only IDs that WriteLogEntries will accept.
// Important for testing Ping.
const (
	ValidProjectID = "PROJECT_ID"
	ValidOrgID     = "433637338589"

	SharedServiceAccount = "serviceAccount:cloud-logs@system.gserviceaccount.com"
)

// WriteLogEntries writes log entries to Stackdriver Logging. All log entries in
// Stackdriver Logging are written by this method.
func (h *loggingHandler) WriteLogEntries(_ context.Context, req *logpb.WriteLogEntriesRequest) (*logpb.WriteLogEntriesResponse, error) {
	if !strings.HasPrefix(req.LogName, "projects/"+ValidProjectID+"/") && !strings.HasPrefix(req.LogName, "organizations/"+ValidOrgID+"/") {
		return nil, fmt.Errorf("bad LogName: %q", req.LogName)
	}
	// TODO(jba): support insertId?
	h.mu.Lock()
	defer h.mu.Unlock()
	for _, e := range req.Entries {
		// Assign timestamp if missing.
		if e.Timestamp == nil {
			e.Timestamp = &tspb.Timestamp{Seconds: time.Now().Unix(), Nanos: 0}
		}
		// Fill from common fields in request.
		if e.LogName == "" {
			e.LogName = req.LogName
		}
		if e.Resource == nil {
			// TODO(jba): use a global one if nil?
			e.Resource = req.Resource
		}
		for k, v := range req.Labels {
			if _, ok := e.Labels[k]; !ok {
				e.Labels[k] = v
			}
		}

		// Store by log name.
		h.logs[e.LogName] = append(h.logs[e.LogName], e)
	}
	return &logpb.WriteLogEntriesResponse{}, nil
}

// ListLogEntries lists log entries. Use this method to retrieve log entries
// from Stackdriver Logging.
//
// This fake implementation ignores project IDs. It does not support full filtering, only
// expressions of the form "logName = NAME".
func (h *loggingHandler) ListLogEntries(_ context.Context, req *logpb.ListLogEntriesRequest) (*logpb.ListLogEntriesResponse, error) {
	h.mu.Lock()
	defer h.mu.Unlock()
	entries, err := h.filterEntries(req.Filter)
	if err != nil {
		return nil, err
	}
	if err = sortEntries(entries, req.OrderBy); err != nil {
		return nil, err
	}

	from, to, nextPageToken, err := testutil.PageBounds(int(req.PageSize), req.PageToken, len(entries))
	if err != nil {
		return nil, err
	}
	return &logpb.ListLogEntriesResponse{
		Entries:       entries[from:to],
		NextPageToken: nextPageToken,
	}, nil
}

func (h *loggingHandler) filterEntries(filter string) ([]*logpb.LogEntry, error) {
	logName, err := parseFilter(filter)
	if err != nil {
		return nil, err
	}
	if logName != "" {
		return h.logs[logName], nil
	}
	var entries []*logpb.LogEntry
	for _, es := range h.logs {
		entries = append(entries, es...)
	}
	return entries, nil
}

var filterRegexp = regexp.MustCompile(`^logName\s*=\s*"?([-_/.%\w]+)"?`)

// returns the log name, or "" for the empty filter
func parseFilter(filter string) (string, error) {
	if filter == "" {
		return "", nil
	}
	subs := filterRegexp.FindStringSubmatch(filter)
	if subs == nil {
		return "", invalidArgument(fmt.Sprintf("fake.go: failed to parse filter %s", filter))
	}
	return subs[1], nil // cannot panic by construction of regexp
}

func sortEntries(entries []*logpb.LogEntry, orderBy string) error {
	switch orderBy {
	case "", "timestamp asc":
		sort.Sort(byTimestamp(entries))
		return nil

	case "timestamp desc":
		sort.Sort(sort.Reverse(byTimestamp(entries)))
		return nil

	default:
		return invalidArgument("bad order_by")
	}
}

type byTimestamp []*logpb.LogEntry

func (s byTimestamp) Len() int      { return len(s) }
func (s byTimestamp) Swap(i, j int) { s[i], s[j] = s[j], s[i] }
func (s byTimestamp) Less(i, j int) bool {
	c := compareTimestamps(s[i].Timestamp, s[j].Timestamp)
	switch {
	case c < 0:
		return true
	case c > 0:
		return false
	default:
		return s[i].InsertId < s[j].InsertId
	}
}

func compareTimestamps(ts1, ts2 *tspb.Timestamp) int64 {
	if ts1.Seconds != ts2.Seconds {
		return ts1.Seconds - ts2.Seconds
	}
	return int64(ts1.Nanos - ts2.Nanos)
}

// Lists monitored resource descriptors that are used by Stackdriver Logging.
func (h *loggingHandler) ListMonitoredResourceDescriptors(context.Context, *logpb.ListMonitoredResourceDescriptorsRequest) (*logpb.ListMonitoredResourceDescriptorsResponse, error) {
	return &logpb.ListMonitoredResourceDescriptorsResponse{
		ResourceDescriptors: []*mrpb.MonitoredResourceDescriptor{
			{
				Type:        "global",
				DisplayName: "Global",
				Description: "... a log is not associated with any specific resource.",
				Labels: []*lpb.LabelDescriptor{
					{Key: "project_id", Description: "The identifier of the GCP project..."},
				},
			},
		},
	}, nil
}

// Lists logs.
func (h *loggingHandler) ListLogs(_ context.Context, req *logpb.ListLogsRequest) (*logpb.ListLogsResponse, error) {
	// Return fixed, fake response.
	logNames := []string{"a", "b", "c"}
	from, to, npt, err := testutil.PageBounds(int(req.PageSize), req.PageToken, len(logNames))
	if err != nil {
		return nil, err
	}
	var lns []string
	for _, ln := range logNames[from:to] {
		lns = append(lns, req.Parent+"/logs/"+ln)
	}
	return &logpb.ListLogsResponse{
		LogNames:      lns,
		NextPageToken: npt,
	}, nil
}

// Gets a sink.
func (h *configHandler) GetSink(_ context.Context, req *logpb.GetSinkRequest) (*logpb.LogSink, error) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if s, ok := h.sinks[req.SinkName]; ok {
		return s, nil
	}
	// TODO(jba): use error codes
	return nil, fmt.Errorf("sink %q not found", req.SinkName)
}

// Creates a sink.
func (h *configHandler) CreateSink(_ context.Context, req *logpb.CreateSinkRequest) (*logpb.LogSink, error) {
	h.mu.Lock()
	defer h.mu.Unlock()
	fullName := fmt.Sprintf("%s/sinks/%s", req.Parent, req.Sink.Name)
	if _, ok := h.sinks[fullName]; ok {
		return nil, fmt.Errorf("sink with name %q already exists", fullName)
	}
	h.setSink(fullName, req.Sink, req.UniqueWriterIdentity)
	return req.Sink, nil
}

func (h *configHandler) setSink(name string, s *logpb.LogSink, uniqueWriterIdentity bool) {
	if uniqueWriterIdentity {
		s.WriterIdentity = "serviceAccount:" + name + "@gmail.com"
	} else {
		s.WriterIdentity = SharedServiceAccount
	}
	h.sinks[name] = s
}

// Creates or updates a sink.
func (h *configHandler) UpdateSink(_ context.Context, req *logpb.UpdateSinkRequest) (*logpb.LogSink, error) {
	h.mu.Lock()
	defer h.mu.Unlock()
	sink := h.sinks[req.SinkName]
	// Update of a non-existent sink will create it.
	if sink == nil {
		h.setSink(req.SinkName, req.Sink, req.UniqueWriterIdentity)
		sink = req.Sink
	} else {
		// sink is the existing sink named req.SinkName.
		// Update all and only the fields of sink that are specified in the update mask.
		paths := req.UpdateMask.GetPaths()
		if len(paths) == 0 {
			// An empty update mask is considered to have these fields by default.
			paths = []string{"destination", "filter", "include_children"}
		}
		for _, p := range paths {
			switch p {
			case "destination":
				sink.Destination = req.Sink.Destination
			case "filter":
				sink.Filter = req.Sink.Filter
			case "include_children":
				sink.IncludeChildren = req.Sink.IncludeChildren
			case "output_version_format":
				// noop
			default:
				return nil, fmt.Errorf("unknown path in mask: %q", p)
			}
		}
		if req.UniqueWriterIdentity {
			if sink.WriterIdentity != SharedServiceAccount {
				return nil, invalidArgument("cannot change unique writer identity")
			}
			sink.WriterIdentity = "serviceAccount:" + req.SinkName + "@gmail.com"
		}
	}
	return sink, nil

}

// Deletes a sink.
func (h *configHandler) DeleteSink(_ context.Context, req *logpb.DeleteSinkRequest) (*emptypb.Empty, error) {
	h.mu.Lock()
	defer h.mu.Unlock()
	delete(h.sinks, req.SinkName)
	return &emptypb.Empty{}, nil
}

// Lists sinks. This fake implementation ignores the Parent field of
// ListSinksRequest. All sinks are listed, regardless of their project.
func (h *configHandler) ListSinks(_ context.Context, req *logpb.ListSinksRequest) (*logpb.ListSinksResponse, error) {
	h.mu.Lock()
	var sinks []*logpb.LogSink
	for _, s := range h.sinks {
		sinks = append(sinks, s)
	}
	h.mu.Unlock() // safe because no *logpb.LogSink is ever modified
	// Since map iteration varies, sort the sinks.
	sort.Sort(sinksByName(sinks))
	from, to, nextPageToken, err := testutil.PageBounds(int(req.PageSize), req.PageToken, len(sinks))
	if err != nil {
		return nil, err
	}
	return &logpb.ListSinksResponse{
		Sinks:         sinks[from:to],
		NextPageToken: nextPageToken,
	}, nil
}

type sinksByName []*logpb.LogSink

func (s sinksByName) Len() int           { return len(s) }
func (s sinksByName) Swap(i, j int)      { s[i], s[j] = s[j], s[i] }
func (s sinksByName) Less(i, j int) bool { return s[i].Name < s[j].Name }

// Gets a metric.
func (h *metricHandler) GetLogMetric(_ context.Context, req *logpb.GetLogMetricRequest) (*logpb.LogMetric, error) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if s, ok := h.metrics[req.MetricName]; ok {
		return s, nil
	}
	// TODO(jba): use error codes
	return nil, fmt.Errorf("metric %q not found", req.MetricName)
}

// Creates a metric.
func (h *metricHandler) CreateLogMetric(_ context.Context, req *logpb.CreateLogMetricRequest) (*logpb.LogMetric, error) {
	h.mu.Lock()
	defer h.mu.Unlock()
	fullName := fmt.Sprintf("%s/metrics/%s", req.Parent, req.Metric.Name)
	if _, ok := h.metrics[fullName]; ok {
		return nil, fmt.Errorf("metric with name %q already exists", fullName)
	}
	h.metrics[fullName] = req.Metric
	return req.Metric, nil
}

// Creates or updates a metric.
func (h *metricHandler) UpdateLogMetric(_ context.Context, req *logpb.UpdateLogMetricRequest) (*logpb.LogMetric, error) {
	h.mu.Lock()
	defer h.mu.Unlock()
	// Update of a non-existent metric will create it.
	h.metrics[req.MetricName] = req.Metric
	return req.Metric, nil
}

// Deletes a metric.
func (h *metricHandler) DeleteLogMetric(_ context.Context, req *logpb.DeleteLogMetricRequest) (*emptypb.Empty, error) {
	h.mu.Lock()
	defer h.mu.Unlock()
	delete(h.metrics, req.MetricName)
	return &emptypb.Empty{}, nil
}

// Lists metrics. This fake implementation ignores the Parent field of
// ListMetricsRequest. All metrics are listed, regardless of their project.
func (h *metricHandler) ListLogMetrics(_ context.Context, req *logpb.ListLogMetricsRequest) (*logpb.ListLogMetricsResponse, error) {
	h.mu.Lock()
	var metrics []*logpb.LogMetric
	for _, s := range h.metrics {
		metrics = append(metrics, s)
	}
	h.mu.Unlock() // safe because no *logpb.LogMetric is ever modified
	// Since map iteration varies, sort the metrics.
	sort.Sort(metricsByName(metrics))
	from, to, nextPageToken, err := testutil.PageBounds(int(req.PageSize), req.PageToken, len(metrics))
	if err != nil {
		return nil, err
	}
	return &logpb.ListLogMetricsResponse{
		Metrics:       metrics[from:to],
		NextPageToken: nextPageToken,
	}, nil
}

type metricsByName []*logpb.LogMetric

func (s metricsByName) Len() int           { return len(s) }
func (s metricsByName) Swap(i, j int)      { s[i], s[j] = s[j], s[i] }
func (s metricsByName) Less(i, j int) bool { return s[i].Name < s[j].Name }

func invalidArgument(msg string) error {
	// TODO(jba): status codes
	return errors.New(msg)
}
