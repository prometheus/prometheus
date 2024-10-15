// Copyright 2022 The Prometheus Authors
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

package textparse

import (
	"errors"
	"io"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/prometheus/common/model"
	"github.com/stretchr/testify/require"

	"github.com/prometheus/prometheus/model/exemplar"
	"github.com/prometheus/prometheus/model/histogram"
	"github.com/prometheus/prometheus/model/labels"
	"github.com/prometheus/prometheus/util/testutil"
)

func TestNewParser(t *testing.T) {
	t.Parallel()

	requirePromParser := func(t *testing.T, p Parser) {
		require.NotNil(t, p)
		_, ok := p.(*PromParser)
		require.True(t, ok)
	}

	requireOpenMetricsParser := func(t *testing.T, p Parser) {
		require.NotNil(t, p)
		_, ok := p.(*OpenMetricsParser)
		require.True(t, ok)
	}

	for name, tt := range map[string]*struct {
		contentType    string
		validateParser func(*testing.T, Parser)
		err            string
	}{
		"empty-string": {
			validateParser: requirePromParser,
		},
		"invalid-content-type-1": {
			contentType:    "invalid/",
			validateParser: requirePromParser,
			err:            "expected token after slash",
		},
		"invalid-content-type-2": {
			contentType:    "invalid/invalid/invalid",
			validateParser: requirePromParser,
			err:            "unexpected content after media subtype",
		},
		"invalid-content-type-3": {
			contentType:    "/",
			validateParser: requirePromParser,
			err:            "no media type",
		},
		"invalid-content-type-4": {
			contentType:    "application/openmetrics-text; charset=UTF-8; charset=utf-8",
			validateParser: requirePromParser,
			err:            "duplicate parameter name",
		},
		"openmetrics": {
			contentType:    "application/openmetrics-text",
			validateParser: requireOpenMetricsParser,
		},
		"openmetrics-with-charset": {
			contentType:    "application/openmetrics-text; charset=utf-8",
			validateParser: requireOpenMetricsParser,
		},
		"openmetrics-with-charset-and-version": {
			contentType:    "application/openmetrics-text; version=1.0.0; charset=utf-8",
			validateParser: requireOpenMetricsParser,
		},
		"plain-text": {
			contentType:    "text/plain",
			validateParser: requirePromParser,
		},
		"plain-text-with-version": {
			contentType:    "text/plain; version=0.0.4",
			validateParser: requirePromParser,
		},
		"some-other-valid-content-type": {
			contentType:    "text/html",
			validateParser: requirePromParser,
		},
	} {
		t.Run(name, func(t *testing.T) {
			tt := tt // Copy to local variable before going parallel.
			t.Parallel()

			p, err := New([]byte{}, tt.contentType, false, false, labels.NewSymbolTable())
			tt.validateParser(t, p)
			if tt.err == "" {
				require.NoError(t, err)
			} else {
				require.ErrorContains(t, err, tt.err)
			}
		})
	}
}

// parsedEntry represents data that is parsed for each entry.
type parsedEntry struct {
	// In all but EntryComment, EntryInvalid.
	m string

	// In EntryHistogram.
	shs *histogram.Histogram
	fhs *histogram.FloatHistogram

	// In EntrySeries.
	v float64

	// In EntrySeries and EntryHistogram.
	lset labels.Labels
	t    *int64
	es   []exemplar.Exemplar
	ct   *int64

	// In EntryType.
	typ model.MetricType
	// In EntryHelp.
	help string
	// In EntryUnit.
	unit string
	// In EntryComment.
	comment string
}

func requireEntries(t *testing.T, exp, got []parsedEntry) {
	t.Helper()

	testutil.RequireEqualWithOptions(t, exp, got, []cmp.Option{
		cmp.AllowUnexported(parsedEntry{}),
	})
}

func testParse(t *testing.T, p Parser) (ret []parsedEntry) {
	t.Helper()

	for {
		et, err := p.Next()
		if errors.Is(err, io.EOF) {
			break
		}
		require.NoError(t, err)

		var got parsedEntry
		var m []byte
		switch et {
		case EntryInvalid:
			t.Fatal("entry invalid not expected")
		case EntrySeries, EntryHistogram:
			if et == EntrySeries {
				m, got.t, got.v = p.Series()
				got.m = string(m)
			} else {
				m, got.t, got.shs, got.fhs = p.Histogram()
				got.m = string(m)
			}

			p.Metric(&got.lset)
			for e := (exemplar.Exemplar{}); p.Exemplar(&e); {
				got.es = append(got.es, e)
			}
			// Parser reuses int pointer.
			if ct := p.CreatedTimestamp(); ct != nil {
				got.ct = int64p(*ct)
			}
		case EntryType:
			m, got.typ = p.Type()
			got.m = string(m)

		case EntryHelp:
			m, h := p.Help()
			got.m = string(m)
			got.help = string(h)

		case EntryUnit:
			m, u := p.Unit()
			got.m = string(m)
			got.unit = string(u)

		case EntryComment:
			got.comment = string(p.Comment())
		}
		ret = append(ret, got)
	}
	return ret
}
