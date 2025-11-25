// Copyright 2017 The Prometheus Authors
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
	"errors"
	"fmt"
	"io"
	"math"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"testing"
	"time"

	"github.com/alecthomas/kingpin/v2"
	remoteapi "github.com/prometheus/client_golang/exp/api/remote"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/common/expfmt"
	"github.com/prometheus/common/model"
	"github.com/prometheus/common/promslog"
	"github.com/stretchr/testify/require"

	"github.com/prometheus/prometheus/model/labels"
	"github.com/prometheus/prometheus/notifier"
	"github.com/prometheus/prometheus/rules"
	"github.com/prometheus/prometheus/util/testutil"
)

func init() {
	// This can be removed when the legacy global mode is fully deprecated.
	//nolint:staticcheck
	model.NameValidationScheme = model.UTF8Validation
}

const startupTime = 10 * time.Second

var (
	promPath    = os.Args[0]
	promConfig  = filepath.Join("..", "..", "documentation", "examples", "prometheus.yml")
	agentConfig = filepath.Join("..", "..", "documentation", "examples", "prometheus-agent.yml")
)

func TestMain(m *testing.M) {
	for i, arg := range os.Args {
		if arg == "-test.main" {
			os.Args = append(os.Args[:i], os.Args[i+1:]...)
			main()
			return
		}
	}

	// On linux with a global proxy the tests will fail as the go client(http,grpc) tries to connect through the proxy.
	os.Setenv("no_proxy", "localhost,127.0.0.1,0.0.0.0,:")

	exitCode := m.Run()
	os.Exit(exitCode)
}

func TestComputeExternalURL(t *testing.T) {
	tests := []struct {
		input string
		valid bool
	}{
		{
			input: "",
			valid: true,
		},
		{
			input: "http://proxy.com/prometheus",
			valid: true,
		},
		{
			input: "'https://url/prometheus'",
			valid: false,
		},
		{
			input: "'relative/path/with/quotes'",
			valid: false,
		},
		{
			input: "http://alertmanager.company.com",
			valid: true,
		},
		{
			input: "https://double--dash.de",
			valid: true,
		},
		{
			input: "'http://starts/with/quote",
			valid: false,
		},
		{
			input: "ends/with/quote\"",
			valid: false,
		},
	}

	for _, test := range tests {
		_, err := computeExternalURL(test.input, "0.0.0.0:9090")
		if test.valid {
			require.NoError(t, err)
		} else {
			require.Error(t, err, "input=%q", test.input)
		}
	}
}

// Let's provide an invalid configuration file and verify the exit status indicates the error.
func TestFailedStartupExitCode(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping test in short mode.")
	}
	t.Parallel()

	fakeInputFile := "fake-input-file"
	expectedExitStatus := 2

	prom := exec.Command(promPath, "-test.main", "--web.listen-address=0.0.0.0:0", "--config.file="+fakeInputFile)
	err := prom.Run()
	require.Error(t, err)

	var exitError *exec.ExitError
	require.ErrorAs(t, err, &exitError)
	status := exitError.Sys().(syscall.WaitStatus)
	require.Equal(t, expectedExitStatus, status.ExitStatus())
}

type senderFunc func(alerts ...*notifier.Alert)

func (s senderFunc) Send(alerts ...*notifier.Alert) {
	s(alerts...)
}

func TestSendAlerts(t *testing.T) {
	testCases := []struct {
		in  []*rules.Alert
		exp []*notifier.Alert
	}{
		{
			in: []*rules.Alert{
				{
					Labels:      labels.FromStrings("l1", "v1"),
					Annotations: labels.FromStrings("a2", "v2"),
					ActiveAt:    time.Unix(1, 0),
					FiredAt:     time.Unix(2, 0),
					ValidUntil:  time.Unix(3, 0),
				},
			},
			exp: []*notifier.Alert{
				{
					Labels:       labels.FromStrings("l1", "v1"),
					Annotations:  labels.FromStrings("a2", "v2"),
					StartsAt:     time.Unix(2, 0),
					EndsAt:       time.Unix(3, 0),
					GeneratorURL: "http://localhost:9090/graph?g0.expr=up&g0.tab=1",
				},
			},
		},
		{
			in: []*rules.Alert{
				{
					Labels:      labels.FromStrings("l1", "v1"),
					Annotations: labels.FromStrings("a2", "v2"),
					ActiveAt:    time.Unix(1, 0),
					FiredAt:     time.Unix(2, 0),
					ResolvedAt:  time.Unix(4, 0),
				},
			},
			exp: []*notifier.Alert{
				{
					Labels:       labels.FromStrings("l1", "v1"),
					Annotations:  labels.FromStrings("a2", "v2"),
					StartsAt:     time.Unix(2, 0),
					EndsAt:       time.Unix(4, 0),
					GeneratorURL: "http://localhost:9090/graph?g0.expr=up&g0.tab=1",
				},
			},
		},
		{
			in: []*rules.Alert{},
		},
	}

	for i, tc := range testCases {
		t.Run(strconv.Itoa(i), func(t *testing.T) {
			senderFunc := senderFunc(func(alerts ...*notifier.Alert) {
				require.NotEmpty(t, tc.in, "sender called with 0 alert")
				require.Equal(t, tc.exp, alerts)
			})
			rules.SendAlerts(senderFunc, "http://localhost:9090")(context.TODO(), "up", tc.in...)
		})
	}
}

func TestWALSegmentSizeBounds(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping test in short mode.")
	}
	t.Parallel()

	for _, tc := range []struct {
		size     string
		exitCode int
	}{
		{
			size:     "9MB",
			exitCode: 1,
		},
		{
			size:     "257MB",
			exitCode: 1,
		},
		{
			size:     "10",
			exitCode: 2,
		},
		{
			size:     "1GB",
			exitCode: 1,
		},
		{
			size:     "12MB",
			exitCode: 0,
		},
	} {
		t.Run(tc.size, func(t *testing.T) {
			t.Parallel()
			prom := exec.Command(promPath, "-test.main", "--storage.tsdb.wal-segment-size="+tc.size, "--web.listen-address=0.0.0.0:0", "--config.file="+promConfig, "--storage.tsdb.path="+filepath.Join(t.TempDir(), "data"))

			// Log stderr in case of failure.
			stderr, err := prom.StderrPipe()
			require.NoError(t, err)

			// WaitGroup is used to ensure that we don't call t.Log() after the test has finished.
			var wg sync.WaitGroup
			wg.Add(1)
			defer wg.Wait()

			go func() {
				defer wg.Done()
				slurp, _ := io.ReadAll(stderr)
				t.Log(string(slurp))
			}()

			err = prom.Start()
			require.NoError(t, err)

			if tc.exitCode == 0 {
				done := make(chan error, 1)
				go func() { done <- prom.Wait() }()
				select {
				case err := <-done:
					t.Fatalf("prometheus should be still running: %v", err)
				case <-time.After(startupTime):
					prom.Process.Kill()
					<-done
				}
				return
			}

			err = prom.Wait()
			require.Error(t, err)
			var exitError *exec.ExitError
			require.ErrorAs(t, err, &exitError)
			status := exitError.Sys().(syscall.WaitStatus)
			require.Equal(t, tc.exitCode, status.ExitStatus())
		})
	}
}

func TestMaxBlockChunkSegmentSizeBounds(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping test in short mode.")
	}
	t.Parallel()

	for _, tc := range []struct {
		size     string
		exitCode int
	}{
		{
			size:     "512KB",
			exitCode: 1,
		},
		{
			size:     "1MB",
			exitCode: 0,
		},
	} {
		t.Run(tc.size, func(t *testing.T) {
			t.Parallel()
			prom := exec.Command(promPath, "-test.main", "--storage.tsdb.max-block-chunk-segment-size="+tc.size, "--web.listen-address=0.0.0.0:0", "--config.file="+promConfig, "--storage.tsdb.path="+filepath.Join(t.TempDir(), "data"))

			// Log stderr in case of failure.
			stderr, err := prom.StderrPipe()
			require.NoError(t, err)

			// WaitGroup is used to ensure that we don't call t.Log() after the test has finished.
			var wg sync.WaitGroup
			wg.Add(1)
			defer wg.Wait()

			go func() {
				defer wg.Done()
				slurp, _ := io.ReadAll(stderr)
				t.Log(string(slurp))
			}()

			err = prom.Start()
			require.NoError(t, err)

			if tc.exitCode == 0 {
				done := make(chan error, 1)
				go func() { done <- prom.Wait() }()
				select {
				case err := <-done:
					t.Fatalf("prometheus should be still running: %v", err)
				case <-time.After(startupTime):
					prom.Process.Kill()
					<-done
				}
				return
			}

			err = prom.Wait()
			require.Error(t, err)
			var exitError *exec.ExitError
			require.ErrorAs(t, err, &exitError)
			status := exitError.Sys().(syscall.WaitStatus)
			require.Equal(t, tc.exitCode, status.ExitStatus())
		})
	}
}

func TestTimeMetrics(t *testing.T) {
	tmpDir := t.TempDir()

	reg := prometheus.NewRegistry()
	db, err := openDBWithMetrics(tmpDir, promslog.NewNopLogger(), reg, nil, nil)
	require.NoError(t, err)
	defer func() {
		require.NoError(t, db.Close())
	}()

	// Check initial values.
	require.Equal(t, map[string]float64{
		"prometheus_tsdb_lowest_timestamp_seconds": float64(math.MaxInt64) / 1000,
		"prometheus_tsdb_head_min_time_seconds":    float64(math.MaxInt64) / 1000,
		"prometheus_tsdb_head_max_time_seconds":    float64(math.MinInt64) / 1000,
	}, getCurrentGaugeValuesFor(t, reg,
		"prometheus_tsdb_lowest_timestamp_seconds",
		"prometheus_tsdb_head_min_time_seconds",
		"prometheus_tsdb_head_max_time_seconds",
	))

	app := db.Appender(context.Background())
	_, err = app.Append(0, labels.FromStrings(model.MetricNameLabel, "a"), 1000, 1)
	require.NoError(t, err)
	_, err = app.Append(0, labels.FromStrings(model.MetricNameLabel, "a"), 2000, 1)
	require.NoError(t, err)
	_, err = app.Append(0, labels.FromStrings(model.MetricNameLabel, "a"), 3000, 1)
	require.NoError(t, err)
	require.NoError(t, app.Commit())

	require.Equal(t, map[string]float64{
		"prometheus_tsdb_lowest_timestamp_seconds": 1.0,
		"prometheus_tsdb_head_min_time_seconds":    1.0,
		"prometheus_tsdb_head_max_time_seconds":    3.0,
	}, getCurrentGaugeValuesFor(t, reg,
		"prometheus_tsdb_lowest_timestamp_seconds",
		"prometheus_tsdb_head_min_time_seconds",
		"prometheus_tsdb_head_max_time_seconds",
	))
}

func getCurrentGaugeValuesFor(t *testing.T, reg prometheus.Gatherer, metricNames ...string) map[string]float64 {
	f, err := reg.Gather()
	require.NoError(t, err)

	res := make(map[string]float64, len(metricNames))
	for _, g := range f {
		for _, m := range metricNames {
			if g.GetName() != m {
				continue
			}

			require.Len(t, g.GetMetric(), 1)
			_, ok := res[m]
			require.False(t, ok, "expected only one metric family for", m)
			res[m] = *g.GetMetric()[0].GetGauge().Value
		}
	}
	return res
}

func TestAgentSuccessfulStartup(t *testing.T) {
	t.Parallel()

	prom := exec.Command(promPath, "-test.main", "--agent", "--web.listen-address=0.0.0.0:0", "--config.file="+agentConfig)
	require.NoError(t, prom.Start())

	actualExitStatus := 0
	done := make(chan error, 1)

	go func() { done <- prom.Wait() }()
	select {
	case err := <-done:
		t.Logf("prometheus agent should be still running: %v", err)
		actualExitStatus = prom.ProcessState.ExitCode()
	case <-time.After(startupTime):
		prom.Process.Kill()
	}
	require.Equal(t, 0, actualExitStatus)
}

func TestAgentFailedStartupWithServerFlag(t *testing.T) {
	t.Parallel()

	prom := exec.Command(promPath, "-test.main", "--agent", "--storage.tsdb.path=.", "--web.listen-address=0.0.0.0:0", "--config.file="+promConfig)

	output := bytes.Buffer{}
	prom.Stderr = &output
	require.NoError(t, prom.Start())

	actualExitStatus := 0
	done := make(chan error, 1)

	go func() { done <- prom.Wait() }()
	select {
	case err := <-done:
		t.Logf("prometheus agent should not be running: %v", err)
		actualExitStatus = prom.ProcessState.ExitCode()
	case <-time.After(startupTime):
		prom.Process.Kill()
	}

	require.Equal(t, 3, actualExitStatus)

	// Assert on last line.
	lines := strings.Split(output.String(), "\n")
	last := lines[len(lines)-1]
	require.Equal(t, "The following flag(s) can not be used in agent mode: [\"--storage.tsdb.path\"]", last)
}

func TestAgentFailedStartupWithInvalidConfig(t *testing.T) {
	t.Parallel()

	prom := exec.Command(promPath, "-test.main", "--agent", "--web.listen-address=0.0.0.0:0", "--config.file="+promConfig)
	require.NoError(t, prom.Start())

	actualExitStatus := 0
	done := make(chan error, 1)

	go func() { done <- prom.Wait() }()
	select {
	case err := <-done:
		t.Logf("prometheus agent should not be running: %v", err)
		actualExitStatus = prom.ProcessState.ExitCode()
	case <-time.After(startupTime):
		prom.Process.Kill()
	}
	require.Equal(t, 2, actualExitStatus)
}

func TestModeSpecificFlags(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping test in short mode.")
	}
	t.Parallel()

	testcases := []struct {
		mode       string
		arg        string
		exitStatus int
	}{
		{"agent", "--storage.agent.path", 0},
		{"server", "--storage.tsdb.path", 0},
		{"server", "--storage.agent.path", 3},
		{"agent", "--storage.tsdb.path", 3},
	}

	for _, tc := range testcases {
		t.Run(fmt.Sprintf("%s mode with option %s", tc.mode, tc.arg), func(t *testing.T) {
			t.Parallel()
			args := []string{"-test.main", tc.arg, t.TempDir(), "--web.listen-address=0.0.0.0:0"}

			if tc.mode == "agent" {
				args = append(args, "--agent", "--config.file="+agentConfig)
			} else {
				args = append(args, "--config.file="+promConfig)
			}

			prom := exec.Command(promPath, args...)

			// Log stderr in case of failure.
			stderr, err := prom.StderrPipe()
			require.NoError(t, err)

			// WaitGroup is used to ensure that we don't call t.Log() after the test has finished.
			var wg sync.WaitGroup
			wg.Add(1)
			defer wg.Wait()

			go func() {
				defer wg.Done()
				slurp, _ := io.ReadAll(stderr)
				t.Log(string(slurp))
			}()

			err = prom.Start()
			require.NoError(t, err)

			if tc.exitStatus == 0 {
				done := make(chan error, 1)
				go func() { done <- prom.Wait() }()
				select {
				case err := <-done:
					t.Errorf("prometheus should be still running: %v", err)
				case <-time.After(startupTime):
					prom.Process.Kill()
					<-done
				}
				return
			}

			err = prom.Wait()
			require.Error(t, err)
			var exitError *exec.ExitError
			if errors.As(err, &exitError) {
				status := exitError.Sys().(syscall.WaitStatus)
				require.Equal(t, tc.exitStatus, status.ExitStatus())
			} else {
				t.Errorf("unable to retrieve the exit status for prometheus: %v", err)
			}
		})
	}
}

func TestDocumentation(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.SkipNow()
	}
	t.Parallel()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, promPath, "-test.main", "--write-documentation")

	var stdout bytes.Buffer
	cmd.Stdout = &stdout

	if err := cmd.Run(); err != nil {
		var exitError *exec.ExitError
		if errors.As(err, &exitError) && exitError.ExitCode() != 0 {
			fmt.Println("Command failed with non-zero exit code")
		}
	}

	generatedContent := strings.ReplaceAll(stdout.String(), filepath.Base(promPath), strings.TrimSuffix(filepath.Base(promPath), ".test"))

	expectedContent, err := os.ReadFile(filepath.Join("..", "..", "docs", "command-line", "prometheus.md"))
	require.NoError(t, err)

	require.Equal(t, string(expectedContent), generatedContent, "Generated content does not match documentation. Hint: run `make cli-documentation`.")
}

func TestRwProtoMsgFlagParser(t *testing.T) {
	t.Parallel()

	defaultOpts := remoteapi.MessageTypes{
		remoteapi.WriteV1MessageType, remoteapi.WriteV2MessageType,
	}

	for _, tcase := range []struct {
		args        []string
		expected    remoteapi.MessageTypes
		expectedErr error
	}{
		{
			args:     nil,
			expected: defaultOpts,
		},
		{
			args:        []string{"--test-proto-msgs", "test"},
			expectedErr: errors.New("unknown type for remote write protobuf message test, supported: prometheus.WriteRequest, io.prometheus.write.v2.Request"),
		},
		{
			args:     []string{"--test-proto-msgs", "io.prometheus.write.v2.Request"},
			expected: remoteapi.MessageTypes{remoteapi.WriteV2MessageType},
		},
		{
			args: []string{
				"--test-proto-msgs", "io.prometheus.write.v2.Request",
				"--test-proto-msgs", "io.prometheus.write.v2.Request",
			},
			expectedErr: errors.New("duplicated io.prometheus.write.v2.Request flag value, got io.prometheus.write.v2.Request already"),
		},
		{
			args: []string{
				"--test-proto-msgs", "io.prometheus.write.v2.Request",
				"--test-proto-msgs", "prometheus.WriteRequest",
			},
			expected: remoteapi.MessageTypes{remoteapi.WriteV2MessageType, remoteapi.WriteV1MessageType},
		},
		{
			args: []string{
				"--test-proto-msgs", "io.prometheus.write.v2.Request",
				"--test-proto-msgs", "prometheus.WriteRequest",
				"--test-proto-msgs", "io.prometheus.write.v2.Request",
			},
			expectedErr: errors.New("duplicated io.prometheus.write.v2.Request flag value, got io.prometheus.write.v2.Request, prometheus.WriteRequest already"),
		},
	} {
		t.Run(strings.Join(tcase.args, ","), func(t *testing.T) {
			a := kingpin.New("test", "")
			var opt remoteapi.MessageTypes
			a.Flag("test-proto-msgs", "").Default(defaultOpts.Strings()...).SetValue(rwProtoMsgFlagValue(&opt))

			_, err := a.Parse(tcase.args)
			if tcase.expectedErr != nil {
				require.Error(t, err)
				require.Equal(t, tcase.expectedErr, err)
			} else {
				require.NoError(t, err)
				require.Equal(t, tcase.expected, opt)
			}
		})
	}
}

// reloadPrometheusConfig sends a reload request to the Prometheus server to apply
// updated configurations.
func reloadPrometheusConfig(t *testing.T, reloadURL string) {
	t.Helper()

	r, err := http.Post(reloadURL, "text/plain", nil)
	require.NoError(t, err, "Failed to reload Prometheus")
	require.Equal(t, http.StatusOK, r.StatusCode, "Unexpected status code when reloading Prometheus")
}

func getMetricValue(t *testing.T, body io.Reader, metricType model.MetricType, metricName string) (float64, error) {
	t.Helper()

	p := expfmt.NewTextParser(model.UTF8Validation)
	metricFamilies, err := p.TextToMetricFamilies(body)
	if err != nil {
		return 0, err
	}
	metricFamily, ok := metricFamilies[metricName]
	if !ok {
		return 0, errors.New("metric family not found")
	}
	metric := metricFamily.GetMetric()
	if len(metric) != 1 {
		return 0, errors.New("metric not found")
	}
	switch metricType {
	case model.MetricTypeGauge:
		return metric[0].GetGauge().GetValue(), nil
	case model.MetricTypeCounter:
		return metric[0].GetCounter().GetValue(), nil
	default:
		t.Fatalf("metric type %s not supported", metricType)
	}

	return 0, errors.New("cannot get value")
}

func TestRuntimeGOGCConfig(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping test in short mode.")
	}
	t.Parallel()

	for _, tc := range []struct {
		name         string
		config       string
		gogcEnvVar   string
		expectedGOGC float64
	}{
		{
			name:         "empty config file",
			expectedGOGC: 75,
		},
		{
			name:         "empty config file with GOGC env var set",
			gogcEnvVar:   "66",
			expectedGOGC: 66,
		},
		{
			name: "gogc set through config",
			config: `
runtime:
  gogc: 77`,
			expectedGOGC: 77.0,
		},
		{
			name: "gogc set through config and env var",
			config: `
runtime:
  gogc: 77`,
			gogcEnvVar:   "88",
			expectedGOGC: 77.0,
		},
		{
			name: "incomplete runtime block",
			config: `
runtime:`,
			expectedGOGC: 75.0,
		},
		{
			name: "incomplete runtime block and GOGC env var set",
			config: `
runtime:`,
			gogcEnvVar:   "88",
			expectedGOGC: 88.0,
		},
		{
			name: "unrelated config and GOGC env var set",
			config: `
global:
  scrape_interval: 500ms`,
			gogcEnvVar:   "80",
			expectedGOGC: 80,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			tmpDir := t.TempDir()
			configFile := filepath.Join(tmpDir, "prometheus.yml")

			port := testutil.RandomUnprivilegedPort(t)
			os.WriteFile(configFile, []byte(tc.config), 0o777)
			prom := prometheusCommandWithLogging(
				t,
				configFile,
				port,
				fmt.Sprintf("--storage.tsdb.path=%s", tmpDir),
				"--web.enable-lifecycle",
			)
			// Inject GOGC when set.
			prom.Env = os.Environ()
			if tc.gogcEnvVar != "" {
				prom.Env = append(prom.Env, fmt.Sprintf("GOGC=%s", tc.gogcEnvVar))
			}
			require.NoError(t, prom.Start())

			ensureGOGCValue := func(val float64) {
				var (
					r   *http.Response
					err error
				)
				// Wait for the /metrics endpoint to be ready.
				require.Eventually(t, func() bool {
					r, err = http.Get(fmt.Sprintf("http://127.0.0.1:%d/metrics", port))
					if err != nil {
						return false
					}
					return r.StatusCode == http.StatusOK
				}, 5*time.Second, 50*time.Millisecond)
				defer r.Body.Close()

				// Check the final GOGC that's set, consider go_gc_gogc_percent from /metrics as source of truth.
				gogc, err := getMetricValue(t, r.Body, model.MetricTypeGauge, "go_gc_gogc_percent")
				require.NoError(t, err)
				require.Equal(t, val, gogc)
			}

			// The value is applied on startup.
			ensureGOGCValue(tc.expectedGOGC)

			// After a reload with the same config, the value stays the same.
			reloadURL := fmt.Sprintf("http://127.0.0.1:%d/-/reload", port)
			reloadPrometheusConfig(t, reloadURL)
			ensureGOGCValue(tc.expectedGOGC)

			// After a reload with different config, the value gets updated.
			newConfig := `
runtime:
  gogc: 99`
			os.WriteFile(configFile, []byte(newConfig), 0o777)
			reloadPrometheusConfig(t, reloadURL)
			ensureGOGCValue(99.0)
		})
	}
}

// TestHeadCompactionWhileScraping verifies that running a head compaction
// concurrently with a scrape does not trigger the data race described in
// https://github.com/prometheus/prometheus/issues/16490.
func TestHeadCompactionWhileScraping(t *testing.T) {
	t.Parallel()

	// To increase the chance of reproducing the data race
	for i := range 5 {
		t.Run(strconv.Itoa(i), func(t *testing.T) {
			t.Parallel()

			tmpDir := t.TempDir()
			configFile := filepath.Join(tmpDir, "prometheus.yml")

			port := testutil.RandomUnprivilegedPort(t)
			config := fmt.Sprintf(`
scrape_configs:
  - job_name: 'self1'
    scrape_interval: 61ms
    static_configs:
      - targets: ['localhost:%d']
  - job_name: 'self2'
    scrape_interval: 67ms
    static_configs:
      - targets: ['localhost:%d']
`, port, port)
			os.WriteFile(configFile, []byte(config), 0o777)

			prom := prometheusCommandWithLogging(
				t,
				configFile,
				port,
				fmt.Sprintf("--storage.tsdb.path=%s", tmpDir),
				"--storage.tsdb.min-block-duration=100ms",
			)
			require.NoError(t, prom.Start())

			require.Eventually(t, func() bool {
				r, err := http.Get(fmt.Sprintf("http://127.0.0.1:%d/metrics", port))
				if err != nil {
					return false
				}
				defer r.Body.Close()
				if r.StatusCode != http.StatusOK {
					return false
				}
				metrics, err := io.ReadAll(r.Body)
				if err != nil {
					return false
				}

				// Wait for some compactions to run
				compactions, err := getMetricValue(t, bytes.NewReader(metrics), model.MetricTypeCounter, "prometheus_tsdb_compactions_total")
				if err != nil {
					return false
				}
				if compactions < 3 {
					return false
				}

				// Sanity check: Some actual scraping was done.
				series, err := getMetricValue(t, bytes.NewReader(metrics), model.MetricTypeCounter, "prometheus_tsdb_head_series_created_total")
				require.NoError(t, err)
				require.NotZero(t, series)

				// No compaction must have failed
				failures, err := getMetricValue(t, bytes.NewReader(metrics), model.MetricTypeCounter, "prometheus_tsdb_compactions_failed_total")
				require.NoError(t, err)
				require.Zero(t, failures)
				return true
			}, 15*time.Second, 500*time.Millisecond)
		})
	}
}

// This test verifies that metrics for the highest timestamps per queue account for relabelling.
// See: https://github.com/prometheus/prometheus/pull/17065.
func TestRemoteWrite_PerQueueMetricsAfterRelabeling(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	configFile := filepath.Join(tmpDir, "prometheus.yml")

	port := testutil.RandomUnprivilegedPort(t)
	targetPort := testutil.RandomUnprivilegedPort(t)

	server := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		panic("should never be reached")
	}))
	t.Cleanup(server.Close)

	// Simulate a remote write relabeling that doesn't yield any series.
	config := fmt.Sprintf(`
global:
  scrape_interval: 1s
scrape_configs:
  - job_name: 'self'
    static_configs:
      - targets: ['localhost:%d']
  - job_name: 'target'
    static_configs:
      - targets: ['localhost:%d']

remote_write:
  - url: %s
    write_relabel_configs:
      - source_labels: [job,__name__]
        regex: 'target,special_metric'
        action: keep
`, port, targetPort, server.URL)
	require.NoError(t, os.WriteFile(configFile, []byte(config), 0o777))

	prom := prometheusCommandWithLogging(
		t,
		configFile,
		port,
		fmt.Sprintf("--storage.tsdb.path=%s", tmpDir),
	)
	require.NoError(t, prom.Start())

	require.Eventually(t, func() bool {
		r, err := http.Get(fmt.Sprintf("http://127.0.0.1:%d/metrics", port))
		if err != nil {
			return false
		}
		defer r.Body.Close()
		if r.StatusCode != http.StatusOK {
			return false
		}

		metrics, err := io.ReadAll(r.Body)
		if err != nil {
			return false
		}

		gHighestTimestamp, err := getMetricValue(t, bytes.NewReader(metrics), model.MetricTypeGauge, "prometheus_remote_storage_highest_timestamp_in_seconds")
		// The highest timestamp at storage level sees all samples, it should also consider the ones that are filtered out by relabeling.
		if err != nil || gHighestTimestamp == 0 {
			return false
		}

		// The queue shouldn't see and send any sample, all samples are dropped due to relabeling, the metrics should reflect that.
		droppedSamples, err := getMetricValue(t, bytes.NewReader(metrics), model.MetricTypeCounter, "prometheus_remote_storage_samples_dropped_total")
		if err != nil || droppedSamples == 0 {
			return false
		}

		highestTimestamp, err := getMetricValue(t, bytes.NewReader(metrics), model.MetricTypeGauge, "prometheus_remote_storage_queue_highest_timestamp_seconds")
		require.NoError(t, err)
		require.Zero(t, highestTimestamp)

		highestSentTimestamp, err := getMetricValue(t, bytes.NewReader(metrics), model.MetricTypeGauge, "prometheus_remote_storage_queue_highest_sent_timestamp_seconds")
		require.NoError(t, err)
		require.Zero(t, highestSentTimestamp)
		return true
	}, 10*time.Second, 100*time.Millisecond)
}

// TestRemoteWrite_ReshardingWithoutDeadlock ensures that resharding (scaling up) doesn't block when the shards are full.
// See: https://github.com/prometheus/prometheus/issues/17384.
//
// The following shows key resharding metrics before and after the fix.
// In v3.7.0, the deadlock prevented the resharding logic from observing the true incoming data rate.
//
// | Metric              | v3.7.0        | after the fix       |
// |---------------------|---------------|---------------------|
// | dataInRate          | 0.6           | 307.2               |
// | dataPendingRate     | 0.2           | 306.8               |
// | dataPending         | 0             | 1228.8              |
// | desiredShards       | 0.6           | 369.2               |.
func TestRemoteWrite_ReshardingWithoutDeadlock(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	configFile := filepath.Join(tmpDir, "prometheus.yml")

	port := testutil.RandomUnprivilegedPort(t)

	server := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		time.Sleep(time.Second)
	}))
	t.Cleanup(server.Close)

	config := fmt.Sprintf(`
global:
  # Using a smaller interval may cause the scrape to time out.
  scrape_interval: 1s  
scrape_configs:
  - job_name: 'self'
    static_configs:
      - targets: ['localhost:%d']

remote_write:
  - url: %s
    queue_config:
      # Speed up the queue being full.
      capacity: 1
      # Helps keep the “time to send one sample” low so it doesn’t influence the resharding logic.
      max_samples_per_send: 1
`, port, server.URL)
	require.NoError(t, os.WriteFile(configFile, []byte(config), 0o777))

	prom := prometheusCommandWithLogging(
		t,
		configFile,
		port,
		fmt.Sprintf("--storage.tsdb.path=%s", tmpDir),
		"--log.level=debug",
	)
	require.NoError(t, prom.Start())

	const desiredShardsMetric = "prometheus_remote_storage_shards_desired"
	getMetrics := func() ([]byte, error) {
		r, err := http.Get(fmt.Sprintf("http://127.0.0.1:%d/metrics", port))
		if err != nil {
			return nil, err
		}
		defer r.Body.Close()
		if r.StatusCode != http.StatusOK {
			return nil, fmt.Errorf("unexpected status code: %d", r.StatusCode)
		}

		metrics, err := io.ReadAll(r.Body)
		if err != nil {
			return nil, err
		}
		return metrics, nil
	}

	// Ensure the initial desired shards is 1.
	require.Eventually(t, func() bool {
		metrics, err := getMetrics()
		if err != nil {
			return false
		}
		initialDesiredShards, err := getMetricValue(t, bytes.NewReader(metrics), model.MetricTypeGauge, desiredShardsMetric)
		if err != nil {
			return false
		}
		return initialDesiredShards == 1.0
	}, 10*time.Second, 100*time.Millisecond)

	// Ensure scaling up is triggered after some time.
	require.Eventually(t, func() bool {
		metrics, err := getMetrics()
		if err != nil {
			return false
		}
		desiredShards, err := getMetricValue(t, bytes.NewReader(metrics), model.MetricTypeGauge, desiredShardsMetric)
		if err != nil || desiredShards <= 1.0 {
			return false
		}
		return true
		// 3*shardUpdateDuration to allow for the resharding logic to run.
	}, 30*time.Second, time.Second)
}
