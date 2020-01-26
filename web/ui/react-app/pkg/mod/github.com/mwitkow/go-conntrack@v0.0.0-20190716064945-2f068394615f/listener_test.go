// Copyright 2016 Michal Witkowski. All Rights Reserved.
// See LICENSE for licensing terms.

package conntrack_test

import (
	"net"
	"net/http"
	"testing"

	"context"

	"time"

	"github.com/mwitkow/go-conntrack"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
)

func TestListenerTestSuite(t *testing.T) {
	suite.Run(t, &ListenerTestSuite{})
}

var (
	listenerName = "some_name"
)

type ListenerTestSuite struct {
	suite.Suite

	serverListener net.Listener
	httpServer     http.Server
}

func (s *ListenerTestSuite) SetupSuite() {
	var err error
	s.serverListener, err = net.Listen("tcp", "127.0.0.1:0")
	require.NoError(s.T(), err, "must be able to allocate a port for serverListener")
	s.serverListener = conntrack.NewListener(s.serverListener, conntrack.TrackWithName(listenerName), conntrack.TrackWithTracing())
	s.httpServer = http.Server{
		Handler: http.HandlerFunc(func(resp http.ResponseWriter, req *http.Request) {
			resp.WriteHeader(http.StatusOK)
		}),
	}
	go func() {
		s.httpServer.Serve(s.serverListener)
	}()
}

func (s *ListenerTestSuite) TestTrackingMetricsPreregistered() {
	// this will create the default listener, check if it is registered
	conntrack.NewListener(s.serverListener)

	for testId, testCase := range []struct {
		metricName     string
		existingLabels []string
	}{
		{"net_conntrack_listener_conn_accepted_total", []string{"default"}},
		{"net_conntrack_listener_conn_closed_total", []string{"default"}},
		{"net_conntrack_listener_conn_accepted_total", []string{listenerName}},
		{"net_conntrack_listener_conn_closed_total", []string{listenerName}},
	} {
		lineCount := len(fetchPrometheusLines(s.T(), testCase.metricName, testCase.existingLabels...))
		assert.NotEqual(s.T(), 0, lineCount, "metrics must exist for test case %d", testId)
	}
}

func (s *ListenerTestSuite) TestMonitoringNormalConns() {

	beforeAccepted := sumCountersForMetricAndLabels(s.T(), "net_conntrack_listener_conn_accepted_total", listenerName)
	beforeClosed := sumCountersForMetricAndLabels(s.T(), "net_conntrack_listener_conn_closed_total", listenerName)

	conn, err := (&net.Dialer{}).DialContext(context.TODO(), "tcp", s.serverListener.Addr().String())
	require.NoError(s.T(), err, "DialContext should successfully establish a conn here")
	assert.Equal(s.T(), beforeAccepted+1, sumCountersForMetricAndLabels(s.T(), "net_conntrack_listener_conn_accepted_total", listenerName),
		"the accepted conn counter must be incremented after connection was opened")
	assert.Equal(s.T(), beforeClosed, sumCountersForMetricAndLabels(s.T(), "net_conntrack_listener_conn_closed_total", listenerName),
		"the closed conn counter must not be incremented before the connection is closed")
	conn.Close()
	assert.Equal(s.T(), beforeClosed+1, sumCountersForMetricAndLabels(s.T(), "net_conntrack_listener_conn_closed_total", listenerName),
		"the closed conn counter must be incremented after connection was closed")
}

func (s *ListenerTestSuite) TestTracingNormalComms() {
	conn, err := (&net.Dialer{}).DialContext(context.TODO(), "tcp", s.serverListener.Addr().String())
	require.NoError(s.T(), err, "DialContext should successfully establish a conn here")
	time.Sleep(5 * time.Millisecond)
	assert.Contains(s.T(), fetchTraceEvents(s.T(), "net.ServerConn."+listenerName), conn.LocalAddr().String(),
		"the /debug/trace/events page must contain the live connection")
	time.Sleep(5 * time.Millisecond)
	conn.Close()
}

func (s *ListenerTestSuite) TearDownSuite() {
	if s.serverListener != nil {
		s.T().Logf("stopped http.Server at: %v", s.serverListener.Addr().String())
		s.serverListener.Close()
	}
}
