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

package agent

import (
	"fmt"
	"math"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"testing"
	"time"

	remoteapi "github.com/prometheus/client_golang/exp/api/remote"
	"github.com/prometheus/client_golang/prometheus"
	commonconfig "github.com/prometheus/common/config"
	"github.com/prometheus/common/model"
	"github.com/prometheus/common/promslog"
	"github.com/stretchr/testify/require"

	"github.com/prometheus/prometheus/config"
	"github.com/prometheus/prometheus/model/exemplar"
	"github.com/prometheus/prometheus/model/labels"
	"github.com/prometheus/prometheus/model/relabel"
	"github.com/prometheus/prometheus/model/value"
	writev2 "github.com/prometheus/prometheus/prompb/io/prometheus/write/v2"
	"github.com/prometheus/prometheus/storage"
	"github.com/prometheus/prometheus/storage/remote"
	"github.com/prometheus/prometheus/tsdb/tsdbutil"
)

func TestMemoryWriteAgent(t *testing.T) {
	for _, appenderV2 := range []bool{false, true} {
		for _, enableST := range []bool{false, true} {
			t.Run(fmt.Sprintf("appender_v2=%t/st=%t", appenderV2, enableST), func(t *testing.T) {
				received := make(chan *writev2.Request, 20)
				server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					req, err := remote.DecodeWriteV2Request(r.Body)
					if err != nil {
						http.Error(w, err.Error(), http.StatusBadRequest)
						return
					}
					var samples, histograms, exemplars int
					for _, ts := range req.Timeseries {
						samples += len(ts.Samples)
						histograms += len(ts.Histograms)
						exemplars += len(ts.Exemplars)
					}
					received <- req
					w.Header().Set("X-Prometheus-Remote-Write-Samples-Written", strconv.Itoa(samples))
					w.Header().Set("X-Prometheus-Remote-Write-Histograms-Written", strconv.Itoa(histograms))
					w.Header().Set("X-Prometheus-Remote-Write-Exemplars-Written", strconv.Itoa(exemplars))
					w.WriteHeader(http.StatusNoContent)
				}))
				defer server.Close()
				dir := filepath.Join(t.TempDir(), "absent")
				reg := prometheus.NewRegistry()
				rs := remote.NewStorage(nil, reg, startTime, dir, time.Second, nil, false)
				defer func() { require.NoError(t, rs.Close()) }()
				opts := DefaultOptions()
				opts.DisableWAL = true
				opts.EnableSTStorage = enableST
				db, err := Open(promslog.NewNopLogger(), reg, rs, dir, opts)
				require.NoError(t, err)
				defer func() { require.NoError(t, db.Close()) }()
				endpoint, err := url.Parse(server.URL)
				require.NoError(t, err)
				rw := config.DefaultRemoteWriteConfig
				rw.URL = &commonconfig.URL{URL: endpoint}
				rw.ProtobufMessage = remoteapi.WriteV2MessageType
				rw.SendExemplars = true
				rw.SendNativeHistograms = true
				rw.MetadataConfig.Send = false
				rw.QueueConfig.MaxShards = 1
				rw.QueueConfig.BatchSendDeadline = model.Duration(5 * time.Millisecond)
				rw.WriteRelabelConfigs = []*relabel.Config{{
					SourceLabels: model.LabelNames{"__name__"},
					Regex:        relabel.MustNewRegexp("drop_me"),
					Action:       relabel.Drop,
				}}
				conf := &config.Config{GlobalConfig: config.DefaultGlobalConfig, RemoteWriteConfigs: []*config.RemoteWriteConfig{&rw}}
				conf.GlobalConfig.ExternalLabels = labels.FromStrings("cluster", "before")
				require.NoError(t, rs.ApplyConfig(conf))
				ls := labels.FromStrings("__name__", "float_metric")
				app := db.Appender(t.Context())
				ref, err := app.Append(0, ls, 500, 123)
				require.NoError(t, err)
				require.NoError(t, app.Rollback())

				h := tsdbutil.GenerateTestHistograms(1)[0]
				fh := h.ToFloat(nil)
				e := exemplar.Exemplar{Labels: labels.FromStrings("trace_id", "abc"), Ts: 1000, Value: 42, HasTs: true}
				if appenderV2 {
					a := db.AppenderV2(t.Context())
					_, err = a.Append(ref, labels.EmptyLabels(), 100, 1000, 42, nil, nil, storage.AOptions{Exemplars: []exemplar.Exemplar{e}})
					require.NoError(t, err)
					_, err = a.Append(0, labels.FromStrings("__name__", "histogram_metric"), 100, 1000, 0, h, nil, storage.AOptions{})
					require.NoError(t, err)
					_, err = a.Append(0, labels.FromStrings("__name__", "float_histogram_metric"), 100, 1000, 0, nil, fh, storage.AOptions{})
					require.NoError(t, err)
					_, err = a.Append(0, labels.FromStrings("__name__", "drop_me"), 100, 1000, 1, nil, nil, storage.AOptions{})
					require.NoError(t, err)
					require.NoError(t, a.Commit())
				} else {
					app = db.Appender(t.Context())
					_, err = app.Append(ref, labels.EmptyLabels(), 1000, 42)
					require.NoError(t, err)
					_, err = app.AppendExemplar(ref, labels.EmptyLabels(), e)
					require.NoError(t, err)
					_, err = app.AppendHistogram(0, labels.FromStrings("__name__", "histogram_metric"), 1000, h, nil)
					require.NoError(t, err)
					_, err = app.AppendHistogram(0, labels.FromStrings("__name__", "float_histogram_metric"), 1000, nil, fh)
					require.NoError(t, err)
					_, err = app.Append(0, labels.FromStrings("__name__", "drop_me"), 1000, 1)
					require.NoError(t, err)
					require.NoError(t, app.Commit())
				}
				expectedST := int64(0)
				if appenderV2 && enableST {
					expectedST = 100
				}
				counts := map[string]int{}
				for count := 0; count < 4; {
					select {
					case req := <-received:
						builder := labels.NewScratchBuilder(4)
						for _, ts := range req.Timeseries {
							gotLabels, err := ts.ToLabels(&builder, req.Symbols)
							require.NoError(t, err)
							require.Equal(t, "before", gotLabels.Get("cluster"))
							name := gotLabels.Get("__name__")
							require.NotEqual(t, "drop_me", name)
							for _, sample := range ts.Samples {
								require.Equal(t, int64(1000), sample.Timestamp)
								require.Equal(t, expectedST, sample.StartTimestamp)
								require.Equal(t, 42.0, sample.Value)
								counts["sample"]++
								count++
							}
							for _, got := range ts.Histograms {
								require.Equal(t, int64(1000), got.Timestamp)
								require.Equal(t, expectedST, got.StartTimestamp)
								if name == "histogram_metric" {
									require.Equal(t, h, got.ToIntHistogram())
								} else {
									require.Equal(t, fh, got.ToFloatHistogram())
								}
								counts[name]++
								count++
							}
							for _, got := range ts.Exemplars {
								require.Equal(t, int64(1000), got.Timestamp)
								require.Equal(t, 42.0, got.Value)
								counts["exemplar"]++
								count++
							}
						}
					case <-time.After(5 * time.Second):
						t.Fatal("timed out waiting for remote write")
					}
				}
				require.Equal(t, map[string]int{"sample": 1, "histogram_metric": 1, "float_histogram_metric": 1, "exemplar": 1}, counts)

				// Replacing a queue must accept already-known series without WAL replay.
				conf.GlobalConfig.ExternalLabels = labels.FromStrings("cluster", "after")
				require.NoError(t, rs.ApplyConfig(conf))
				app = db.Appender(t.Context())
				_, err = app.Append(ref, labels.EmptyLabels(), 1000, 1)
				require.ErrorIs(t, err, storage.ErrOutOfOrderSample)
				_, err = app.Append(ref, labels.EmptyLabels(), 2000, math.Float64frombits(value.StaleNaN))
				require.NoError(t, err)
				require.NoError(t, app.Commit())
				select {
				case req := <-received:
					require.Len(t, req.Timeseries, 1)
					builder := labels.NewScratchBuilder(4)
					gotLabels, err := req.Timeseries[0].ToLabels(&builder, req.Symbols)
					require.NoError(t, err)
					require.Equal(t, "after", gotLabels.Get("cluster"))
					require.True(t, value.IsStaleNaN(req.Timeseries[0].Samples[0].Value))
				case <-time.After(5 * time.Second):
					t.Fatal("timed out waiting for reloaded remote write")
				}
				_, err = os.Stat(dir)
				require.ErrorIs(t, err, os.ErrNotExist)
			})
		}
	}
}

func TestMemoryWriteAgentGCAndExistingWAL(t *testing.T) {
	dir := t.TempDir()
	walDir := filepath.Join(dir, "wal")
	require.NoError(t, os.Mkdir(walDir, 0o755))
	walFile := filepath.Join(walDir, "00000000")
	contents := []byte("An existing WAL must not be read, repaired, or removed.")
	require.NoError(t, os.WriteFile(walFile, contents, 0o600))
	rs := remote.NewStorage(nil, nil, startTime, dir, time.Second, nil, false)
	defer func() { require.NoError(t, rs.Close()) }()
	opts := DefaultOptions()
	opts.DisableWAL = true
	db, err := Open(promslog.NewNopLogger(), nil, rs, dir, opts)
	require.NoError(t, err)
	app := db.Appender(t.Context())
	ls := labels.FromStrings("__name__", "metric")
	ref, err := app.Append(0, ls, 1000, 1)
	require.NoError(t, err)
	require.NoError(t, app.Commit())
	db.gc(2000)
	require.Nil(t, db.series.GetByID(1))
	require.Empty(t, db.deleted)
	app = db.Appender(t.Context())
	newRef, err := app.Append(ref, ls, 2000, 2)
	require.NoError(t, err)
	require.NotEqual(t, ref, newRef)
	require.NoError(t, app.Commit())
	require.NoError(t, db.Close())
	actual, err := os.ReadFile(walFile)
	require.NoError(t, err)
	require.Equal(t, contents, actual)
	files, err := os.ReadDir(dir)
	require.NoError(t, err)
	require.Len(t, files, 1)
}
