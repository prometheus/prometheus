// Copyright 2016 Michal Witkowski. All Rights Reserved.
// See LICENSE for licensing terms.

package conntrack_test

import (
	"context"
	"net"
	"net/http"
	"testing"
	"time"

	"github.com/mwitkow/go-conntrack"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
)

func TestDialerWrapper(t *testing.T) {
	suite.Run(t, &DialerTestSuite{})
}

type DialerTestSuite struct {
	suite.Suite

	serverListener net.Listener
	httpServer     http.Server
}

func (s *DialerTestSuite) SetupSuite() {
	var err error
	s.serverListener, err = net.Listen("tcp", "127.0.0.1:0")
	require.NoError(s.T(), err, "must be able to allocate a port for serverListener")
	s.httpServer = http.Server{
		Handler: http.HandlerFunc(func(resp http.ResponseWriter, req *http.Request) {
			resp.WriteHeader(http.StatusOK)
		}),
	}
	go func() {
		s.httpServer.Serve(s.serverListener)
	}()
}

func (s *DialerTestSuite) TestDialerMetricsArePreregistered() {
	conntrack.NewDialContextFunc() // dialer name = default
	conntrack.NewDialContextFunc(conntrack.DialWithName("foobar"))
	conntrack.PreRegisterDialerMetrics("something_manual")
	for testId, testCase := range []struct {
		metricName     string
		existingLabels []string
	}{
		{"net_conntrack_dialer_conn_attempted_total", []string{"default"}},
		{"net_conntrack_dialer_conn_attempted_total", []string{"foobar"}},
		{"net_conntrack_dialer_conn_attempted_total", []string{"something_manual"}},
		{"net_conntrack_dialer_conn_closed_total", []string{"default"}},
		{"net_conntrack_dialer_conn_closed_total", []string{"foobar"}},
		{"net_conntrack_dialer_conn_closed_total", []string{"something_manual"}},
		{"net_conntrack_dialer_conn_established_total", []string{"default"}},
		{"net_conntrack_dialer_conn_established_total", []string{"foobar"}},
		{"net_conntrack_dialer_conn_established_total", []string{"something_manual"}},
		{"net_conntrack_dialer_conn_failed_total", []string{"default", "resolution"}},
		{"net_conntrack_dialer_conn_failed_total", []string{"default", "refused"}},
		{"net_conntrack_dialer_conn_failed_total", []string{"default", "timeout"}},
		{"net_conntrack_dialer_conn_failed_total", []string{"default", "unknown"}},
	} {
		lineCount := len(fetchPrometheusLines(s.T(), testCase.metricName, testCase.existingLabels...))
		assert.NotEqual(s.T(), 0, lineCount, "metrics must exist for test case %d", testId)
	}
}

func (s *DialerTestSuite) TestDialerMetricsAreNotPreregisteredWithMonitoringOff() {
	conntrack.NewDialContextFunc(conntrack.DialWithName("nomon"), conntrack.DialWithoutMonitoring())
	for testId, testCase := range []struct {
		metricName     string
		existingLabels []string
	}{
		{"net_conntrack_dialer_conn_attempted_total", []string{"nomon"}},
		{"net_conntrack_dialer_conn_closed_total", []string{"nomon"}},
		{"net_conntrack_dialer_conn_established_total", []string{"nomon"}},
		{"net_conntrack_dialer_conn_failed_total", []string{"nomon", "resolution"}},
		{"net_conntrack_dialer_conn_failed_total", []string{"nomon", "refused"}},
		{"net_conntrack_dialer_conn_failed_total", []string{"nomon", "timeout"}},
		{"net_conntrack_dialer_conn_failed_total", []string{"nomon", "unknown"}},
	} {
		lineCount := len(fetchPrometheusLines(s.T(), testCase.metricName, testCase.existingLabels...))
		assert.Equal(s.T(), 0, lineCount, "metrics should not be registered exist for test case %d", testId)
	}
}

func (s *DialerTestSuite) TestDialerUnderNormalConnection() {
	dialFunc := conntrack.NewDialContextFunc(conntrack.DialWithName("normal_conn"))

	beforeAttempts := sumCountersForMetricAndLabels(s.T(), "net_conntrack_dialer_conn_attempted_total", "normal_conn")
	beforeEstablished := sumCountersForMetricAndLabels(s.T(), "net_conntrack_dialer_conn_established_total", "normal_conn")
	beforeClosed := sumCountersForMetricAndLabels(s.T(), "net_conntrack_dialer_conn_closed_total", "normal_conn")

	conn, err := dialFunc(context.TODO(), "tcp", s.serverListener.Addr().String())
	require.NoError(s.T(), err, "NewDialContextFunc should successfully establish a conn here")
	assert.Equal(s.T(), beforeAttempts+1, sumCountersForMetricAndLabels(s.T(), "net_conntrack_dialer_conn_attempted_total", "normal_conn"),
		"the attempted conn counter must be incremented after connection was opened")
	assert.Equal(s.T(), beforeEstablished+1, sumCountersForMetricAndLabels(s.T(), "net_conntrack_dialer_conn_established_total", "normal_conn"),
		"the established conn counter must be incremented after connection was opened")
	assert.Equal(s.T(), beforeClosed, sumCountersForMetricAndLabels(s.T(), "net_conntrack_dialer_conn_closed_total", "normal_conn"),
		"the closed conn counter must not be incremented after connection was opened")
	conn.Close()
	assert.Equal(s.T(), beforeClosed+1, sumCountersForMetricAndLabels(s.T(), "net_conntrack_dialer_conn_established_total", "normal_conn"),
		"the closed conn counter must be incremented after connection was closed")
}

func (s *DialerTestSuite) TestDialerWithContextName() {
	dialFunc := conntrack.NewDialContextFunc()
	conntrack.PreRegisterDialerMetrics("ctx_conn")

	beforeAttempts := sumCountersForMetricAndLabels(s.T(), "net_conntrack_dialer_conn_attempted_total", "ctx_conn")
	beforeEstablished := sumCountersForMetricAndLabels(s.T(), "net_conntrack_dialer_conn_established_total", "ctx_conn")
	beforeClosed := sumCountersForMetricAndLabels(s.T(), "net_conntrack_dialer_conn_closed_total", "ctx_conn")

	conn, err := dialFunc(conntrack.DialNameToContext(context.TODO(), "ctx_conn"), "tcp", s.serverListener.Addr().String())
	require.NoError(s.T(), err, "NewDialContextFunc should successfully establish a conn here")
	assert.Equal(s.T(), beforeAttempts+1, sumCountersForMetricAndLabels(s.T(), "net_conntrack_dialer_conn_attempted_total", "ctx_conn"),
		"the attempted conn counter must be incremented after connection was opened")
	assert.Equal(s.T(), beforeEstablished+1, sumCountersForMetricAndLabels(s.T(), "net_conntrack_dialer_conn_established_total", "ctx_conn"),
		"the established conn counter must be incremented after connection was opened")
	assert.Equal(s.T(), beforeClosed, sumCountersForMetricAndLabels(s.T(), "net_conntrack_dialer_conn_closed_total", "ctx_conn"),
		"the closed conn counter must not be incremented after connection was opened")
	conn.Close()
	assert.Equal(s.T(), beforeClosed+1, sumCountersForMetricAndLabels(s.T(), "net_conntrack_dialer_conn_established_total", "ctx_conn"),
		"the closed conn counter must be incremented after connection was closed")
}

func (s *ListenerTestSuite) TestDialerTracingCapturedInPage() {
	dialFunc := conntrack.NewDialContextFunc(conntrack.DialWithTracing())
	dialerName := "some_dialer"
	conn, err := dialFunc(conntrack.DialNameToContext(context.TODO(), dialerName), "tcp", s.serverListener.Addr().String())
	time.Sleep(5 * time.Millisecond)
	require.NoError(s.T(), err, "DialContext should successfully establish a conn here")
	assert.Contains(s.T(), fetchTraceEvents(s.T(), "net.ClientConn."+dialerName), conn.LocalAddr().String(),
		"the /debug/trace/events page must contain the live connection")
	time.Sleep(5 * time.Millisecond)
	conn.Close()
}

func (s *DialerTestSuite) TestDialerResolutionFailure() {
	dialFunc := conntrack.NewDialContextFunc(conntrack.DialWithName("res_err"))

	beforeAttempts := sumCountersForMetricAndLabels(s.T(), "net_conntrack_dialer_conn_attempted_total", "res_err")
	beforeEstablished := sumCountersForMetricAndLabels(s.T(), "net_conntrack_dialer_conn_established_total", "res_err")
	beforeResolutionErrors := sumCountersForMetricAndLabels(s.T(), "net_conntrack_dialer_conn_failed_total", "res_err", "resolution")
	beforeClosed := sumCountersForMetricAndLabels(s.T(), "net_conntrack_dialer_conn_closed_total", "res_err")

	_, err := dialFunc(context.TODO(), "tcp", "dialer.test.wrong.domain.wrong:443")
	require.Error(s.T(), err, "NewDialContextFunc should fail here")
	assert.Equal(s.T(), beforeAttempts+1, sumCountersForMetricAndLabels(s.T(), "net_conntrack_dialer_conn_attempted_total", "res_err"),
		"the attempted conn counter must be incremented after connection was opened")
	assert.Equal(s.T(), beforeEstablished, sumCountersForMetricAndLabels(s.T(), "net_conntrack_dialer_conn_established_total", "res_err"),
		"the established conn counter must not be incremented on a failure")
	assert.Equal(s.T(), beforeClosed, sumCountersForMetricAndLabels(s.T(), "net_conntrack_dialer_conn_closed_total", "res_err"),
		"the closed conn counter must not be incremented on a failure")
	assert.Equal(s.T(), beforeResolutionErrors+1, sumCountersForMetricAndLabels(s.T(), "net_conntrack_dialer_conn_failed_total", "res_err", "resolution"),
		"the failure counter for resolution error should be incremented")
}

func (s *DialerTestSuite) TestDialerRefusedFailure() {
	dialFunc := conntrack.NewDialContextFunc(conntrack.DialWithName("ref_err"))

	beforeAttempts := sumCountersForMetricAndLabels(s.T(), "net_conntrack_dialer_conn_attempted_total", "ref_err")
	beforeEstablished := sumCountersForMetricAndLabels(s.T(), "net_conntrack_dialer_conn_established_total", "ref_err")
	beforeResolutionErrors := sumCountersForMetricAndLabels(s.T(), "net_conntrack_dialer_conn_failed_total", "ref_err", "resolution")
	beforeClosed := sumCountersForMetricAndLabels(s.T(), "net_conntrack_dialer_conn_closed_total", "ref_err")

	_, err := dialFunc(context.TODO(), "tcp", "127.0.0.1:337") // 337 is a cool port, let's hope its unused.
	require.Error(s.T(), err, "NewDialContextFunc should fail here")
	assert.Equal(s.T(), beforeAttempts+1, sumCountersForMetricAndLabels(s.T(), "net_conntrack_dialer_conn_attempted_total", "ref_err"),
		"the attempted conn counter must be incremented after connection was opened")
	assert.Equal(s.T(), beforeEstablished, sumCountersForMetricAndLabels(s.T(), "net_conntrack_dialer_conn_established_total", "ref_err"),
		"the established conn counter must not be incremented on a failure")
	assert.Equal(s.T(), beforeClosed, sumCountersForMetricAndLabels(s.T(), "net_conntrack_dialer_conn_closed_total", "ref_err"),
		"the closed conn counter must not be incremented on a failure")
	assert.Equal(s.T(), beforeResolutionErrors+1, sumCountersForMetricAndLabels(s.T(), "net_conntrack_dialer_conn_failed_total", "ref_err", "refused"),
		"the failure counter for connection refused error should be incremented")
}

func (s *DialerTestSuite) TearDownSuite() {
	if s.serverListener != nil {
		s.T().Logf("stopped http.Server at: %v", s.serverListener.Addr().String())
		s.serverListener.Close()
	}
}
