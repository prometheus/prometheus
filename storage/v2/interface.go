package storagev2

import (
	"math"

	"github.com/prometheus/prometheus/model/exemplar"
	"github.com/prometheus/prometheus/model/histogram"
	"github.com/prometheus/prometheus/model/series"
	"github.com/prometheus/prometheus/model/value"
	"github.com/prometheus/prometheus/storage"
)

// Sample to be appended.
// TODO(bwplotka): Double check if function arguments would not be faster.
type Sample struct {
	// SchemaURL string one day.
	// CT int64 // 0 means no created timestamp.
	T int64 // required.

	// h, fh, ordered by priority e.g. if h is not nil fh and v is assumed empty.
	H  *histogram.Histogram
	FH *histogram.FloatHistogram
	V  float64

	Exemplars []exemplar.Exemplar // optional exemplars to attach.
}

func StaleSample(t int64) Sample {
	return Sample{T: t, V: math.Float64frombits(value.StaleNaN)}
}

// IsHistogram returns true if the sample is a histogram with a native representation.
func (sa Sample) IsHistogram() bool {
	return sa.H != nil || sa.FH != nil
}

// Appender provides batched appends against a storage.
// It must be completed with a call to Commit or Rollback and must not be reused afterwards.
//
// Operations on the Appender interface are not goroutine-safe.
//
// The type of samples (float64, histogram, etc) appended for a given series must remain same within an Appender.
// The behaviour is undefined if samples of different types are appended to the same series in a single Commit().
type Appender interface {
	// Commit submits the collected samples and purges the batch. If Commit
	// returns a non-nil error, it also rolls back all modifications made in
	// the appender so far, as Rollback would do. In any case, an Appender
	// must not be used anymore after Commit has been called.
	Commit() error

	// Rollback rolls back all modifications made in the appender so far.
	// Appender has to be discarded after rollback.
	Rollback() error

	// AppendSample adds a sample for the given series.
	// An optional series reference can be provided to accelerate calls.
	// A series reference number is returned which can be used to add further
	// samples to the given series in the same or later transactions.
	// Returned reference numbers are ephemeral and may be rejected in calls
	// to Append() at any point. Adding the sample via Append() returns a new
	// reference number.
	// If the reference is 0 it must not be used for caching.
	AppendSample(ref storage.SeriesRef, se series.Series, sa Sample, opts *AppendOptions) (storage.SeriesRef, error)
	// Append(ref storage.SeriesRef, se series.Series, sa Sample,	ct, t int64, h *histogram.Histogram, fh *histogram.FloatHistogram, v float64,  exemplars []exemplar.Exemplar, opts *AppendOptions) (storage.SeriesRef, error)
}

type AppendOptions struct {
	DiscardOutOfOrder bool
}
