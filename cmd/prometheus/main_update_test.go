// Copyright 2020 The Prometheus Authors
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
	"archive/tar"
	"compress/gzip"
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sync"
	"syscall"
	"testing"
	"time"

	"github.com/prometheus/common/expfmt"
	"github.com/stretchr/testify/require"

	"github.com/prometheus/prometheus/util/testutil"
)

type metricType int

const (
	counterMetric metricType = 1
	gaugeMetric   metricType = 2
)

func retryOnError(ctx context.Context, interval time.Duration, f func() bool) (canceled bool) {
	var ok bool
	ok = f()
	for {
		if ok {
			return false
		}
		select {
		case <-ctx.Done():
			return true
		case <-time.After(interval):
			ok = f()
		}
	}
}

func getPrometheusMetricValue(t *testing.T, port int, metricType metricType, metricName string) (float64, error) {
	t.Helper()

	getMetricValue := func(body io.ReadCloser) (float64, error) {
		p := expfmt.TextParser{}
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
		case counterMetric:
			return metric[0].GetCounter().GetValue(), nil
		case gaugeMetric:
			return metric[0].GetGauge().GetValue(), nil
		}
		return 0, errors.New("unsupported metric type")
	}

	resp, err := http.Get(fmt.Sprintf("http://127.0.0.1:%d/metrics", port))
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return 0, fmt.Errorf("bad status code %d", resp.StatusCode)
	}
	val, err := getMetricValue(resp.Body)
	if err != nil {
		return 0, err
	}
	return val, nil
}

type versionChangeTest struct {
	start time.Time
	runWg *sync.WaitGroup
	ctx   context.Context

	ltsVersion        string
	ltsVersionBinPath string

	port           int
	dataPath       string
	configFilePath string
}

func (c versionChangeTest) downloadAndExtractLatestLTS(t *testing.T) {
	prometheusBinName := "prometheus"
	assetURL := fmt.Sprintf(
		"https://github.com/prometheus/prometheus/releases/download/v%s/prometheus-%s.%s-%s.tar.gz",
		c.ltsVersion, c.ltsVersion, runtime.GOOS, runtime.GOARCH,
	)

	resp, err := http.Get(assetURL)
	require.NoError(t, err)
	defer resp.Body.Close()

	require.Equal(t, http.StatusOK, resp.StatusCode)

	gzReader, err := gzip.NewReader(resp.Body)
	require.NoError(t, err)
	defer gzReader.Close()

	tarReader := tar.NewReader(gzReader)
	for {
		header, err := tarReader.Next()
		if err == io.EOF {
			break
		}
		require.NoError(t, err)

		if header.Typeflag == tar.TypeReg && filepath.Base(header.Name) == prometheusBinName {
			out, err := os.OpenFile(c.ltsVersionBinPath, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o755)
			require.NoError(t, err)
			defer out.Close()

			_, err = io.Copy(out, tarReader)
			require.NoError(t, err)
			break
		}
	}
}

// Relies heavily on metrics, assumes metrics are correcteness.
func (c versionChangeTest) ensureInvariants(t *testing.T) {
	t.Helper()

	type operation int
	const (
		equal     operation = 1
		greaterEq operation = 2
	)

	// Make sure many tsdb related operations are exercised.
	for _, metricCheck := range []struct {
		mType metricType
		mName string
		op    operation
		value float64
	}{
		// Prometheus is ready.
		{
			mType: gaugeMetric,
			mName: "prometheus_ready",
			op:    equal,
			value: 1,
		},
		// WAL is written.
		{
			mType: gaugeMetric,
			mName: "prometheus_tsdb_wal_storage_size_bytes",
			op:    greaterEq,
			value: 8,
		},
		// At least one compaction.
		{
			mType: counterMetric,
			mName: "prometheus_tsdb_compactions_total",
			op:    greaterEq,
			value: 1,
		},
		// At least one loaded block.
		{
			mType: gaugeMetric,
			mName: "prometheus_tsdb_blocks_loaded",
			op:    greaterEq,
			value: 1,
		},
		// Chunks are mmapped.
		{
			mType: counterMetric,
			mName: "prometheus_tsdb_mmap_chunks_total",
			op:    greaterEq,
			value: 1,
		},
		// Checkpoints created.
		{
			mType: counterMetric,
			mName: "prometheus_tsdb_checkpoint_creations_total",
			op:    greaterEq,
			value: 1,
		},
		// Not failures during those operations.
		{
			mType: counterMetric,
			mName: "prometheus_tsdb_mmap_chunk_corruptions_total",
			op:    equal,
			value: 0,
		},
		{
			mType: counterMetric,
			mName: "prometheus_tsdb_wal_writes_failed_total",
			op:    equal,
			value: 0,
		},
		{
			mType: counterMetric,
			mName: "prometheus_tsdb_compactions_failed_total",
			op:    equal,
			value: 0,
		},
		{
			mType: counterMetric,
			mName: "prometheus_tsdb_checkpoint_creations_failed_total",
			op:    equal,
			value: 0,
		},
		{
			mType: counterMetric,
			mName: "prometheus_tsdb_head_truncations_failed_total",
			op:    equal,
			value: 0,
		},
		{
			mType: counterMetric,
			mName: "prometheus_tsdb_reloads_failures_total",
			op:    equal,
			value: 0,
		},
		{
			mType: counterMetric,
			mName: "prometheus_tsdb_wal_corruptions_total",
			op:    equal,
			value: 0,
		},
	} {
		require.False(t, retryOnError(c.ctx, 100*time.Millisecond, func() bool {
			val, err := getPrometheusMetricValue(t, c.port, metricCheck.mType, metricCheck.mName)
			if err != nil {
				return false
			}
			switch metricCheck.op {
			case equal:
				if val == metricCheck.value {
					t.Logf("[%s] metric %s value %f reached", time.Since(c.start), metricCheck.mName, metricCheck.value)
					return true
				}
				return false
			case greaterEq:
				if val >= metricCheck.value {
					t.Logf("[%s] metric %s value %f greater than %f as wanted", time.Since(c.start), metricCheck.mName, val, metricCheck.value)
					return true
				}
				return false
			default:
				t.Fatalf("unsupported operation")
			}
			return false
		}))
	}
}

func (c versionChangeTest) generatePrometheusConfig(t *testing.T) {
	promConfig := fmt.Sprintf(`
scrape_configs:
  - job_name: 'self1'
    scrape_interval: 1s
    static_configs:
      - targets: ['localhost:%d']
  - job_name: 'self2'
    scrape_interval: 2s
    static_configs:
      - targets: ['localhost:%d']
  - job_name: 'self3'
    scrape_interval: 3s
    static_configs:
      - targets: ['localhost:%d']`, c.port, c.port, c.port)

	err := os.WriteFile(c.configFilePath, []byte(promConfig), 0o644)
	require.NoError(t, err)
}

func (c versionChangeTest) runAndCheckPrometheus(t *testing.T, command []string) {
	prometheus := exec.CommandContext(c.ctx, command[0], command[1:]...)
	stderr, err := prometheus.StderrPipe()
	require.NoError(t, err)

	c.runWg.Add(1)
	go func() {
		defer c.runWg.Done()
		slurp, _ := io.ReadAll(stderr)
		t.Log(string(slurp))
	}()

	require.NoError(t, prometheus.Start())
	c.ensureInvariants(t)

	prometheus.Process.Signal(syscall.SIGTERM)
	require.NoError(t, prometheus.Wait())
}

var testVersionUpdate = flag.Bool("test.version-update", false, "run resource-intensive and probably slow version update tests")

// TODO: state what + ask to update ci.ymlk if any change test name.
func TestVersionUpdate_UpgradeDowngrade_LatestLTS(t *testing.T) {
	if !*testVersionUpdate {
		t.Skip("test can be slow and resource-intensive")
	}

	// TODO: automate, maybe via a tag or sth to the release??
	latestLTSVersion := "2.53.4"
	rootDir := t.TempDir()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	c := versionChangeTest{
		start: time.Now(),
		runWg: &sync.WaitGroup{},
		ctx:   ctx,

		ltsVersion:        latestLTSVersion,
		ltsVersionBinPath: filepath.Join(rootDir, fmt.Sprintf("prometheus-%s", latestLTSVersion)),

		port:           testutil.RandomUnprivilegedPort(t),
		dataPath:       filepath.Join(rootDir, "data"),
		configFilePath: filepath.Join(rootDir, "prometheus.yml"),
	}

	c.downloadAndExtractLatestLTS(t)
	c.generatePrometheusConfig(t)
	defer os.Remove(c.ltsVersionBinPath)

	commonArgs := []string{
		fmt.Sprintf("--config.file=%s", c.configFilePath),
		fmt.Sprintf("--web.listen-address=0.0.0.0:%d", c.port),
		fmt.Sprintf("--storage.tsdb.path=%s", c.dataPath),
		// precipitated compaction.
		"--storage.tsdb.min-block-duration=65s",
		// precipitated chunks mmapping.
		"--storage.tsdb.samples-per-chunk=10",
	}

	runLTS := append([]string{c.ltsVersionBinPath}, commonArgs...)
	runCurrent := append([]string{os.Args[0], "-test.main"}, commonArgs...)

	defer c.runWg.Wait()
	// Initialize from LTS
	c.runAndCheckPrometheus(t, runLTS)
	// Upgrade to current.
	c.runAndCheckPrometheus(t, runCurrent)
	// Downgrade to LTS.
	c.runAndCheckPrometheus(t, runLTS)
}
