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

package prometheusremotewrite

import (
	"errors"
	"testing"

	"github.com/google/go-cmp/cmp"

	"github.com/prometheus/prometheus/model/exemplar"
	"github.com/prometheus/prometheus/model/histogram"
	"github.com/prometheus/prometheus/model/labels"
	"github.com/prometheus/prometheus/model/metadata"
	"github.com/prometheus/prometheus/storage"
	"github.com/prometheus/prometheus/util/testutil"
)

// TODO(bwplotka): Move to teststorage.Appendable. This require slight refactor of tests and I couldn't do this before
// switching to AppenderV2 (I would need to adjust AppenderV1 mock exemplar flow which is pointless since we don't plan
// to use it). For now keeping tests diff small for confidence.
type mockCombinedAppender struct {
	pendingSamples    []combinedSample
	pendingHistograms []combinedHistogram

	samples    []combinedSample
	histograms []combinedHistogram
}

type combinedSample struct {
	metricFamilyName string
	ls               labels.Labels
	meta             metadata.Metadata
	t                int64
	st               int64
	v                float64
	es               []exemplar.Exemplar
}

type combinedHistogram struct {
	metricFamilyName string
	ls               labels.Labels
	meta             metadata.Metadata
	t                int64
	st               int64
	h                *histogram.Histogram
	es               []exemplar.Exemplar
}

func (m *mockCombinedAppender) Append(_ storage.SeriesRef, ls labels.Labels, st, t int64, v float64, h *histogram.Histogram, _ *histogram.FloatHistogram, opts storage.AOptions) (_ storage.SeriesRef, err error) {
	if h != nil {
		m.pendingHistograms = append(m.pendingHistograms, combinedHistogram{
			metricFamilyName: opts.MetricFamilyName,
			ls:               ls,
			meta:             opts.Metadata,
			t:                t,
			st:               st,
			h:                h,
			es:               opts.Exemplars,
		})
		return 0, nil
	}
	m.pendingSamples = append(m.pendingSamples, combinedSample{
		metricFamilyName: opts.MetricFamilyName,
		ls:               ls,
		meta:             opts.Metadata,
		t:                t,
		st:               st,
		v:                v,
		es:               opts.Exemplars,
	})
	return 0, nil
}

func (m *mockCombinedAppender) Commit() error {
	m.samples = append(m.samples, m.pendingSamples...)
	m.pendingSamples = m.pendingSamples[:0]
	m.histograms = append(m.histograms, m.pendingHistograms...)
	m.pendingHistograms = m.pendingHistograms[:0]
	return nil
}

func (*mockCombinedAppender) Rollback() error {
	return errors.New("not implemented")
}

func requireEqual(t testing.TB, expected, actual any, msgAndArgs ...any) {
	testutil.RequireEqualWithOptions(t, expected, actual, []cmp.Option{cmp.AllowUnexported(combinedSample{}, combinedHistogram{})}, msgAndArgs...)
}
