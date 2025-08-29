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

// Appender is a storage.AppenderV2 mock.
// It allows:
// * recording all samples that were added through the appender.
// * optionally backed by another appender it writes samples through (Next).
// * optionally runs another appender before result recording e.g. to simulate chained validation (Prev)
// TODO(bwplotka): Move to storage/interface/mock or something?
type Appender struct {
	Prev storage.AppendableV2 // Optional appender to run before the result collection.
	Next storage.AppendableV2 // Optional appender to run after results are collected (e.g. TestStorage).

	AppendErr               error // Inject appender error on every Append run.
	AppendAllExemplarsError error // Inject storage.AppendPartialError for all exemplars.
	CommitErr               error // Inject commit error.

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
// This is for compatibility with old tests that only focus on metadata.
//
// Deprecated: Rewrite tests to test metadata on ResultSamples instead.
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
	prev storage.AppenderV2
	next storage.AppenderV2

	*Appender
}

func (a *Appender) AppenderV2(ctx context.Context) storage.AppenderV2 {
	ret := &appender{Appender: a}
	if a.Prev != nil {
		ret.prev = a.Prev.AppenderV2(ctx)
	}
	if a.Next != nil {
		ret.next = a.Next.AppenderV2(ctx)
	}
	return ret
}

func (a *appender) Append(ref storage.SeriesRef, ls labels.Labels, st, t int64, v float64, h *histogram.Histogram, fh *histogram.FloatHistogram, opts storage.AOptions) (storage.SeriesRef, error) {
	if a.Prev != nil {
		if _, err := a.prev.Append(ref, ls, st, t, v, h, fh, opts); err != nil {
			return 0, err
		}
	}

	if a.AppendErr != nil {
		return 0, a.AppendErr
	}

	a.mtx.Lock()
	a.PendingSamples = append(a.PendingSamples, Sample{
		MF: opts.MetricFamilyName,
		M:  opts.Metadata,
		L:  ls,
		ST: st, T: t,
		V: v, H: h, FH: fh,
		ES: opts.Exemplars,
	})
	a.mtx.Unlock()

	var err error
	if a.AppendAllExemplarsError != nil {
		var exErrs []error
		for range opts.Exemplars {
			exErrs = append(exErrs, a.AppendAllExemplarsError)
		}
		if len(exErrs) > 0 {
			err = &storage.AppendPartialError{ExemplarErrors: exErrs}
		}
		if ref == 0 {
			// Use labels hash as a stand-in for unique series reference, to avoid having to track all series.
			ref = storage.SeriesRef(ls.Hash())
		}
		return ref, err
	}

	if a.next != nil {
		return a.next.Append(ref, ls, st, t, v, h, fh, opts)
	}

	if ref == 0 {
		// Use labels hash as a stand-in for unique series reference, to avoid having to track all series.
		ref = storage.SeriesRef(ls.Hash())
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
