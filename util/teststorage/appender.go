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
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/prometheus/common/model"
	"github.com/stretchr/testify/require"
	"go.uber.org/atomic"

	"github.com/prometheus/prometheus/model/exemplar"
	"github.com/prometheus/prometheus/model/histogram"
	"github.com/prometheus/prometheus/model/labels"
	"github.com/prometheus/prometheus/model/metadata"
	"github.com/prometheus/prometheus/model/value"
	"github.com/prometheus/prometheus/storage"
	"github.com/prometheus/prometheus/util/testutil"
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
		h = " " + s.H.String()
	}
	fh := ""
	if s.FH != nil {
		fh = " " + s.FH.String()
	}
	b.WriteString(fmt.Sprintf("%s %v%v%v st@%v t@%v", s.L.String(), s.V, h, fh, s.ST, s.T))
	if len(s.ES) > 0 {
		b.WriteString(fmt.Sprintf(" %v", s.ES))
	}
	b.WriteString("\n")
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

// IsStale returns whether the sample represents a stale sample, according to
// https://prometheus.io/docs/specs/native_histograms/#staleness-markers.
func (s Sample) IsStale() bool {
	switch {
	case s.FH != nil:
		return value.IsStaleNaN(s.FH.Sum)
	case s.H != nil:
		return value.IsStaleNaN(s.H.Sum)
	default:
		return value.IsStaleNaN(s.V)
	}
}

var sampleComparer = cmp.Comparer(func(a, b Sample) bool {
	return a.Equals(b)
})

// RequireEqual is a special require equal that correctly compare Prometheus structures.
//
// In comparison to testutil.RequireEqual, this function adds special logic for comparing []Samples.
//
// It also ignores ordering between consecutive stale samples to avoid false
// negatives due to map iteration order in staleness tracking.
func RequireEqual(t testing.TB, expected, got []Sample, msgAndArgs ...any) {
	opts := []cmp.Option{sampleComparer}
	expected = reorderExpectedForStaleness(expected, got)
	testutil.RequireEqualWithOptions(t, expected, got, opts, msgAndArgs...)
}

// RequireNotEqual is the negation of RequireEqual.
func RequireNotEqual(t testing.TB, expected, got []Sample, msgAndArgs ...any) {
	t.Helper()

	opts := []cmp.Option{cmp.Comparer(labels.Equal), sampleComparer}
	expected = reorderExpectedForStaleness(expected, got)
	if !cmp.Equal(expected, got, opts...) {
		return
	}
	require.Fail(t, fmt.Sprintf("Equal, but expected not: \n"+
		"a: %s\n"+
		"b: %s", expected, got), msgAndArgs...)
}

func reorderExpectedForStaleness(expected, got []Sample) []Sample {
	if len(expected) != len(got) || !includeStaleNaNs(expected) {
		return expected
	}
	result := make([]Sample, len(expected))
	copy(result, expected)

	// Try to reorder only consecutive stale samples to avoid false negatives
	// due to map iteration order in staleness tracking.
	for i := range result {
		if !result[i].IsStale() {
			continue
		}
		if result[i].Equals(got[i]) {
			continue
		}
		for j := i + 1; j < len(result); j++ {
			if !result[j].IsStale() {
				break
			}
			if result[j].Equals(got[i]) {
				// Swap.
				result[i], result[j] = result[j], result[i]
				break
			}
		}
	}
	return result
}

func includeStaleNaNs(s []Sample) bool {
	for _, e := range s {
		if e.IsStale() {
			return true
		}
	}
	return false
}

// Appendable is a storage.Appendable mock.
// It allows recording all samples that were added through the appender and injecting errors.
// Appendable will panic if more than one Appender is open.
type Appendable struct {
	appendErrFn          func(ls labels.Labels) error // If non-nil, inject appender error on every Append, AppendHistogram and ST zero calls.
	appendExemplarsError error                        // If non-nil, inject exemplar error.
	commitErr            error                        // If non-nil, inject commit error.
	skipRecording        bool                         // If true, Appendable won't record samples, useful for benchmarks.

	mtx           sync.Mutex
	openAppenders atomic.Int32 // Guard against multi-appender use.

	// Recorded results.
	pendingSamples    []Sample
	resultSamples     []Sample
	rolledbackSamples []Sample

	// Optional chain (Appender will collect samples, then run next).
	next compatAppendable
}

// NewAppendable returns mock Appendable.
func NewAppendable() *Appendable {
	return &Appendable{}
}

type compatAppendable interface {
	storage.Appendable
	storage.AppendableV2
}

// Then chains another appender from the provided Appendable for the Appender calls.
func (a *Appendable) Then(appendable compatAppendable) *Appendable {
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

// SkipRecording enables or disables recording appended samples.
// If skipped, Appendable allocs less, but Result*() methods will give always empty results. This is useful for benchmarking.
func (a *Appendable) SkipRecording(skipRecording bool) *Appendable {
	a.skipRecording = skipRecording
	return a
}

// PendingSamples returns pending samples (samples appended without commit).
func (a *Appendable) PendingSamples() []Sample {
	a.mtx.Lock()
	defer a.mtx.Unlock()
	if len(a.pendingSamples) == 0 {
		return nil
	}

	ret := make([]Sample, len(a.pendingSamples))
	copy(ret, a.pendingSamples)
	return ret
}

// ResultSamples returns committed samples.
func (a *Appendable) ResultSamples() []Sample {
	a.mtx.Lock()
	defer a.mtx.Unlock()
	if len(a.resultSamples) == 0 {
		return nil
	}

	ret := make([]Sample, len(a.resultSamples))
	copy(ret, a.resultSamples)
	return ret
}

// RolledbackSamples returns rolled back samples.
func (a *Appendable) RolledbackSamples() []Sample {
	a.mtx.Lock()
	defer a.mtx.Unlock()
	if len(a.rolledbackSamples) == 0 {
		return nil
	}

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

type baseAppender struct {
	err error

	nextTr storage.AppenderTransaction
	a      *Appendable
}

func (a *baseAppender) checkErr() error {
	a.a.mtx.Lock()
	defer a.a.mtx.Unlock()

	return a.err
}

func (a *baseAppender) Commit() error {
	if err := a.checkErr(); err != nil {
		return err
	}
	defer a.a.openAppenders.Dec()

	if a.a.commitErr != nil {
		return a.a.commitErr
	}

	a.a.mtx.Lock()
	if !a.a.skipRecording {
		a.a.resultSamples = append(a.a.resultSamples, a.a.pendingSamples...)
		a.a.pendingSamples = a.a.pendingSamples[:0]
	}
	a.err = errClosedAppender
	a.a.mtx.Unlock()

	if a.nextTr != nil {
		return a.nextTr.Commit()
	}
	return nil
}

func (a *baseAppender) Rollback() error {
	if err := a.checkErr(); err != nil {
		return err
	}
	defer a.a.openAppenders.Dec()

	a.a.mtx.Lock()
	if !a.a.skipRecording {
		a.a.rolledbackSamples = append(a.a.rolledbackSamples, a.a.pendingSamples...)
		a.a.pendingSamples = a.a.pendingSamples[:0]
	}
	a.err = errClosedAppender
	a.a.mtx.Unlock()

	if a.nextTr != nil {
		return a.nextTr.Rollback()
	}
	return nil
}

type appender struct {
	baseAppender

	next storage.Appender
}

func (a *Appendable) Appender(ctx context.Context) storage.Appender {
	ret := &appender{baseAppender: baseAppender{a: a}}
	if a.openAppenders.Inc() > 1 {
		ret.err = errors.New("teststorage.Appendable.Appender() concurrent use is not supported; attempted opening new Appender() without Commit/Rollback of the previous one. Extend the implementation if concurrent mock is needed")
		return ret
	}

	if a.next != nil {
		app := a.next.Appender(ctx)
		ret.next, ret.nextTr = app, app
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

	if !a.a.skipRecording {
		a.a.mtx.Lock()
		a.a.pendingSamples = append(a.a.pendingSamples, Sample{L: ls, T: t, V: v})
		a.a.mtx.Unlock()
	}

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
		// Check for buggy ref while we are at it. This only makes sense for cases without .Then*, because further appendable
		// might have a different ref computation logic e.g. TSDB uses atomic increments.
		return 0, errors.New("teststorage.appender: found input ref not matching labels; potential bug in Appendable usage")
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

	if !a.a.skipRecording {
		a.a.mtx.Lock()
		a.a.pendingSamples = append(a.a.pendingSamples, Sample{L: ls, T: t, H: h, FH: fh})
		a.a.mtx.Unlock()
	}

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

	if !a.a.skipRecording {
		var appended bool

		a.a.mtx.Lock()
		// NOTE(bwplotka): Eventually exemplar has to be attached to a series and soon
		// the AppenderV2 will guarantee that for TSDB. Assume this from the mock perspective
		// with the naive attaching. See: https://github.com/prometheus/prometheus/issues/17632
		i := len(a.a.pendingSamples) - 1
		for ; i >= 0; i-- { // Attach exemplars to the last matching sample.
			if labels.Equal(l, a.a.pendingSamples[i].L) {
				a.a.pendingSamples[i].ES = append(a.a.pendingSamples[i].ES, e)
				appended = true
				break
			}
		}
		a.a.mtx.Unlock()
		if !appended {
			return 0, fmt.Errorf("teststorage.appender: exemplar appender without series; ref %v; l %v; exemplar: %v", ref, l, e)
		}
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

	if !a.a.skipRecording {
		var updated bool

		a.a.mtx.Lock()
		// NOTE(bwplotka): Eventually metadata has to be attached to a series and soon
		// the AppenderV2 will guarantee that for TSDB. Assume this from the mock perspective
		// with the naive attaching. See: https://github.com/prometheus/prometheus/issues/17632
		i := len(a.a.pendingSamples) - 1
		for ; i >= 0; i-- { // Attach metadata to the last matching sample.
			if labels.Equal(l, a.a.pendingSamples[i].L) {
				a.a.pendingSamples[i].M = m
				updated = true
				break
			}
		}
		a.a.mtx.Unlock()
		if !updated {
			return 0, fmt.Errorf("teststorage.appender: metadata update without series; ref %v; l %v; m: %v", ref, l, m)
		}
	}

	if a.next != nil {
		return a.next.UpdateMetadata(ref, l, m)
	}
	return computeOrCheckRef(ref, l)
}

type appenderV2 struct {
	baseAppender

	next storage.AppenderV2
}

func (a *Appendable) AppenderV2(ctx context.Context) storage.AppenderV2 {
	ret := &appenderV2{baseAppender: baseAppender{a: a}}
	if a.openAppenders.Inc() > 1 {
		ret.err = errors.New("teststorage.Appendable.AppenderV2() concurrent use is not supported; attempted opening new AppenderV2() without Commit/Rollback of the previous one. Extend the implementation if concurrent mock is needed")
		return ret
	}

	if a.next != nil {
		app := a.next.AppenderV2(ctx)
		ret.next, ret.nextTr = app, app
	}
	return ret
}

func (a *appenderV2) Append(ref storage.SeriesRef, ls labels.Labels, st, t int64, v float64, h *histogram.Histogram, fh *histogram.FloatHistogram, opts storage.AOptions) (_ storage.SeriesRef, err error) {
	if err := a.checkErr(); err != nil {
		return 0, err
	}

	if a.a.appendErrFn != nil {
		if err := a.a.appendErrFn(ls); err != nil {
			return 0, err
		}
	}

	var partialErr error
	if !a.a.skipRecording {
		var es []exemplar.Exemplar

		if len(opts.Exemplars) > 0 {
			if a.a.appendExemplarsError != nil {
				var exErrs []error
				for range opts.Exemplars {
					exErrs = append(exErrs, a.a.appendExemplarsError)
				}
				if len(exErrs) > 0 {
					partialErr = &storage.AppendPartialError{ExemplarErrors: exErrs}
				}
			} else {
				// As per AppenderV2 interface, opts.Exemplar slice is unsafe for reuse.
				es = make([]exemplar.Exemplar, len(opts.Exemplars))
				copy(es, opts.Exemplars)
			}
		}

		a.a.mtx.Lock()
		a.a.pendingSamples = append(a.a.pendingSamples, Sample{
			MF: opts.MetricFamilyName,
			M:  opts.Metadata,
			L:  ls,
			ST: st, T: t,
			V: v, H: h, FH: fh,
			ES: es,
		})
		a.a.mtx.Unlock()
	}

	if a.next != nil {
		ref, err = a.next.Append(ref, ls, st, t, v, h, fh, opts)
		if err != nil {
			return 0, err
		}
	} else {
		ref, err = computeOrCheckRef(ref, ls)
		if err != nil {
			return ref, err
		}
	}
	return ref, partialErr
}
