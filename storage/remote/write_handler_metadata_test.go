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

package remote

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	remoteapi "github.com/prometheus/client_golang/exp/api/remote"
	"github.com/prometheus/common/promslog"
	"github.com/stretchr/testify/require"

	"github.com/prometheus/prometheus/model/labels"
	writev2 "github.com/prometheus/prometheus/prompb/io/prometheus/write/v2"
	"github.com/prometheus/prometheus/tsdb"
	"github.com/prometheus/prometheus/tsdb/record"
	"github.com/prometheus/prometheus/tsdb/wlog"
)

func TestRemoteWriteHandlerV2RepeatedMetadata(t *testing.T) {
	const samples = 64
	help := strings.Repeat("h", 64<<10)
	symbols := writev2.NewSymbolTable()
	ls := labels.FromStrings("__name__", "m")
	labelRefs := symbols.SymbolizeLabels(ls, nil)
	helpRef := symbols.Symbolize(help)
	series := make([]writev2.TimeSeries, samples)
	for i := range series {
		series[i] = writev2.TimeSeries{
			LabelsRefs: labelRefs,
			Samples:    []writev2.Sample{{Timestamp: int64(i + 1), Value: 1}},
			Metadata:   writev2.Metadata{Type: writev2.Metadata_METRIC_TYPE_GAUGE, HelpRef: helpRef},
		}
	}
	payload, _, _, _, err := buildV2WriteRequest(promslog.NewNopLogger(), series, symbols.Symbols(), nil, nil, nil, "snappy")
	require.NoError(t, err)
	opts := tsdb.DefaultOptions()
	opts.EnableMetadataWALRecords = true
	db, err := tsdb.Open(t.TempDir(), nil, nil, opts, nil)
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, db.Close()) })
	handler := NewWriteHandler(promslog.NewNopLogger(), nil, db.Head(), []remoteapi.WriteMessageType{remoteapi.WriteV2MessageType}, false, false, true)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/write", bytes.NewReader(payload)).WithContext(t.Context())
	req.Header.Set("Content-Type", remoteWriteContentTypeHeaders[remoteapi.WriteV2MessageType])
	req.Header.Set("Content-Encoding", "snappy")
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, req)
	require.Equal(t, http.StatusNoContent, response.Code, response.Body.String())
	require.Equal(t, "64", response.Header().Get("X-Prometheus-Remote-Write-Samples-Written"))

	segments, err := wlog.NewSegmentsReader(filepath.Join(db.Dir(), "wal"))
	require.NoError(t, err)
	defer func() { require.NoError(t, segments.Close()) }()
	reader := wlog.NewReader(segments)
	decoder := record.NewDecoder(labels.NewSymbolTable(), promslog.NewNopLogger())
	var metadataEntries, sampleEntries int
	for reader.Next() {
		r := reader.Record()
		switch decoder.Type(r) {
		case record.Metadata:
			m, err := decoder.Metadata(r, nil)
			require.NoError(t, err)
			metadataEntries += len(m)
			for _, entry := range m {
				require.Equal(t, uint8(record.Gauge), entry.Type)
				require.Equal(t, help, entry.Help)
			}
		case record.Samples, record.SamplesV2:
			s, err := decoder.Samples(r, nil)
			require.NoError(t, err)
			sampleEntries += len(s)
		}
	}
	require.NoError(t, reader.Err())
	require.Equal(t, 1, metadataEntries)
	require.Equal(t, samples, sampleEntries)
}
