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
	"context"
	"errors"
	"fmt"
	"math"
	"slices"
	"strings"
	"sync"

	"github.com/prometheus/common/model"
	"go.uber.org/atomic"

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
	// Attempting to format similar to ~ OpenMetrics 2.0 for readability.
	b := strings.Builder{}
	if s.M.Help != "" {
		b.WriteString("HELP ")
		b.WriteString(s.M.Help)
		b.WriteString("\n")
	}
	if s.M.Type != model.MetricTypeUnknown && s.M.Type != "" {
		b.WriteString("type@")
		b.WriteString(string(s.M.Type))
		b.WriteString(" ")
	}
	if s.M.Unit != "" {
		b.WriteString("unit@")
		b.WriteString(s.M.Unit)
		b.WriteString(" ")
	}
	// Print all value types on purpose, to catch bugs for appending multiple sample types at once.
	h := ""
	if s.H != nil {
		h = s.H.String()
	}
	fh := ""
	if s.FH != nil {
		fh = s.FH.String()
	}
	b.WriteString(fmt.Sprintf("%s %v%v%v st@%v t@%v\n", s.L.String(), s.V, h, fh, s.ST, s.T))
	return b.String()
}

func (s Sample) Equals(other Sample) bool {
	return strings.Compare(s.MF, other.MF) == 0 &&
		labels.Equal(s.L, other.L) &&
		s.M.Equals(other.M) &&
		s.ST == other.ST &&
		s.T == other.T &&
		math.Float64bits(s.V) == math.Float64bits(other.V) && // Compare Float64bits so NaN values which are exactly the same will compare equal.
		s.H.Equals(other.H) &&
		s.FH.Equals(other.FH) &&
		slices.EqualFunc(s.ES, other.ES, exemplar.Exemplar.Equals)
}

// Appendable is a storage.Appendable mock.
// It allows recording all samples that were added through the appender and injecting errors.
// Appendable will panic if more than one Appender is open.
type Appendable struct {
	appendErrFn          func(ls labels.Labels) error // If non-nil, inject appender error on every Append, AppendHistogram and ST zero calls.
	appendExemplarsError error                        // If non-nil, inject exemplar error.
	commitErr            error                        // If non-nil, inject commit error.

	mtx           sync.Mutex
	openAppenders atomic.Int32 // Guard against multi-appender use.

	// Recorded results.
	pendingSamples    []Sample
	resultSamples     []Sample
	rolledbackSamples []Sample

	// Optional chain (Appender will collect samples, then run next).
	next storage.Appendable
}

// NewAppendable returns mock Appendable.
func NewAppendable() *Appendable {
	return &Appendable{}
}

// Then chains another appender from the provided appendable for the Appender calls.
func (a *Appendable) Then(appendable storage.Appendable) *Appendable {
	a.next = appendable
	return a
}

// WithErrs allows injecting errors to the appender.
func (a *Appendable) WithErrs(appendErrFn func(ls labels.Labels) error, appendExemplarsError, commitErr error) *Appendable {
	a.appendErrFn = appendErrFn
	a.appendExemplarsError = appendExemplarsError
	a.commitErr = commitErr
	return a
}

// PendingSamples returns pending samples (samples appended without commit).
func (a *Appendable) PendingSamples() []Sample {
	a.mtx.Lock()
	defer a.mtx.Unlock()

	ret := make([]Sample, len(a.pendingSamples))
	copy(ret, a.pendingSamples)
	return ret
}

// ResultSamples returns committed samples.
func (a *Appendable) ResultSamples() []Sample {
	a.mtx.Lock()
	defer a.mtx.Unlock()

	ret := make([]Sample, len(a.resultSamples))
	copy(ret, a.resultSamples)
	return ret
}

// RolledbackSamples returns rolled back samples.
func (a *Appendable) RolledbackSamples() []Sample {
	a.mtx.Lock()
	defer a.mtx.Unlock()

	ret := make([]Sample, len(a.rolledbackSamples))
	copy(ret, a.rolledbackSamples)
	return ret
}

func (a *Appendable) ResultReset() {
	a.mtx.Lock()
	defer a.mtx.Unlock()

	a.pendingSamples = a.pendingSamples[:0]
	a.resultSamples = a.resultSamples[:0]
	a.rolledbackSamples = a.rolledbackSamples[:0]
}

// ResultMetadata returns resultSamples with samples only containing L and M.
// This is for compatibility with tests that only focus on metadata.
//
// TODO: Rewrite tests to test metadata on resultSamples instead.
func (a *Appendable) ResultMetadata() []Sample {
	a.mtx.Lock()
	defer a.mtx.Unlock()

	var ret []Sample
	for _, s := range a.resultSamples {
		if s.M.IsEmpty() {
			continue
		}
		ret = append(ret, Sample{L: s.L, M: s.M})
	}
	return ret
}

func (a *Appendable) String() string {
	var sb strings.Builder
	sb.WriteString("committed:\n")
	for _, s := range a.resultSamples {
		sb.WriteString("\n")
		sb.WriteString(s.String())
	}
	sb.WriteString("pending:\n")
	for _, s := range a.pendingSamples {
		sb.WriteString("\n")
		sb.WriteString(s.String())
	}
	sb.WriteString("rolledback:\n")
	for _, s := range a.rolledbackSamples {
		sb.WriteString("\n")
		sb.WriteString(s.String())
	}
	return sb.String()
}

var errClosedAppender = errors.New("appender was already committed/rolledback")

type appender struct {
	err  error
	next storage.Appender

	a *Appendable
}

func (a *appender) checkErr() error {
	a.a.mtx.Lock()
	defer a.a.mtx.Unlock()

	return a.err
}

func (a *Appendable) Appender(ctx context.Context) storage.Appender {
	ret := &appender{a: a}
	if a.openAppenders.Inc() > 1 {
		ret.err = errors.New("teststorage.Appendable.Appender() concurrent use is not supported; attempted opening new Appender() without Commit/Rollback of the previous one. Extend the implementation if concurrent mock is needed")
	}

	if a.next != nil {
		ret.next = a.next.Appender(ctx)
	}
	return ret
}

func (*appender) SetOptions(*storage.AppendOptions) {}

func (a *appender) Append(ref storage.SeriesRef, ls labels.Labels, t int64, v float64) (storage.SeriesRef, error) {
	if err := a.checkErr(); err != nil {
		return 0, err
	}

	if a.a.appendErrFn != nil {
		if err := a.a.appendErrFn(ls); err != nil {
			return 0, err
		}
	}

	a.a.mtx.Lock()
	a.a.pendingSamples = append(a.a.pendingSamples, Sample{L: ls, T: t, V: v})
	a.a.mtx.Unlock()

	if a.next != nil {
		return a.next.Append(ref, ls, t, v)
	}

	return computeOrCheckRef(ref, ls)
}

func computeOrCheckRef(ref storage.SeriesRef, ls labels.Labels) (storage.SeriesRef, error) {
	h := ls.Hash()
	if ref == 0 {
		// Use labels hash as a stand-in for unique series reference, to avoid having to track all series.
		return storage.SeriesRef(h), nil
	}

	if storage.SeriesRef(h) != ref {
		// Check for buggy ref while we at it.
		return 0, errors.New("teststorage.appender: found input ref not matching labels; potential bug in Appendable user")
	}
	return ref, nil
}

func (a *appender) AppendHistogram(ref storage.SeriesRef, ls labels.Labels, t int64, h *histogram.Histogram, fh *histogram.FloatHistogram) (storage.SeriesRef, error) {
	if err := a.checkErr(); err != nil {
		return 0, err
	}
	if a.a.appendErrFn != nil {
		if err := a.a.appendErrFn(ls); err != nil {
			return 0, err
		}
	}

	a.a.mtx.Lock()
	a.a.pendingSamples = append(a.a.pendingSamples, Sample{L: ls, T: t, H: h, FH: fh})
	a.a.mtx.Unlock()

	if a.next != nil {
		return a.next.AppendHistogram(ref, ls, t, h, fh)
	}

	return computeOrCheckRef(ref, ls)
}

func (a *appender) AppendExemplar(ref storage.SeriesRef, l labels.Labels, e exemplar.Exemplar) (storage.SeriesRef, error) {
	if err := a.checkErr(); err != nil {
		return 0, err
	}
	if a.a.appendExemplarsError != nil {
		return 0, a.a.appendExemplarsError
	}

	a.a.mtx.Lock()
	// NOTE(bwplotka): Eventually exemplar has to be attached to a series and soon
	// the AppenderV2 will guarantee that for TSDB. Assume this from the mock perspective
	// with the naive attaching. See: https://github.com/prometheus/prometheus/issues/17632
	i := len(a.a.pendingSamples) - 1
	for ; i >= 0; i-- { // Attach exemplars to the last matching sample.
		if ref == storage.SeriesRef(a.a.pendingSamples[i].L.Hash()) {
			a.a.pendingSamples[i].ES = append(a.a.pendingSamples[i].ES, e)
			break
		}
	}
	a.a.mtx.Unlock()
	if i < 0 {
		return 0, fmt.Errorf("teststorage.appender: exemplar appender without series; ref %v; l %v; exemplar: %v", ref, l, e)
	}

	if a.next != nil {
		return a.next.AppendExemplar(ref, l, e)
	}
	return computeOrCheckRef(ref, l)
}

func (a *appender) AppendSTZeroSample(ref storage.SeriesRef, l labels.Labels, _, st int64) (storage.SeriesRef, error) {
	return a.Append(ref, l, st, 0.0) // This will change soon with AppenderV2, but we already report ST as 0 samples.
}

func (a *appender) AppendHistogramSTZeroSample(ref storage.SeriesRef, l labels.Labels, _, st int64, h *histogram.Histogram, _ *histogram.FloatHistogram) (storage.SeriesRef, error) {
	if h != nil {
		return a.AppendHistogram(ref, l, st, &histogram.Histogram{}, nil)
	}
	return a.AppendHistogram(ref, l, st, nil, &histogram.FloatHistogram{}) // This will change soon with AppenderV2, but we already report ST as 0 histograms.
}

func (a *appender) UpdateMetadata(ref storage.SeriesRef, l labels.Labels, m metadata.Metadata) (storage.SeriesRef, error) {
	if err := a.checkErr(); err != nil {
		return 0, err
	}

	a.a.mtx.Lock()
	// NOTE(bwplotka): Eventually metadata has to be attached to a series and soon
	// the AppenderV2 will guarantee that for TSDB. Assume this from the mock perspective
	// with the naive attaching. See: https://github.com/prometheus/prometheus/issues/17632
	i := len(a.a.pendingSamples) - 1
	for ; i >= 0; i-- { // Attach metadata to the last matching sample.
		if ref == storage.SeriesRef(a.a.pendingSamples[i].L.Hash()) {
			a.a.pendingSamples[i].M = m
			break
		}
	}
	a.a.mtx.Unlock()
	if i < 0 {
		return 0, fmt.Errorf("teststorage.appender: metadata update without series; ref %v; l %v; m: %v", ref, l, m)
	}

	if a.next != nil {
		return a.next.UpdateMetadata(ref, l, m)
	}
	return computeOrCheckRef(ref, l)
}

func (a *appender) Commit() error {
	if err := a.checkErr(); err != nil {
		return err
	}
	defer a.a.openAppenders.Dec()

	if a.a.commitErr != nil {
		return a.a.commitErr
	}

	a.a.mtx.Lock()
	a.a.resultSamples = append(a.a.resultSamples, a.a.pendingSamples...)
	a.a.pendingSamples = a.a.pendingSamples[:0]
	a.err = errClosedAppender
	a.a.mtx.Unlock()

	if a.a.next != nil {
		return a.next.Commit()
	}
	return nil
}

func (a *appender) Rollback() error {
	if err := a.checkErr(); err != nil {
		return err
	}
	defer a.a.openAppenders.Dec()

	a.a.mtx.Lock()
	a.a.rolledbackSamples = append(a.a.rolledbackSamples, a.a.pendingSamples...)
	a.a.pendingSamples = a.a.pendingSamples[:0]
	a.err = errClosedAppender
	a.a.mtx.Unlock()

	if a.next != nil {
		return a.next.Rollback()
	}
	return nil
}
