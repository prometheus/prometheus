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
	"flag"
	"fmt"
	"io"
	"math"
	"net/http"
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
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/common/expfmt"
	"github.com/prometheus/common/model"
	"github.com/prometheus/common/promslog"
	"github.com/stretchr/testify/require"
	"go.uber.org/atomic"

	"github.com/prometheus/prometheus/config"
	"github.com/prometheus/prometheus/model/labels"
	"github.com/prometheus/prometheus/notifier"
	"github.com/prometheus/prometheus/rules"
	"github.com/prometheus/prometheus/tsdb/fileutil"
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
		tc := tc
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

	defaultOpts := config.RemoteWriteProtoMsgs{
		config.RemoteWriteProtoMsgV1, config.RemoteWriteProtoMsgV2,
	}

	for _, tcase := range []struct {
		args        []string
		expected    []config.RemoteWriteProtoMsg
		expectedErr error
	}{
		{
			args:     nil,
			expected: defaultOpts,
		},
		{
			args:        []string{"--test-proto-msgs", "test"},
			expectedErr: errors.New("unknown remote write protobuf message test, supported: prometheus.WriteRequest, io.prometheus.write.v2.Request"),
		},
		{
			args:     []string{"--test-proto-msgs", "io.prometheus.write.v2.Request"},
			expected: config.RemoteWriteProtoMsgs{config.RemoteWriteProtoMsgV2},
		},
		{
			args: []string{
				"--test-proto-msgs", "io.prometheus.write.v2.Request",
				"--test-proto-msgs", "io.prometheus.write.v2.Request",
			},
			expectedErr: errors.New("duplicated io.prometheus.write.v2.Request flag value, got [io.prometheus.write.v2.Request] already"),
		},
		{
			args: []string{
				"--test-proto-msgs", "io.prometheus.write.v2.Request",
				"--test-proto-msgs", "prometheus.WriteRequest",
			},
			expected: config.RemoteWriteProtoMsgs{config.RemoteWriteProtoMsgV2, config.RemoteWriteProtoMsgV1},
		},
		{
			args: []string{
				"--test-proto-msgs", "io.prometheus.write.v2.Request",
				"--test-proto-msgs", "prometheus.WriteRequest",
				"--test-proto-msgs", "io.prometheus.write.v2.Request",
			},
			expectedErr: errors.New("duplicated io.prometheus.write.v2.Request flag value, got [io.prometheus.write.v2.Request prometheus.WriteRequest] already"),
		},
	} {
		t.Run(strings.Join(tcase.args, ","), func(t *testing.T) {
			a := kingpin.New("test", "")
			var opt []config.RemoteWriteProtoMsg
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

func eventuallyGetMetricValue(t *testing.T, metricsURL string, metricType model.MetricType, metricName string) (float64, error) {
	t.Helper()

	var (
		r   *http.Response
		err error
	)
	require.Eventually(t, func() bool {
		r, err = http.Get(metricsURL)
		if err != nil {
			return false
		}
		return r.StatusCode == http.StatusOK
	}, 5*time.Second, 100*time.Millisecond)
	defer r.Body.Close()

	p := expfmt.TextParser{}
	metricFamilies, err := p.TextToMetricFamilies(r.Body)
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

	return metric[0].GetGauge().GetValue(), nil
}

var sizeRestrictedFSAvailable = flag.Bool("test.size-restricted-fs", false, "run tests that require size restricted FS")

// TestNoSpaceLeftFailureRecovery uses the size-restricted filesystems in /mnt/test_prom_size_restricted_fs
// to reproduce an ENOSPC failure during, it then frees some space and ensures
// that things retun to normal and that the already produced WAL isn't corrupted.
// Issues such as https://github.com/prometheus/prometheus/issues/13027 have suggested that the WAL
// becomes corrupted when the disk is full. This test will help give additional attention to such cases.
// and help provide an environement to learn more about them.
// Note: Update the test invocation if the test name is changed.
func TestSizeRestrictedFS_NoSpaceLeftFailureRecovery(t *testing.T) {
	const fsRoot = "/mnt/test_prom_size_restricted_fs"
	if !*sizeRestrictedFSAvailable {
		t.Skip("test requires size restricted FS")
	}

	for i := range 100 {
		t.Run(fmt.Sprintf("fs_%d", i), func(t *testing.T) {
			t.Parallel()

			root := filepath.Join(fsRoot, fmt.Sprintf("fs_%d", i), "tmp")
			require.NoError(t, os.RemoveAll(root))
			require.NoError(t, os.MkdirAll(root, 0o777))

			// Reserve some space in the FS.
			placeholder := filepath.Join(root, "placeholder")
			f, err := os.OpenFile(placeholder, os.O_WRONLY|os.O_CREATE, 0o777)
			require.NoError(t, err)
			err = fileutil.Preallocate(f, 2*1024*1024, false)
			require.NoError(t, err)
			require.NoError(t, f.Close())

			port := testutil.RandomUnprivilegedPort(t)
			metricsURL := fmt.Sprintf("http://127.0.0.1:%d/metrics", port)
			recodingRulesFilePath := filepath.Join(root, "rules.yml")
			recordingRules := `
groups:
- name: group0
  rules:
  - record: up:count
    expr:  count(up==1)
`
			err = os.WriteFile(recodingRulesFilePath, []byte(recordingRules), 0o644)
			require.NoError(t, err)
			configFilePath := filepath.Join(root, "prometheus.yml")
			// Scraping + rules + remote_write to self.
			config := fmt.Sprintf(`
global:
  scrape_timeout: 100ms
  evaluation_interval: 50ms
  external_labels:
    prometheus: notme

scrape_configs:
- job_name: self1
  scrape_interval: 61ms
  static_configs:
    - targets: ["localhost:%d"]
- job_name: self2
  scrape_interval: 67ms
  static_configs:
    - targets: ["localhost:%d"]
- job_name: self3
  scrape_interval: 71ms
  static_configs:
    - targets: ["localhost:%d"]

remote_write:
  - url: "http://localhost:%d/api/v1/write"

rule_files:
  - %s
`, port, port, port, port, recodingRulesFilePath)
			err = os.WriteFile(configFilePath, []byte(config), 0o644)
			require.NoError(t, err)

			dbDir, err := os.MkdirTemp(root, "prometheus_data")
			require.NoError(t, err)

			var nospcDetected atomic.Bool
			detectNOSPC := func(l string) {
				t.Log(l)
				if nospcDetected.Load() {
					return
				}
				if strings.Contains(l, syscall.ENOSPC.Error()) {
					nospcDetected.Store(true)
				}
			}

			// The instance will be terminated at the end.
			t.Run(fmt.Sprintf("provoke ENOSPC fs_%d", i), func(t *testing.T) {
				prom := prometheusCommand(
					t,
					detectNOSPC,
					configFilePath,
					port,
					"--storage.tsdb.path",
					dbDir,
					"--web.enable-remote-write-receiver",
					// TODO: remove
					"--log.level",
					"debug",
				)
				require.NoError(t, prom.Start())

				for !nospcDetected.Load() {
					time.Sleep(100 * time.Millisecond)
				}
				// Sanity check: some WAL writes failed.
				failedWrites, err := eventuallyGetMetricValue(t, metricsURL, model.MetricTypeCounter, "prometheus_tsdb_wal_writes_failed_total")
				require.NoError(t, err)
				require.NotZero(t, failedWrites)

				// Let the instance run under ENOSPC to better simulate reality
				time.Sleep(500 * time.Millisecond)

				initialWalSize, err := eventuallyGetMetricValue(t, metricsURL, model.MetricTypeGauge, "prometheus_tsdb_wal_storage_size_bytes")
				require.NoError(t, err)
				require.NotZero(t, initialWalSize)

				// Free the reserved space
				require.NoError(t, os.Remove(placeholder))

				// Ensure it resumes WAL writing
				time.Sleep(500 * time.Millisecond)
				walSize, err := eventuallyGetMetricValue(t, metricsURL, model.MetricTypeGauge, "prometheus_tsdb_wal_storage_size_bytes")
				require.NoError(t, err)
				require.Greater(t, walSize, initialWalSize+1024)
			})

			// Restart the instance
			prom := prometheusCommand(
				t,
				nil,
				configFilePath,
				port,
				"--storage.tsdb.path",
				dbDir,
				"--web.enable-remote-write-receiver",
				// TODO: remove
				"--log.level",
				"debug",
			)
			require.NoError(t, prom.Start())

			// Ensure no corruptions
			corruptions, err := eventuallyGetMetricValue(t, metricsURL, model.MetricTypeCounter, "prometheus_tsdb_wal_corruptions_total")
			require.NoError(t, err)
			require.Equal(t, 0.0, corruptions)

			corruptions, err = eventuallyGetMetricValue(t, metricsURL, model.MetricTypeCounter, "prometheus_tsdb_wal_reader_corruption_errors_total")
			require.NoError(t, err)
			require.Equal(t, 0.0, corruptions)
		})
	}
}
