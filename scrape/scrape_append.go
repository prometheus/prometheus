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

package scrape

import (
	"time"

	"github.com/prometheus/prometheus/model/histogram"
	"github.com/prometheus/prometheus/model/labels"
	"github.com/prometheus/prometheus/model/timestamp"
	"github.com/prometheus/prometheus/model/value"
	"github.com/prometheus/prometheus/storage"
)

// appenderWithLimits returns an appender with additional validation.
func appenderWithLimits(app storage.Appender, sampleLimit, bucketLimit int, maxSchema int32) storage.Appender {
	app = &timeLimitAppender{
		Appender: app,
		maxTime:  timestamp.FromTime(time.Now().Add(maxAheadTime)),
	}

	// The sampleLimit is applied after metrics are potentially dropped via relabeling.
	if sampleLimit > 0 {
		app = &limitAppender{
			Appender: app,
			limit:    sampleLimit,
		}
	}

	if bucketLimit > 0 {
		app = &bucketLimitAppender{
			Appender: app,
			limit:    bucketLimit,
		}
	}

	if maxSchema < histogram.ExponentialSchemaMax {
		app = &maxSchemaAppender{
			Appender:  app,
			maxSchema: maxSchema,
		}
	}

	return app
}

// limitAppender limits the number of total appended samples in a batch.
type limitAppender struct {
	storage.Appender

	limit int
	i     int
}

func (app *limitAppender) Append(ref storage.SeriesRef, lset labels.Labels, t int64, v float64) (storage.SeriesRef, error) {
	// Bypass sample_limit checks only if we have a staleness marker for a known series (ref value is non-zero).
	// This ensures that if a series is already in TSDB then we always write the marker.
	if ref == 0 || !value.IsStaleNaN(v) {
		app.i++
		if app.i > app.limit {
			return 0, errSampleLimit
		}
	}
	ref, err := app.Appender.Append(ref, lset, t, v)
	if err != nil {
		return 0, err
	}
	return ref, nil
}

func (app *limitAppender) AppendHistogram(ref storage.SeriesRef, lset labels.Labels, t int64, h *histogram.Histogram, fh *histogram.FloatHistogram) (storage.SeriesRef, error) {
	// Bypass sample_limit checks only if we have a staleness marker for a known series (ref value is non-zero).
	// This ensures that if a series is already in TSDB then we always write the marker.
	if ref == 0 || (h != nil && !value.IsStaleNaN(h.Sum)) || (fh != nil && !value.IsStaleNaN(fh.Sum)) {
		app.i++
		if app.i > app.limit {
			return 0, errSampleLimit
		}
	}
	ref, err := app.Appender.AppendHistogram(ref, lset, t, h, fh)
	if err != nil {
		return 0, err
	}
	return ref, nil
}

type timeLimitAppender struct {
	storage.Appender

	maxTime int64
}

func (app *timeLimitAppender) Append(ref storage.SeriesRef, lset labels.Labels, t int64, v float64) (storage.SeriesRef, error) {
	if t > app.maxTime {
		return 0, storage.ErrOutOfBounds
	}

	ref, err := app.Appender.Append(ref, lset, t, v)
	if err != nil {
		return 0, err
	}
	return ref, nil
}

// bucketLimitAppender limits the number of total appended samples in a batch.
type bucketLimitAppender struct {
	storage.Appender

	limit int
}

func (app *bucketLimitAppender) AppendHistogram(ref storage.SeriesRef, lset labels.Labels, t int64, h *histogram.Histogram, fh *histogram.FloatHistogram) (storage.SeriesRef, error) {
	var err error
	if h != nil {
		// Return with an early error if the histogram has too many buckets and the
		// schema is not exponential, in which case we can't reduce the resolution.
		if len(h.PositiveBuckets)+len(h.NegativeBuckets) > app.limit && !histogram.IsExponentialSchema(h.Schema) {
			return 0, errBucketLimit
		}
		for len(h.PositiveBuckets)+len(h.NegativeBuckets) > app.limit {
			if h.Schema <= histogram.ExponentialSchemaMin {
				return 0, errBucketLimit
			}
			if err = h.ReduceResolution(h.Schema - 1); err != nil {
				return 0, err
			}
		}
	}
	if fh != nil {
		// Return with an early error if the histogram has too many buckets and the
		// schema is not exponential, in which case we can't reduce the resolution.
		if len(fh.PositiveBuckets)+len(fh.NegativeBuckets) > app.limit && !histogram.IsExponentialSchema(fh.Schema) {
			return 0, errBucketLimit
		}
		for len(fh.PositiveBuckets)+len(fh.NegativeBuckets) > app.limit {
			if fh.Schema <= histogram.ExponentialSchemaMin {
				return 0, errBucketLimit
			}
			if err = fh.ReduceResolution(fh.Schema - 1); err != nil {
				return 0, err
			}
		}
	}
	if ref, err = app.Appender.AppendHistogram(ref, lset, t, h, fh); err != nil {
		return 0, err
	}
	return ref, nil
}

type maxSchemaAppender struct {
	storage.Appender

	maxSchema int32
}

func (app *maxSchemaAppender) AppendHistogram(ref storage.SeriesRef, lset labels.Labels, t int64, h *histogram.Histogram, fh *histogram.FloatHistogram) (storage.SeriesRef, error) {
	var err error
	if h != nil {
		if histogram.IsExponentialSchemaReserved(h.Schema) && h.Schema > app.maxSchema {
			if err = h.ReduceResolution(app.maxSchema); err != nil {
				return 0, err
			}
		}
	}
	if fh != nil {
		if histogram.IsExponentialSchemaReserved(fh.Schema) && fh.Schema > app.maxSchema {
			if err = fh.ReduceResolution(app.maxSchema); err != nil {
				return 0, err
			}
		}
	}
	if ref, err = app.Appender.AppendHistogram(ref, lset, t, h, fh); err != nil {
		return 0, err
	}
	return ref, nil
}
