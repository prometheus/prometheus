// Copyright The Prometheus Authors
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
	"bufio"
	"compress/gzip"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/grafana/regexp"
	"github.com/prometheus/common/model"
	"github.com/stretchr/testify/require"

	"github.com/prometheus/prometheus/util/testutil"
)

const prometheusDocsLTSConfigURL = "https://raw.githubusercontent.com/prometheus/docs/main/docs-config.ts"

// fetchLTSPrefix fetches the single LTS version prefix (e.g. "3.5") from the docs config.
func fetchLTSPrefix(t *testing.T) string {
	t.Helper()

	resp, err := http.Get(prometheusDocsLTSConfigURL)
	require.NoError(t, err)
	defer resp.Body.Close()
	require.Equal(t, http.StatusOK, resp.StatusCode)

	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)

	// Extracts "X.Y" from the ltsVersions block:
	//   ltsVersions: {
	//     prometheus: ["X.Y"],
	//   },
	matches := regexp.MustCompile(`ltsVersions:\s*{\s*prometheus:\s*\["(\d+\.\d+)"]`).FindSubmatch(body)
	require.NotEmpty(t, matches)
	return string(matches[1])
}

func fetchLatestLTSRelease(t *testing.T, prefix string) (version, assetURL string) {
	t.Helper()

	resp, err := http.Get("https://api.github.com/repos/prometheus/prometheus/releases?per_page=100")
	require.NoError(t, err)
	defer resp.Body.Close()
	require.Equal(t, http.StatusOK, resp.StatusCode)

	var releases []struct {
		TagName    string `json:"tag_name"`
		Draft      bool   `json:"draft"`
		Prerelease bool   `json:"prerelease"`
	}
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&releases))

	// Releases are assumed sorted by publish date (newest first).
	for _, r := range releases {
		if r.Draft || r.Prerelease || !strings.HasPrefix(r.TagName, fmt.Sprintf("v%s.", prefix)) {
			continue
		}
		version = strings.TrimPrefix(r.TagName, "v")
		assetURL = fmt.Sprintf(
			"https://github.com/prometheus/prometheus/releases/download/%s/prometheus-%s.%s-%s.tar.gz",
			r.TagName, version, runtime.GOOS, runtime.GOARCH,
		)
		return version, assetURL
	}
	require.FailNow(t, "no release found matching LTS", "prefix", prefix)
	return "", ""
}

func getPrometheusMetricValue(t *testing.T, port int, metricType model.MetricType, metricName string) (float64, error) {
	t.Helper()

	resp, err := http.Get(fmt.Sprintf("http://127.0.0.1:%d/metrics", port))
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return 0, fmt.Errorf("bad status code %d", resp.StatusCode)
	}
	return getMetricValue(t, resp.Body, metricType, metricName)
}

type versionChangeTest struct {
	start time.Time

	ltsAssetURL       string
	ltsVersionBinPath string

	prometheusPort           int
	prometheusDataPath       string
	prometheusConfigFilePath string
	rulesFilePath            string
	remoteWriteURL           string
}

func (c versionChangeTest) downloadAndExtractLatestLTS(t *testing.T) {
	const prometheusBinName = "prometheus"

	resp, err := http.Get(c.ltsAssetURL)
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
			return
		}
	}
	require.FailNow(t, "prometheus binary not found in LTS tarball", "URL", c.ltsAssetURL)
}

// ensureHealthyMetrics polls metrics until all health invariants are satisfied. It checks
// both liveness (expected operations completed) and safety (no failures).
func (c versionChangeTest) ensureHealthyMetrics(t *testing.T) {
	t.Helper()
	checkStartTime := time.Now()

	for _, mc := range []struct {
		mType model.MetricType
		mName string
		check func(float64) bool
	}{
		{model.MetricTypeGauge, "prometheus_ready", func(v float64) bool { return v == 1 }},
		{model.MetricTypeGauge, "prometheus_config_last_reload_successful", func(v float64) bool { return v == 1 }},

		{model.MetricTypeCounter, "prometheus_target_scrape_pools_total", func(v float64) bool { return v == 3 }},
		{model.MetricTypeCounter, "prometheus_target_scrape_pools_failed_total", func(v float64) bool { return v == 0 }},
		{model.MetricTypeCounter, "prometheus_target_scrape_pool_reloads_failed_total", func(v float64) bool { return v == 0 }},

		{model.MetricTypeGauge, "prometheus_remote_storage_highest_timestamp_in_seconds", func(v float64) bool { return v > float64(checkStartTime.Unix()) }},

		{model.MetricTypeGauge, "prometheus_rule_group_rules", func(v float64) bool { return v == 2 }},
		{model.MetricTypeCounter, "prometheus_rule_evaluations_total", func(v float64) bool { return v >= 1 }},
		{model.MetricTypeCounter, "prometheus_rule_evaluation_failures_total", func(v float64) bool { return v == 0 }},

		{model.MetricTypeCounter, "prometheus_tsdb_compactions_triggered_total", func(v float64) bool { return v >= 1 }},
		{model.MetricTypeCounter, "prometheus_tsdb_compactions_total", func(v float64) bool { return v >= 1 }},
		{model.MetricTypeCounter, "prometheus_tsdb_compactions_failed_total", func(v float64) bool { return v == 0 }},

		{model.MetricTypeGauge, "prometheus_tsdb_head_series", func(v float64) bool { return v >= 1 }},
		{model.MetricTypeGauge, "prometheus_tsdb_head_chunks", func(v float64) bool { return v >= 1 }},
		{model.MetricTypeGauge, "prometheus_tsdb_head_chunks_storage_size_bytes", func(v float64) bool { return v >= 1 }},
		{model.MetricTypeCounter, "prometheus_tsdb_head_series_created_total", func(v float64) bool { return v >= 1 }},
		{model.MetricTypeGauge, "prometheus_tsdb_blocks_loaded", func(v float64) bool { return v >= 1 }},
		{model.MetricTypeGauge, "prometheus_tsdb_storage_blocks_bytes", func(v float64) bool { return v >= 1 }},
		{model.MetricTypeCounter, "prometheus_tsdb_reloads_total", func(v float64) bool { return v >= 1 }},
		{model.MetricTypeCounter, "prometheus_tsdb_reloads_failures_total", func(v float64) bool { return v == 0 }},

		{model.MetricTypeCounter, "prometheus_tsdb_head_chunks_created_total", func(v float64) bool { return v >= 1 }},
		{model.MetricTypeCounter, "prometheus_tsdb_head_chunks_removed_total", func(v float64) bool { return v >= 1 }},
		{model.MetricTypeCounter, "prometheus_tsdb_mmap_chunks_total", func(v float64) bool { return v >= 1 }},
		{model.MetricTypeCounter, "prometheus_tsdb_mmap_chunk_corruptions_total", func(v float64) bool { return v == 0 }},

		{model.MetricTypeCounter, "prometheus_tsdb_checkpoint_creations_total", func(v float64) bool { return v >= 1 }},
		{model.MetricTypeCounter, "prometheus_tsdb_checkpoint_creations_failed_total", func(v float64) bool { return v == 0 }},
		{model.MetricTypeCounter, "prometheus_tsdb_checkpoint_deletions_total", func(v float64) bool { return v >= 1 }},
		{model.MetricTypeCounter, "prometheus_tsdb_checkpoint_deletions_failed_total", func(v float64) bool { return v == 0 }},

		{model.MetricTypeCounter, "prometheus_tsdb_head_truncations_total", func(v float64) bool { return v >= 1 }},
		{model.MetricTypeCounter, "prometheus_tsdb_head_truncations_failed_total", func(v float64) bool { return v == 0 }},

		{model.MetricTypeGauge, "prometheus_tsdb_wal_storage_size_bytes", func(v float64) bool { return v >= 1 }},
		{model.MetricTypeCounter, "prometheus_tsdb_wal_completed_pages_total", func(v float64) bool { return v >= 1 }},
		{model.MetricTypeCounter, "prometheus_tsdb_wal_page_flushes_total", func(v float64) bool { return v >= 1 }},
		{model.MetricTypeCounter, "prometheus_tsdb_wal_truncations_total", func(v float64) bool { return v >= 1 }},
		{model.MetricTypeCounter, "prometheus_tsdb_wal_writes_failed_total", func(v float64) bool { return v == 0 }},
		{model.MetricTypeCounter, "prometheus_tsdb_wal_corruptions_total", func(v float64) bool { return v == 0 }},
		{model.MetricTypeCounter, "prometheus_tsdb_wal_truncations_failed_total", func(v float64) bool { return v == 0 }},
	} {
		require.Eventually(t, func() bool {
			val, err := getPrometheusMetricValue(t, c.prometheusPort, mc.mType, mc.mName)
			if err != nil {
				return false
			}
			if mc.check(val) {
				t.Logf("[%s] metric %s=%g is healthy", time.Since(c.start), mc.mName, val)
				return true
			}
			return false
		}, 7*time.Minute, 1*time.Second)
	}
}

func (c versionChangeTest) generatePrometheusConfig(t *testing.T) {
	t.Helper()

	rulesContent := `
groups:
  - name: upgrade_test
    interval: 3s
    rules:
      - record: upgrade_test:up:sum
        expr: sum(up)
      - alert: UpgradeTestAlwaysFiring
        expr: vector(1)
        for: 1s
        labels:
          severity: foo
`
	require.NoError(t, os.WriteFile(c.rulesFilePath, []byte(rulesContent), 0o644))

	promConfig := fmt.Sprintf(`
rule_files:
  - %s

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
      - targets: ['localhost:%d']

remote_write:
  - url: %s`, c.rulesFilePath, c.prometheusPort, c.prometheusPort, c.prometheusPort, c.remoteWriteURL)

	require.NoError(t, os.WriteFile(c.prometheusConfigFilePath, []byte(promConfig), 0o644))
}

// snapshotQueryRange queries all metrics over [c.start, end].
// The result is deterministic and can be compared across Prometheus versions to detect data loss or corruption.
func (c versionChangeTest) snapshotQueryRange(t *testing.T, end time.Time) string {
	t.Helper()

	u := fmt.Sprintf("http://127.0.0.1:%d/api/v1/query_range?query=%s&start=%d&end=%d&step=15",
		c.prometheusPort, url.QueryEscape(`{__name__=~".+"}`), c.start.Unix(), end.Unix())
	resp, err := http.Get(u)
	require.NoError(t, err)
	defer resp.Body.Close()
	require.Equal(t, http.StatusOK, resp.StatusCode)

	var parsed struct {
		Data struct {
			Result json.RawMessage `json:"result"`
		} `json:"data"`
	}
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&parsed))
	require.NotEqual(t, "[]", string(parsed.Data.Result))
	return string(parsed.Data.Result)
}

func ensureHealthyLogs(t *testing.T, r io.Reader) {
	scanner := bufio.NewScanner(r)
	for scanner.Scan() {
		line := scanner.Text()
		t.Log(line)
		require.NotContains(t, line, "level=ERROR")
	}
	require.NoError(t, scanner.Err())
}

var testVersionUpgrade = flag.Bool("test.version-upgrade", false, "run resource-intensive and probably slow version upgrade tests")

// TestVersionUpgrade_UpgradeDowngradeLatestLTS verifies that Prometheus can
// upgrade from the latest LTS release to the current build and then downgrade
// back without errors (data loss, corruption, etc.).
//
// NOTE: If this test is renamed, update the corresponding invocation in CI.
func TestVersionUpgrade_UpgradeDowngradeLatestLTS(t *testing.T) {
	if !*testVersionUpgrade {
		t.Skip("test can be slow, resource-intensive or requires internet access")
	}

	start := time.Now()
	rootDir := t.TempDir()

	ltsPrefix := fetchLTSPrefix(t)
	t.Logf("[%s] current LTS major.minor is %s from %s", time.Since(start), ltsPrefix, prometheusDocsLTSConfigURL)
	ltsVersion, ltsAssetURL := fetchLatestLTSRelease(t, ltsPrefix)
	t.Logf("[%s] using LTS tag %s from %s", time.Since(start), ltsVersion, ltsAssetURL)

	rwServer := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	t.Cleanup(rwServer.Close)

	c := versionChangeTest{
		start: start,

		ltsAssetURL: ltsAssetURL,

		ltsVersionBinPath: filepath.Join(rootDir, fmt.Sprintf("prometheus-%s", ltsVersion)),

		prometheusPort:           testutil.RandomUnprivilegedPort(t),
		prometheusDataPath:       filepath.Join(rootDir, "data"),
		prometheusConfigFilePath: filepath.Join(rootDir, "prometheus.yml"),
		rulesFilePath:            filepath.Join(rootDir, "rules.yml"),
		remoteWriteURL:           rwServer.URL,
	}

	t.Logf("[%s] downloading and preparing LTS %s", time.Since(start), ltsVersion)
	// TODO: Cache if downloads are expensive in CI.
	c.downloadAndExtractLatestLTS(t)
	t.Logf("[%s] downloaded and prepared LTS %s at %s", time.Since(start), ltsVersion, c.ltsVersionBinPath)

	c.generatePrometheusConfig(t)

	commonArgs := []string{
		fmt.Sprintf("--config.file=%s", c.prometheusConfigFilePath),
		fmt.Sprintf("--web.listen-address=0.0.0.0:%d", c.prometheusPort),
		fmt.Sprintf("--storage.tsdb.path=%s", c.prometheusDataPath),
		// Accelerate compaction.
		"--storage.tsdb.min-block-duration=65s",
		// Accelerate chunks mmapping.
		"--storage.tsdb.samples-per-chunk=10",
		"--log.level=debug",
	}

	runLTS := append([]string{c.ltsVersionBinPath}, commonArgs...)
	runCurrent := append([]string{os.Args[0], "-test.main"}, commonArgs...)

	var (
		queryRangeSnapshotEnd time.Time
		queryRangeBaseline    string
	)

	t.Run("Running the LTS version", func(t *testing.T) {
		cmd := commandWithLogging(t, ensureHealthyLogs, runLTS[0], runLTS[1:]...)
		require.NoError(t, cmd.Start())
		c.ensureHealthyMetrics(t)
		queryRangeSnapshotEnd = time.Now().Add(-3 * time.Second)
		queryRangeBaseline = c.snapshotQueryRange(t, queryRangeSnapshotEnd)
	})

	t.Run("Upgrading to the current build", func(t *testing.T) {
		cmd := commandWithLogging(t, ensureHealthyLogs, runCurrent[0], runCurrent[1:]...)
		require.NoError(t, cmd.Start())
		c.ensureHealthyMetrics(t)
		require.Equal(t, queryRangeBaseline, c.snapshotQueryRange(t, queryRangeSnapshotEnd))
		// Take a new snapshot after the upgrade.
		queryRangeSnapshotEnd = time.Now().Add(-3 * time.Second)
		queryRangeBaseline = c.snapshotQueryRange(t, queryRangeSnapshotEnd)
	})

	t.Run("Downgrading to the LTS version", func(t *testing.T) {
		cmd := commandWithLogging(t, ensureHealthyLogs, runLTS[0], runLTS[1:]...)
		require.NoError(t, cmd.Start())
		c.ensureHealthyMetrics(t)
		require.Equal(t, queryRangeBaseline, c.snapshotQueryRange(t, queryRangeSnapshotEnd))
	})
}

// TestVersionUpgrade_fetchLTSPrefix sanity-checks that the docs config reports the right LTS prefix.
func TestVersionUpgrade_fetchLTSPrefix(t *testing.T) {
	if !*testVersionUpgrade {
		t.Skip("test can be slow, resource-intensive or requires internet access")
	}
	// Update this when the LTS changes which happens at most once a year.
	require.Equal(t, "3.5", fetchLTSPrefix(t))
}
