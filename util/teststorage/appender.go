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

package teststorage

import (
	"bytes"
	"context"
	"fmt"
	"math"
	"strings"
	"sync"

	"github.com/prometheus/common/model"
	"github.com/prometheus/prometheus/model/exemplar"
	"github.com/prometheus/prometheus/model/histogram"
	"github.com/prometheus/prometheus/model/labels"
	"github.com/prometheus/prometheus/model/metadata"
	"github.com/prometheus/prometheus/storage"
)

// Sample represents test, combined sample for mocking storage.AppenderV2.
type Sample struct {
	MF    string
	L     labels.Labels
	M     metadata.Metadata
	ST, T int64
	V     float64
	H     *histogram.Histogram
	FH    *histogram.FloatHistogram
	ES    []exemplar.Exemplar
}

func (s Sample) String() string {
	b := bytes.Buffer{}
	if s.M.Help != "" {
		_, _ = fmt.Fprintf(&b, "HELP %s\n", s.M.Help)
	}
	if s.M.Type != model.MetricTypeUnknown && s.M.Type != "" {
		_, _ = fmt.Fprintf(&b, "type@%s ", s.M.Type)
	}
	if s.M.Unit != "" {
		_, _ = fmt.Fprintf(&b, "unit@%s ", s.M.Unit)
	}
	h := ""
	if s.H != nil {
		h = s.H.String()
	}

	fh := ""
	if s.FH != nil {
		fh = s.FH.String()
	}
	_, _ = fmt.Fprintf(&b, "%s %v%v%v st@%v t@%v\n", s.L.String(), s.V, h, fh, s.ST, s.T)
	return b.String()
}

func (s Sample) exemplarsEqual(other []exemplar.Exemplar) bool {
	if len(s.ES) != len(other) {
		return false
	}
	for i := range s.ES {
		if !s.ES[i].Equals(other[i]) {
			return false
		}
	}
	return true
}

func (s Sample) Equal(other Sample) bool {
	return strings.Compare(s.MF, other.MF) == 0 &&
		labels.Equal(s.L, other.L) &&
		s.M.Equals(other.M) &&
		s.ST == other.ST &&
		s.T == other.T &&
		math.Float64bits(s.V) == math.Float64bits(s.V) && // Compare Float64bits so NaN values which are exactly the same will compare equal.
		s.H.Equals(other.H) &&
		s.FH.Equals(other.FH) &&
		s.exemplarsEqual(other.ES)
}

// Appender is a storage.Appender mock.
// It allows:
// * recording all samples that were added through the appender.
// * optionally backed by another appender it writes samples through (Next).
// * optionally runs another appender before result recording e.g. to simulate chained validation (Prev)
type Appender struct {
	Prev storage.Appendable // Optional appender to run before the result collection.
	Next storage.Appendable // Optional appender to run after results are collected (e.g. TestStorage).

	AppendErr            error // Inject appender error on every Append, AppendHistogram and ST zero calls.
	AppendExemplarsError error // Inject exemplar error.
	CommitErr            error // Inject commit error.

	mtx sync.Mutex // mutex for result writes and ResultSamplesGreaterThan read.

	// Recorded results.
	PendingSamples    []Sample
	ResultSamples     []Sample
	RolledbackSamples []Sample
}

func (a *Appender) ResultReset() {
	a.PendingSamples = a.PendingSamples[:0]
	a.ResultSamples = a.ResultSamples[:0]
	a.RolledbackSamples = a.RolledbackSamples[:0]
}

func (a *Appender) ResultSamplesGreaterThan(than int) bool {
	a.mtx.Lock()
	defer a.mtx.Unlock()
	return len(a.ResultSamples) > than
}

// ResultMetadata returns ResultSamples with samples only containing L and M.
// This is for compatibility with tests that only focus on metadata.
//
// TODO: Rewrite tests to test metadata on ResultSamples instead.
func (a *Appender) ResultMetadata() []Sample {
	var ret []Sample
	for _, s := range a.ResultSamples {
		ret = append(ret, Sample{L: s.L, M: s.M})
	}
	return ret
}

func (a *Appender) String() string {
	var sb strings.Builder
	sb.WriteString("committed:\n")
	for _, s := range a.ResultSamples {
		sb.WriteString("\n")
		sb.WriteString(s.String())
	}
	sb.WriteString("pending:\n")
	for _, s := range a.PendingSamples {
		sb.WriteString("\n")
		sb.WriteString(s.String())
	}
	sb.WriteString("rolledback:\n")
	for _, s := range a.RolledbackSamples {
		sb.WriteString("\n")
		sb.WriteString(s.String())
	}
	return sb.String()
}

func NewAppender() *Appender {
	return &Appender{}
}

type appender struct {
	prev storage.Appender
	next storage.Appender

	*Appender
}

func (a *Appender) Appender(ctx context.Context) storage.Appender {
	ret := &appender{Appender: a}
	if a.Prev != nil {
		ret.prev = a.Prev.Appender(ctx)
	}
	if a.Next != nil {
		ret.next = a.Next.Appender(ctx)
	}
	return ret
}

func (a *appender) SetOptions(*storage.AppendOptions) {}

func (a *appender) Append(ref storage.SeriesRef, ls labels.Labels, t int64, v float64) (storage.SeriesRef, error) {
	if a.Prev != nil {
		if _, err := a.prev.Append(ref, ls, t, v); err != nil {
			return 0, err
		}
	}

	if a.AppendErr != nil {
		return 0, a.AppendErr
	}

	a.mtx.Lock()
	a.PendingSamples = append(a.PendingSamples, Sample{L: ls, T: t, V: v})
	a.mtx.Unlock()

	if a.next != nil {
		return a.next.Append(ref, ls, t, v)
	}

	if ref == 0 {
		// Use labels hash as a stand-in for unique series reference, to avoid having to track all series.
		ref = storage.SeriesRef(ls.Hash())
	}
	return ref, nil
}

func (a *appender) AppendHistogram(ref storage.SeriesRef, ls labels.Labels, t int64, h *histogram.Histogram, fh *histogram.FloatHistogram) (storage.SeriesRef, error) {
	if a.Prev != nil {
		if _, err := a.prev.AppendHistogram(ref, ls, t, h, fh); err != nil {
			return 0, err
		}
	}

	if a.AppendErr != nil {
		return 0, a.AppendErr
	}

	a.mtx.Lock()
	a.PendingSamples = append(a.PendingSamples, Sample{L: ls, T: t, H: h, FH: fh})
	a.mtx.Unlock()

	if a.next != nil {
		return a.next.AppendHistogram(ref, ls, t, h, fh)
	}

	if ref == 0 {
		// Use labels hash as a stand-in for unique series reference, to avoid having to track all series.
		ref = storage.SeriesRef(ls.Hash())
	}
	return ref, nil
}

func (a *appender) AppendExemplar(ref storage.SeriesRef, l labels.Labels, e exemplar.Exemplar) (storage.SeriesRef, error) {
	if a.Prev != nil {
		if _, err := a.prev.AppendExemplar(ref, l, e); err != nil {
			return 0, err
		}
	}

	if a.AppendExemplarsError != nil {
		return 0, a.AppendExemplarsError
	}

	a.mtx.Lock()
	// NOTE(bwplotka): Eventually exemplar has to be attached to a series and soon
	// the AppenderV2 will guarantee that for TSDB. Assume this from the mock perspective
	// with the naive attaching. See: https://github.com/prometheus/prometheus/pull/17610
	i := 0
	for range len(a.PendingSamples) {
		if ref == storage.SeriesRef(a.PendingSamples[i].L.Hash()) {
			a.PendingSamples[i].ES = append(a.PendingSamples[i].ES, e)
			break
		}
		i++
	}
	a.mtx.Unlock()
	if i >= len(a.PendingSamples) {
		return 0, fmt.Errorf("teststorage.appender: exemplar appender without series; ref %v; l %v; exemplar: %v", ref, l, e)
	}

	if a.next != nil {
		return a.next.AppendExemplar(ref, l, e)
	}
	return ref, nil
}

func (a *appender) AppendSTZeroSample(ref storage.SeriesRef, l labels.Labels, _, st int64) (storage.SeriesRef, error) {
	return a.Append(ref, l, st, 0.0)
}

func (a *appender) AppendHistogramSTZeroSample(ref storage.SeriesRef, l labels.Labels, _, st int64, h *histogram.Histogram, _ *histogram.FloatHistogram) (storage.SeriesRef, error) {
	if h != nil {
		return a.AppendHistogram(ref, l, st, &histogram.Histogram{}, nil)
	}
	return a.AppendHistogram(ref, l, st, nil, &histogram.FloatHistogram{})
}

func (a *appender) UpdateMetadata(ref storage.SeriesRef, l labels.Labels, m metadata.Metadata) (storage.SeriesRef, error) {
	if a.Prev != nil {
		if _, err := a.prev.UpdateMetadata(ref, l, m); err != nil {
			return 0, err
		}
	}

	a.mtx.Lock()
	// NOTE(bwplotka): Eventually exemplar has to be attached to a series and soon
	// the AppenderV2 will guarantee that for TSDB. Assume this from the mock perspective
	// with the naive attaching. See: https://github.com/prometheus/prometheus/pull/17610
	i := 0
	for range len(a.PendingSamples) {
		if ref == storage.SeriesRef(a.PendingSamples[i].L.Hash()) {
			a.PendingSamples[i].M = m
			break
		}
		i++
	}
	a.mtx.Unlock()
	if i >= len(a.PendingSamples) {
		return 0, fmt.Errorf("teststorage.appender: metadata update without series; ref %v; l %v; m: %v", ref, l, m)
	}

	if a.next != nil {
		return a.next.UpdateMetadata(ref, l, m)
	}
	return ref, nil
}

func (a *appender) Commit() error {
	if a.Prev != nil {
		if err := a.prev.Commit(); err != nil {
			return err
		}
	}

	if a.CommitErr != nil {
		return a.CommitErr
	}

	a.mtx.Lock()
	a.ResultSamples = append(a.ResultSamples, a.PendingSamples...)
	a.PendingSamples = a.PendingSamples[:0]
	a.mtx.Unlock()

	if a.next != nil {
		return a.next.Commit()
	}
	return nil
}

func (a *appender) Rollback() error {
	if a.prev != nil {
		if err := a.prev.Rollback(); err != nil {
			return err
		}
	}

	a.mtx.Lock()
	a.RolledbackSamples = append(a.RolledbackSamples, a.PendingSamples...)
	a.PendingSamples = a.PendingSamples[:0]
	a.mtx.Unlock()

	if a.next != nil {
		return a.next.Rollback()
	}
	return nil
}
