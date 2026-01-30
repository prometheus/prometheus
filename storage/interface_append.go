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

package storage

import (
	"context"
	"errors"

	"github.com/prometheus/prometheus/model/exemplar"
	"github.com/prometheus/prometheus/model/histogram"
	"github.com/prometheus/prometheus/model/labels"
	"github.com/prometheus/prometheus/model/metadata"
)

// AppendableV2 allows creating AppenderV2.
type AppendableV2 interface {
	// AppenderV2 returns a new appender for the storage.
	//
	// Implementations CAN choose whether to use the context e.g. for deadlines,
	// but it's not mandatory.
	AppenderV2(ctx context.Context) AppenderV2
}

// AOptions is a shorthand for AppendV2Options.
// NOTE: AppendOption is used already.
type AOptions = AppendV2Options

// AppendV2Options provides optional, auxiliary data and configuration for AppenderV2.Append.
type AppendV2Options struct {
	// MetricFamilyName (optional) provides metric family name for the appended sample's
	// series. If the client of the AppenderV2 has this information
	// (e.g. from scrape) it's recommended to pass it to the appender.
	//
	// Provided string bytes are unsafe to reuse, it only lives for the duration of the Append call.
	//
	// Some implementations use this to avoid slow and prone to error metric family detection for:
	// * Metadata per metric family storages (e.g. Prometheus metadata WAL/API/RW1)
	// * Strictly complex types storages (e.g. OpenTelemetry Collector).
	//
	// NOTE(krajorama): Example purpose is highlighted in OTLP ingestion: OTLP calculates the
	// metric family name for all metrics and uses it for generating summary,
	// histogram series by adding the magic suffixes. The metric family name is
	// passed down to the appender in case the storage needs it for metadata updates.
	// Known user of this is Mimir that implements /api/v1/metadata and uses
	// Remote-Write 1.0 for this. Might be removed later if no longer
	// needed by any downstream project.
	// NOTE(bwplotka): Long term, once Prometheus uses complex types on storage level
	// the MetricFamilyName can be removed as MetricFamilyName will equal to __name__ always.
	MetricFamilyName string

	// Metadata (optional) attached to the appended sample.
	// Metadata strings are safe for reuse.
	// IMPORTANT: Appender v1 was only providing update. This field MUST be
	// set (if known) even if it didn't change since the last iteration.
	// This moves the responsibility for metadata storage options to TSDB.
	Metadata metadata.Metadata

	// Exemplars (optional) attached to the appended sample.
	// Exemplar slice MUST be sorted by Exemplar.TS.
	// Exemplar slice is unsafe for reuse.
	// Duplicate exemplars errors MUST be ignored by implementations.
	Exemplars []exemplar.Exemplar

	// RejectOutOfOrder tells implementation that this append should not be out
	// of order. An OOO append MUST be rejected with storage.ErrOutOfOrderSample
	// error.
	RejectOutOfOrder bool
}

// AppendPartialError represents an AppenderV2.Append error that tells
// callers sample was written but some auxiliary optional data (e.g. exemplars)
// was not (or partially written)
//
// It's up to the caller to decide if it's an ignorable error or not, plus
// it allows extra reporting (e.g. for Remote Write 2.0 X-Remote-Write-Written headers).
type AppendPartialError struct {
	ExemplarErrors []error
}

// Error returns combined error string.
func (e *AppendPartialError) Error() string {
	errs := errors.Join(e.ExemplarErrors...)
	if errs == nil {
		return ""
	}
	return errs.Error()
}

// ErrOrNil returns AppendPartialError as error, returning nil
// if there are no errors.
func (e *AppendPartialError) ErrOrNil() error {
	if len(e.ExemplarErrors) == 0 {
		return nil
	}
	return e
}

// Handle handles the given err that may be an AppendPartialError.
// If the err is nil or not an AppendPartialError it returns err.
// Otherwise, partial errors are aggregated.
func (e *AppendPartialError) Handle(err error) error {
	if err == nil {
		return nil
	}

	var pErr *AppendPartialError
	if !errors.As(err, &pErr) {
		return err
	}
	e.ExemplarErrors = append(e.ExemplarErrors, pErr.ExemplarErrors...)
	return nil
}

var _ error = &AppendPartialError{}

// AppenderV2 provides appends against a storage for all types of samples.
// It must be completed with a call to Commit or Rollback and must not be reused afterwards.
//
// Operations on the AppenderV2 interface are not goroutine-safe.
//
// The order of samples appended via the AppenderV2 is preserved within each series.
// I.e. timestamp order within batch is not validated, samples are not reordered per timestamp or by float/histogram
// type.
type AppenderV2 interface {
	AppenderTransaction

	// Append appends a sample and related exemplars, metadata, and start timestamp (st) to the storage.
	//
	// ref (optional) represents the stable ID for the given series identified by ls (excluding metadata).
	// Callers MAY provide the ref to help implementation avoid ls -> ref computation, otherwise ref MUST be 0 (unknown).
	//
	// ls represents labels for the sample's series.
	//
	// st (optional) represents sample start timestamp. 0 means unknown. Implementations
	// are responsible for any potential ST storage logic (e.g. ST zero injections).
	//
	// t represents sample timestamp.
	//
	// v, h, fh represents sample value for each sample type.
	// Callers MUST only provide one of the sample types (either v, h or fh).
	// Implementations can detect the type of the sample with the following switch:
	//
	// switch {
	//  case fh != nil: It's a float histogram append.
	//  case h != nil: It's a histogram append.
	//  default: It's a float append.
	// }
	// TODO(bwplotka): We plan to experiment on using generics for complex sampleType, but do it after we unify interface (derisk) and before we add native summaries.
	//
	// Implementations MUST attempt to append sample even if metadata, exemplar or (st) start timestamp appends fail.
	// Implementations MAY return AppendPartialError as an error. Use errors.As to detect.
	// For the successful Append, Implementations MUST return valid SeriesRef that represents ls.
	//   NOTE(bwplotka): Given OTLP and native histograms and the relaxation of the requirement for
	//   type and unit suffixes in metric names we start to hit cases of ls being not enough for id
	//   of the series (metadata matters). Current solution is to enable 'type-and-unit-label' features for those cases, but we may
	//   start to extend the id with metadata one day.
	Append(ref SeriesRef, ls labels.Labels, st, t int64, v float64, h *histogram.Histogram, fh *histogram.FloatHistogram, opts AppendV2Options) (SeriesRef, error)
}

// AppenderTransaction allows transactional appends.
type AppenderTransaction interface {
	// Commit submits the collected samples and purges the batch. If Commit
	// returns a non-nil error, it also rolls back all modifications made in
	// the appender so far, as Rollback would do. In any case, an Appender
	// must not be used anymore after Commit has been called.
	Commit() error

	// Rollback rolls back all modifications made in the appender so far.
	// Appender has to be discarded after rollback.
	Rollback() error
}

// LimitedAppenderV1 is an Appender that only supports appending float and histogram samples.
// This is to support migration to AppenderV2.
// TODO(bwplotka): Remove once migration to AppenderV2 is fully complete.
type LimitedAppenderV1 interface {
	Append(ref SeriesRef, l labels.Labels, t int64, v float64) (SeriesRef, error)
	AppendHistogram(ref SeriesRef, l labels.Labels, t int64, h *histogram.Histogram, fh *histogram.FloatHistogram) (SeriesRef, error)
}

// AppenderV2AsLimitedV1 returns appender that exposes AppenderV2 as LimitedAppenderV1
// TODO(bwplotka): Remove once migration to AppenderV2 is fully complete.
func AppenderV2AsLimitedV1(app AppenderV2) LimitedAppenderV1 {
	return &limitedAppenderV1{AppenderV2: app}
}

type limitedAppenderV1 struct {
	AppenderV2
}

func (a *limitedAppenderV1) Append(ref SeriesRef, l labels.Labels, t int64, v float64) (SeriesRef, error) {
	return a.AppenderV2.Append(ref, l, 0, t, v, nil, nil, AppendV2Options{})
}

func (a *limitedAppenderV1) AppendHistogram(ref SeriesRef, l labels.Labels, t int64, h *histogram.Histogram, fh *histogram.FloatHistogram) (SeriesRef, error) {
	return a.AppenderV2.Append(ref, l, 0, t, 0, h, fh, AppendV2Options{})
}
