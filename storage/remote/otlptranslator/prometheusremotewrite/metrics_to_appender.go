// Copyright 2025 The Prometheus Authors
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
// Provenance-includes-location: https://github.com/open-telemetry/opentelemetry-collector-contrib/blob/95e8f8fdc2a9dc87230406c9a3cf02be4fd68bea/pkg/translator/prometheusremotewrite/metrics_to_prw.go
// Provenance-includes-license: Apache-2.0
// Provenance-includes-copyright: Copyright The OpenTelemetry Authors.

package prometheusremotewrite

import (
	"bytes"
	"context"
	"encoding/hex"
	"errors"
	"fmt"
	"math"
	"slices"
	"sort"
	"strconv"
	"unicode/utf8"

	"github.com/prometheus/common/model"
	"github.com/prometheus/otlptranslator"
	"go.opentelemetry.io/collector/pdata/pcommon"
	"go.opentelemetry.io/collector/pdata/pmetric"
	conventions "go.opentelemetry.io/collector/semconv/v1.6.1"
	"go.uber.org/multierr"

	"github.com/prometheus/prometheus/model/exemplar"
	"github.com/prometheus/prometheus/model/histogram"
	"github.com/prometheus/prometheus/model/labels"
	"github.com/prometheus/prometheus/model/timestamp"
	"github.com/prometheus/prometheus/model/value"
	"github.com/prometheus/prometheus/storage"
	"github.com/prometheus/prometheus/util/annotations"
)

type OTLPToAppender struct {
	app     storage.Appender
	everyN  everyNTimes
	builder labels.ScratchBuilder
}

func NewOTLPToAppender(app storage.Appender) *OTLPToAppender {
	return &OTLPToAppender{
		app: app,
	}
}

func (c *OTLPToAppender) FromMetrics(ctx context.Context, md pmetric.Metrics, settings Settings) (annots annotations.Annotations, errs error) {
	namer := otlptranslator.MetricNamer{
		Namespace:          settings.Namespace,
		WithMetricSuffixes: settings.AddMetricSuffixes,
		UTF8Allowed:        settings.AllowUTF8,
	}
	c.everyN = everyNTimes{n: 128}
	resourceMetricsSlice := md.ResourceMetrics()

	numMetrics := 0
	for i := 0; i < resourceMetricsSlice.Len(); i++ {
		scopeMetricsSlice := resourceMetricsSlice.At(i).ScopeMetrics()
		for j := 0; j < scopeMetricsSlice.Len(); j++ {
			numMetrics += scopeMetricsSlice.At(j).Metrics().Len()
		}
	}

	for i := 0; i < resourceMetricsSlice.Len(); i++ {
		resourceMetrics := resourceMetricsSlice.At(i)
		resource := resourceMetrics.Resource()
		scopeMetricsSlice := resourceMetrics.ScopeMetrics()
		// keep track of the most recent timestamp in the ResourceMetrics for
		// use with the "target" info metric
		var mostRecentTimestamp pcommon.Timestamp
		for j := 0; j < scopeMetricsSlice.Len(); j++ {
			scopeMetrics := scopeMetricsSlice.At(j)
			scope := newScopeFromScopeMetrics(scopeMetrics)
			metricSlice := scopeMetrics.Metrics()

			// TODO: decide if instrumentation library information should be exported as labels
			for k := 0; k < metricSlice.Len(); k++ {
				if err := c.everyN.checkContext(ctx); err != nil {
					errs = multierr.Append(errs, err)
					return
				}

				metric := metricSlice.At(k)
				mostRecentTimestamp = max(mostRecentTimestamp, mostRecentTimestampInMetric(metric))
				temporality, hasTemporality, err := aggregationTemporality(metric)
				if err != nil {
					errs = multierr.Append(errs, err)
					continue
				}

				if hasTemporality &&
					// Cumulative temporality is always valid.
					// Delta temporality is also valid if AllowDeltaTemporality is true.
					// All other temporality values are invalid.
					(temporality != pmetric.AggregationTemporalityCumulative &&
						(!settings.AllowDeltaTemporality || temporality != pmetric.AggregationTemporalityDelta)) {
					errs = multierr.Append(errs, fmt.Errorf("invalid temporality and type combination for metric %q", metric.Name()))
					continue
				}

				promName := namer.Build(TranslatorMetricFromOtelMetric(metric))
				// metadata := prompb.MetricMetadata{
				// 	Type:             otelMetricTypeToPromMetricType(metric),
				// 	MetricFamilyName: promName,
				// 	Help:             metric.Description(),
				// 	Unit:             metric.Unit(),
				// }

				// handle individual metrics based on type
				//exhaustive:enforce
				switch metric.Type() {
				case pmetric.MetricTypeGauge:
					dataPoints := metric.Gauge().DataPoints()
					if dataPoints.Len() == 0 {
						errs = multierr.Append(errs, fmt.Errorf("empty data points. %s is dropped", metric.Name()))
						break
					}
					if err := c.addGaugeNumberDataPoints(ctx, dataPoints, resource, settings, promName, scope); err != nil {
						errs = multierr.Append(errs, err)
						if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
							return
						}
					}
				case pmetric.MetricTypeSum:
					dataPoints := metric.Sum().DataPoints()
					if dataPoints.Len() == 0 {
						errs = multierr.Append(errs, fmt.Errorf("empty data points. %s is dropped", metric.Name()))
						break
					}
					if err := c.addSumNumberDataPoints(ctx, dataPoints, resource, metric, settings, promName, scope); err != nil {
						errs = multierr.Append(errs, err)
						if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
							return
						}
					}
				case pmetric.MetricTypeHistogram:
					dataPoints := metric.Histogram().DataPoints()
					if dataPoints.Len() == 0 {
						errs = multierr.Append(errs, fmt.Errorf("empty data points. %s is dropped", metric.Name()))
						break
					}
					if settings.ConvertHistogramsToNHCB {
						ws, err := c.addCustomBucketsTSDBHistogramDataPoints(
							ctx, dataPoints, resource, settings, promName, temporality, scope,
						)
						annots.Merge(ws)
						if err != nil {
							errs = multierr.Append(errs, err)
							if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
								return
							}
						}
					} else {
						if err := c.addHistogramDataPoints(ctx, dataPoints, resource, settings, promName, scope); err != nil {
							errs = multierr.Append(errs, err)
							if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
								return
							}
						}
					}
				case pmetric.MetricTypeExponentialHistogram:
					dataPoints := metric.ExponentialHistogram().DataPoints()
					if dataPoints.Len() == 0 {
						errs = multierr.Append(errs, fmt.Errorf("empty data points. %s is dropped", metric.Name()))
						break
					}
					ws, err := c.addExponentialHistogramDataPoints(
						ctx,
						dataPoints,
						resource,
						settings,
						promName,
						temporality,
						scope,
					)
					annots.Merge(ws)
					if err != nil {
						errs = multierr.Append(errs, err)
						if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
							return
						}
					}
				case pmetric.MetricTypeSummary:
					dataPoints := metric.Summary().DataPoints()
					if dataPoints.Len() == 0 {
						errs = multierr.Append(errs, fmt.Errorf("empty data points. %s is dropped", metric.Name()))
						break
					}
					if err := c.addSummaryDataPoints(ctx, dataPoints, resource, settings, promName, scope); err != nil {
						errs = multierr.Append(errs, err)
						if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
							return
						}
					}
				default:
					errs = multierr.Append(errs, errors.New("unsupported metric type"))
				}

			}

		}
		addResourceTargetInfoTSDB(resource, settings, mostRecentTimestamp, c)
	}

	return annots, errs
}

func (c *OTLPToAppender) addGaugeNumberDataPoints(ctx context.Context, dataPoints pmetric.NumberDataPointSlice,
	resource pcommon.Resource, settings Settings, name string, scope scope,
) error {
	var ref storage.SeriesRef
	var err error
	var prevLabelsBuf, currLabelsBuf []byte
	for x := 0; x < dataPoints.Len(); x++ {
		if err := c.everyN.checkContext(ctx); err != nil {
			return err
		}

		pt := dataPoints.At(x)
		currLabels := c.toLabels(name, loadAttributes(
			resource,
			pt.Attributes(),
			scope,
			settings,
			nil,
			true,
		))
		currLabelsBuf = currLabels.Bytes(currLabelsBuf)
		if !bytes.Equal(prevLabelsBuf, currLabelsBuf) {
			// If the labels have changed, we need to reset the reference.
			ref = 0
			prevLabelsBuf = currLabelsBuf
		}
		var v float64
		switch pt.ValueType() {
		case pmetric.NumberDataPointValueTypeInt:
			v = float64(pt.IntValue())
		case pmetric.NumberDataPointValueTypeDouble:
			v = pt.DoubleValue()
		}
		if pt.Flags().NoRecordedValue() {
			v = math.Float64frombits(value.StaleNaN)
		}
		ref, err = c.appendSample(ref, currLabels, convertTimeStamp(pt.Timestamp()), v)
		if err != nil {
			return fmt.Errorf("failed to add gauge number data point: %w", err)
		}
	}

	return nil
}

func (c *OTLPToAppender) addSumNumberDataPoints(ctx context.Context, dataPoints pmetric.NumberDataPointSlice,
	resource pcommon.Resource, metric pmetric.Metric, settings Settings, name string, scope scope,
) error {
	var ref storage.SeriesRef
	var err error
	var prevLabelsBuf, currLabelsBuf []byte
	for x := 0; x < dataPoints.Len(); x++ {
		if err := c.everyN.checkContext(ctx); err != nil {
			return err
		}

		pt := dataPoints.At(x)
		currLabels := c.toLabels(name, loadAttributes(
			resource,
			pt.Attributes(),
			scope,
			settings,
			nil,
			true,
		))
		currLabelsBuf = currLabels.Bytes(currLabelsBuf)
		if !bytes.Equal(prevLabelsBuf, currLabelsBuf) {
			// If the labels have changed, we need to reset the reference.
			ref = 0
			prevLabelsBuf = currLabelsBuf
		}
		var v float64
		switch pt.ValueType() {
		case pmetric.NumberDataPointValueTypeInt:
			v = float64(pt.IntValue())
		case pmetric.NumberDataPointValueTypeDouble:
			v = pt.DoubleValue()
		}
		if pt.Flags().NoRecordedValue() {
			v = math.Float64frombits(value.StaleNaN)
		}
		ref, err = c.appendSample(ref, currLabels, convertTimeStamp(pt.Timestamp()), v)
		if err != nil {
			return fmt.Errorf("failed to add sum number data point: %w", err)
		}

		exemplars, err := loadExemplars(c, ctx, &c.everyN, pt)
		if err != nil {
			return err
		}
		err = c.appendExemplars(ctx, &c.everyN, ref, currLabels, exemplars)
		if err != nil {
			return err
		}

		// add created time series if needed
		// TODO
		// if settings.ExportCreatedMetric && metric.Sum().IsMonotonic() {
		// 	startTimestamp := pt.StartTimestamp()
		// 	if startTimestamp == 0 {
		// 		return nil
		// 	}

		// 	createdLabels := make([]prompb.Label, len(lbls))
		// 	copy(createdLabels, lbls)
		// 	for i, l := range createdLabels {
		// 		if l.Name == model.MetricNameLabel {
		// 			createdLabels[i].Value = name + createdSuffix
		// 			break
		// 		}
		// 	}
		// 	c.addTimeSeriesIfNeeded(createdLabels, startTimestamp, pt.Timestamp())
		// }
	}

	return nil
}

// addHistogramDataPoints adds OTel histogram data points to the corresponding Prometheus time series
// as classical histogram samples.
//
// Note that we can't convert to native histograms, since these have exponential buckets and don't line up
// with the user defined bucket boundaries of non-exponential OTel histograms.
// However, work is under way to resolve this shortcoming through a feature called native histograms custom buckets:
// https://github.com/prometheus/prometheus/issues/13485.
func (c *OTLPToAppender) addHistogramDataPoints(ctx context.Context, dataPoints pmetric.HistogramDataPointSlice,
	resource pcommon.Resource, settings Settings, baseName string, scope scope,
) error {
	for x := 0; x < dataPoints.Len(); x++ {
		if err := c.everyN.checkContext(ctx); err != nil {
			return err
		}

		pt := dataPoints.At(x)
		timestamp := convertTimeStamp(pt.Timestamp())
		currLabels := loadAttributes(resource, pt.Attributes(), scope, settings, nil, false)
		// baseLabels := createAttributes(resource, pt.Attributes(), scope, settings, nil, false)

		// If the sum is unset, it indicates the _sum metric point should be
		// omitted
		if pt.HasSum() {
			// treat sum as a sample in an individual TimeSeries
			sum := pt.Sum()
			if pt.Flags().NoRecordedValue() {
				sum = math.Float64frombits(value.StaleNaN)
			}
			_, err := c.appendSample(0, c.toLabels(baseName+sumStr, currLabels), timestamp, sum)
			if err != nil {
				return fmt.Errorf("failed to add histogram sum data point: %w", err)
			}
		}

		// treat count as a sample in an individual TimeSeries
		count := float64(pt.Count())
		if pt.Flags().NoRecordedValue() {
			count = math.Float64frombits(value.StaleNaN)
		}
		_, err := c.appendSample(0, c.toLabels(baseName+countStr, currLabels), timestamp, count)
		if err != nil {
			return fmt.Errorf("failed to add histogram count data point: %w", err)
		}

		// cumulative count for conversion to cumulative histogram
		var cumulativeCount uint64

		var bucketBounds []bucketBoundsDataTSDB

		// process each bound, based on histograms proto definition, # of buckets = # of explicit bounds + 1
		for i := 0; i < pt.ExplicitBounds().Len() && i < pt.BucketCounts().Len(); i++ {
			if err := c.everyN.checkContext(ctx); err != nil {
				return err
			}

			bound := pt.ExplicitBounds().At(i)
			cumulativeCount += pt.BucketCounts().At(i)
			bucket := float64(cumulativeCount)
			if pt.Flags().NoRecordedValue() {
				bucket = math.Float64frombits(value.StaleNaN)
			}
			boundStr := strconv.FormatFloat(bound, 'f', -1, 64)
			currLabels[model.BucketLabel] = boundStr
			bucketLabels := c.toLabels(baseName+bucketStr, currLabels)
			ref, err := c.appendSample(0, bucketLabels, timestamp, bucket)
			if err != nil {
				return fmt.Errorf("failed to add histogram bucket data point: %w", err)
			}

			bucketBounds = append(bucketBounds, bucketBoundsDataTSDB{ref: ref, labels: bucketLabels, bound: bound})
		}
		// add le=+Inf bucket
		infBucket := float64(pt.Count())
		if pt.Flags().NoRecordedValue() {
			infBucket = math.Float64frombits(value.StaleNaN)
		}
		currLabels[model.BucketLabel] = pInfStr
		bucketLabels := c.toLabels(baseName+bucketStr, currLabels)
		ref, err := c.appendSample(0, c.toLabels(baseName+bucketStr, currLabels), timestamp, infBucket)
		if err != nil {
			return fmt.Errorf("failed to add histogram bucket data point: %w", err)
		}
		bucketBounds = append(bucketBounds, bucketBoundsDataTSDB{ref: ref, labels: bucketLabels, bound: math.Inf(1)})

		exemplars, err := loadExemplars(c, ctx, &c.everyN, pt)
		if err != nil {
			return fmt.Errorf("failed to load exemplars: %w", err)
		}

		if err := c.addExemplars(ctx, pt, exemplars, bucketBounds); err != nil {
			return err
		}

		// startTimestamp := pt.StartTimestamp()
		// if settings.ExportCreatedMetric && startTimestamp != 0 {
		// 	labels := createLabels(baseName+createdSuffix, baseLabels)
		// 	c.addTimeSeriesIfNeeded(labels, startTimestamp, pt.Timestamp())
		// }
	}

	return nil
}

func (c *OTLPToAppender) addSummaryDataPoints(ctx context.Context, dataPoints pmetric.SummaryDataPointSlice, resource pcommon.Resource,
	settings Settings, baseName string, scope scope,
) error {
	for x := 0; x < dataPoints.Len(); x++ {
		if err := c.everyN.checkContext(ctx); err != nil {
			return err
		}

		pt := dataPoints.At(x)
		timestamp := convertTimeStamp(pt.Timestamp())
		currLabels := loadAttributes(resource, pt.Attributes(), scope, settings, nil, false)

		// treat sum as a sample in an individual TimeSeries
		sum := pt.Sum()
		if pt.Flags().NoRecordedValue() {
			sum = math.Float64frombits(value.StaleNaN)
		}
		// sum and count of the summary should append suffix to baseName
		_, err := c.appendSample(0, c.toLabels(baseName+sumStr, currLabels), timestamp, sum)
		if err != nil {
			return fmt.Errorf("failed to add summary sum data point: %w", err)
		}

		// treat count as a sample in an individual TimeSeries
		count := float64(pt.Count())
		if pt.Flags().NoRecordedValue() {
			count = math.Float64frombits(value.StaleNaN)
		}
		_, err = c.appendSample(0, c.toLabels(baseName+countStr, currLabels), timestamp, count)
		if err != nil {
			return fmt.Errorf("failed to add summary count data point: %w", err)
		}

		// process each percentile/quantile
		for i := 0; i < pt.QuantileValues().Len(); i++ {
			qt := pt.QuantileValues().At(i)
			quantile := qt.Value()
			if pt.Flags().NoRecordedValue() {
				quantile = math.Float64frombits(value.StaleNaN)
			}
			percentileStr := strconv.FormatFloat(qt.Quantile(), 'f', -1, 64)
			currLabels[model.QuantileLabel] = percentileStr
			_, err := c.appendSample(0, c.toLabels(baseName, currLabels), timestamp, quantile)
			if err != nil {
				return fmt.Errorf("failed to add summary quantile data point: %w", err)
			}
		}

		// startTimestamp := pt.StartTimestamp()
		// if settings.ExportCreatedMetric && startTimestamp != 0 {
		// 	createdLabels := createLabels(baseName+createdSuffix, baseLabels)
		// 	c.addTimeSeriesIfNeeded(createdLabels, startTimestamp, pt.Timestamp())
		// }
	}

	return nil
}

// addExemplars adds exemplars for the dataPoint. For each exemplar, if it can find a bucket bound corresponding to its value,
// the exemplar is added to the bucket bound's time series, provided that the time series' has samples.
func (c *OTLPToAppender) addExemplars(ctx context.Context, dataPoint pmetric.HistogramDataPoint, exemplars []exemplar.Exemplar, bucketBounds []bucketBoundsDataTSDB) error {
	if len(bucketBounds) == 0 || len(exemplars) == 0 {
		return nil
	}

	sort.Sort(byBucketBoundsDataTSDB(bucketBounds))
	for _, exemplar := range exemplars {
		for _, bound := range bucketBounds {
			if err := c.everyN.checkContext(ctx); err != nil {
				return err
			}
			if exemplar.Value <= bound.bound {
				c.app.AppendExemplar(bound.ref, bound.labels, exemplar)
				break
			}
		}
	}

	return nil
}

type bucketBoundsDataTSDB struct {
	ref    storage.SeriesRef
	labels labels.Labels
	bound  float64
}

// byBucketBoundsData enables the usage of sort.Sort() with a slice of bucket bounds.
type byBucketBoundsDataTSDB []bucketBoundsDataTSDB

func (m byBucketBoundsDataTSDB) Len() int           { return len(m) }
func (m byBucketBoundsDataTSDB) Less(i, j int) bool { return m[i].bound < m[j].bound }
func (m byBucketBoundsDataTSDB) Swap(i, j int)      { m[i], m[j] = m[j], m[i] }

func (c *OTLPToAppender) addCustomBucketsTSDBHistogramDataPoints(ctx context.Context, dataPoints pmetric.HistogramDataPointSlice,
	resource pcommon.Resource, settings Settings, promName string, temporality pmetric.AggregationTemporality,
	scope scope,
) (annotations.Annotations, error) {
	var annots annotations.Annotations
	var ref storage.SeriesRef
	var prevLabelsBuf, currLabelsBuf []byte
	for x := 0; x < dataPoints.Len(); x++ {
		if err := c.everyN.checkContext(ctx); err != nil {
			return annots, err
		}

		pt := dataPoints.At(x)

		h, ws, err := explicitHistogramToCustomBucketsTSDBHistogram(pt, temporality)
		annots.Merge(ws)
		if err != nil {
			return annots, err
		}

		currLabels := c.toLabels(promName, loadAttributes(
			resource,
			pt.Attributes(),
			scope,
			settings,
			nil,
			true,
		))
		currLabelsBuf = currLabels.Bytes(currLabelsBuf)
		if !bytes.Equal(prevLabelsBuf, currLabelsBuf) {
			// If the labels have changed, we need to reset the reference.
			ref = 0
			prevLabelsBuf = currLabelsBuf
		}

		ref, err = c.appendHistogram(ref, currLabels, convertTimeStamp(pt.Timestamp()), h)
		if err != nil {
			return annots, fmt.Errorf("failed to add custom buckets histogram data point: %w", err)
		}

		exemplars, err := loadExemplars(c, ctx, &c.everyN, pt)
		if err != nil {
			return annots, fmt.Errorf("failed to load exemplars: %w", err)
		}
		err = c.appendExemplars(ctx, &c.everyN, ref, currLabels, exemplars)
		if err != nil {
			return annots, fmt.Errorf("failed to add exemplars: %w", err)
		}
	}

	return annots, nil
}

func explicitHistogramToCustomBucketsTSDBHistogram(p pmetric.HistogramDataPoint, temporality pmetric.AggregationTemporality) (*histogram.Histogram, annotations.Annotations, error) {
	var annots annotations.Annotations

	buckets := p.BucketCounts().AsRaw()
	offset := getBucketOffset(buckets)
	bucketCounts := buckets[offset:]
	positiveSpans, positiveDeltas := convertBucketsLayoutTSDB(bucketCounts, int32(offset), 0, false)

	// The counter reset detection must be compatible with Prometheus to
	// safely set ResetHint to NO. This is not ensured currently.
	// Sending a sample that triggers counter reset but with ResetHint==NO
	// would lead to Prometheus panic as it does not double check the hint.
	// Thus we're explicitly saying UNKNOWN here, which is always safe.
	// TODO: using created time stamp should be accurate, but we
	// need to know here if it was used for the detection.
	// Ref: https://github.com/open-telemetry/opentelemetry-collector-contrib/pull/28663#issuecomment-1810577303
	// Counter reset detection in Prometheus: https://github.com/prometheus/prometheus/blob/f997c72f294c0f18ca13fa06d51889af04135195/tsdb/chunkenc/histogram.go#L232
	resetHint := histogram.UnknownCounterReset

	if temporality == pmetric.AggregationTemporalityDelta {
		// If the histogram has delta temporality, set the reset hint to gauge to avoid unnecessary chunk cutting.
		// We're in an early phase of implementing delta support (proposal: https://github.com/prometheus/proposals/pull/48/).
		// This might be changed to a different hint name as gauge type might be misleading for samples that should be
		// summed over time.
		resetHint = histogram.GaugeType
	}

	// TODO(carrieedwards): Add setting to limit maximum bucket count
	h := &histogram.Histogram{
		CounterResetHint: resetHint,
		Schema:           histogram.CustomBucketsSchema,

		PositiveSpans:   positiveSpans,
		PositiveBuckets: positiveDeltas,
		// Note: OTel explicit histograms have an implicit +Inf bucket, which has a lower bound
		// of the last element in the explicit_bounds array.
		// This is similar to the custom_values array in native histograms with custom buckets.
		// Because of this shared property, the OTel explicit histogram's explicit_bounds array
		// can be mapped directly to the custom_values array.
		// See: https://github.com/open-telemetry/opentelemetry-proto/blob/d7770822d70c7bd47a6891fc9faacc66fc4af3d3/opentelemetry/proto/metrics/v1/metrics.proto#L469
		CustomValues: p.ExplicitBounds().AsRaw(),
	}

	if p.Flags().NoRecordedValue() {
		h.Sum = math.Float64frombits(value.StaleNaN)
	} else {
		if p.HasSum() {
			h.Sum = p.Sum()
		}
		h.Count = p.Count()
		if p.Count() == 0 && h.Sum != 0 {
			annots.Add(fmt.Errorf("histogram data point has zero count, but non-zero sum: %f", h.Sum))
		}
	}
	return h, annots, nil
}

// convertBucketsLayout translates OTel Explicit or Exponential Histogram dense buckets
// representation to Prometheus Native Histogram sparse bucket representation. This is used
// for translating Exponential Histograms into Native Histograms, and Explicit Histograms
// into Native Histograms with Custom Buckets.
//
// The translation logic is taken from the client_golang `histogram.go#makeBuckets`
// function, see `makeBuckets` https://github.com/prometheus/client_golang/blob/main/prometheus/histogram.go
//
// scaleDown is the factor by which the buckets are scaled down. In other words 2^scaleDown buckets will be merged into one.
//
// When converting from OTel Exponential Histograms to Native Histograms, the
// bucket indexes conversion is adjusted, since OTel exp. histogram bucket
// index 0 corresponds to the range (1, base] while Prometheus bucket index 0
// to the range (base 1].
//
// When converting from OTel Explicit Histograms to Native Histograms with Custom Buckets,
// the bucket indexes are not scaled, and the indices are not adjusted by 1.
func convertBucketsLayoutTSDB(bucketCounts []uint64, offset, scaleDown int32, adjustOffset bool) ([]histogram.Span, []int64) {
	if len(bucketCounts) == 0 {
		return nil, nil
	}

	var (
		spans     []histogram.Span
		deltas    []int64
		count     int64
		prevCount int64
	)

	appendDelta := func(count int64) {
		spans[len(spans)-1].Length++
		deltas = append(deltas, count-prevCount)
		prevCount = count
	}

	// Let the compiler figure out that this is const during this function by
	// moving it into a local variable.
	numBuckets := len(bucketCounts)

	bucketIdx := offset>>scaleDown + 1

	initialOffset := offset
	if adjustOffset {
		initialOffset = initialOffset>>scaleDown + 1
	}

	spans = append(spans, histogram.Span{
		Offset: initialOffset,
		Length: 0,
	})

	for i := 0; i < numBuckets; i++ {
		nextBucketIdx := (int32(i)+offset)>>scaleDown + 1
		if bucketIdx == nextBucketIdx { // We have not collected enough buckets to merge yet.
			count += int64(bucketCounts[i])
			continue
		}
		if count == 0 {
			count = int64(bucketCounts[i])
			continue
		}

		gap := nextBucketIdx - bucketIdx - 1
		if gap > 2 {
			// We have to create a new span, because we have found a gap
			// of more than two buckets. The constant 2 is copied from the logic in
			// https://github.com/prometheus/client_golang/blob/27f0506d6ebbb117b6b697d0552ee5be2502c5f2/prometheus/histogram.go#L1296
			spans = append(spans, histogram.Span{
				Offset: gap,
				Length: 0,
			})
		} else {
			// We have found a small gap (or no gap at all).
			// Insert empty buckets as needed.
			for j := int32(0); j < gap; j++ {
				appendDelta(0)
			}
		}
		appendDelta(count)
		count = int64(bucketCounts[i])
		bucketIdx = nextBucketIdx
	}

	// Need to use the last item's index. The offset is scaled and adjusted by 1 as described above.
	gap := (int32(numBuckets)+offset-1)>>scaleDown + 1 - bucketIdx
	if gap > 2 {
		// We have to create a new span, because we have found a gap
		// of more than two buckets. The constant 2 is copied from the logic in
		// https://github.com/prometheus/client_golang/blob/27f0506d6ebbb117b6b697d0552ee5be2502c5f2/prometheus/histogram.go#L1296
		spans = append(spans, histogram.Span{
			Offset: gap,
			Length: 0,
		})
	} else {
		// We have found a small gap (or no gap at all).
		// Insert empty buckets as needed.
		for j := int32(0); j < gap; j++ {
			appendDelta(0)
		}
	}
	appendDelta(count)

	return spans, deltas
}

// addExponentialHistogramDataPoints adds OTel exponential histogram data points to the corresponding time series
// as native histogram samples.
func (c *OTLPToAppender) addExponentialHistogramDataPoints(ctx context.Context, dataPoints pmetric.ExponentialHistogramDataPointSlice,
	resource pcommon.Resource, settings Settings, promName string, temporality pmetric.AggregationTemporality,
	scope scope,
) (annotations.Annotations, error) {
	var annots annotations.Annotations
	var ref storage.SeriesRef
	var prevLabelsBuf, currLabelsBuf []byte
	for x := 0; x < dataPoints.Len(); x++ {
		if err := c.everyN.checkContext(ctx); err != nil {
			return annots, err
		}
		pt := dataPoints.At(x)

		h, ws, err := exponentialToNativeHistogramTSDB(pt, temporality)
		annots.Merge(ws)
		if err != nil {
			return annots, err
		}

		currLabels := c.toLabels(promName, loadAttributes(
			resource,
			pt.Attributes(),
			scope,
			settings,
			nil,
			true,
		))
		currLabelsBuf = currLabels.Bytes(currLabelsBuf)
		if !bytes.Equal(prevLabelsBuf, currLabelsBuf) {
			// If the labels have changed, we need to reset the reference.
			ref = 0
			prevLabelsBuf = currLabelsBuf
		}
		ref, err = c.appendHistogram(ref, currLabels, convertTimeStamp(pt.Timestamp()), h)
		if err != nil {
			return annots, fmt.Errorf("failed to add exponential histogram data point: %w", err)
		}

		exemplars, err := loadExemplars(c, ctx, &c.everyN, pt)
		if err != nil {
			return annots, fmt.Errorf("failed to load exemplars: %w", err)
		}

		err = c.appendExemplars(ctx, &c.everyN, ref, currLabels, exemplars)
		if err != nil {
			return annots, fmt.Errorf("failed to add exemplars: %w", err)
		}
	}

	return annots, nil
}

// exponentialToNativeHistogram translates an OTel Exponential Histogram data point
// to a Prometheus Native Histogram.
func exponentialToNativeHistogramTSDB(p pmetric.ExponentialHistogramDataPoint, temporality pmetric.AggregationTemporality) (*histogram.Histogram, annotations.Annotations, error) {
	var annots annotations.Annotations
	scale := p.Scale()
	if scale < -4 {
		return nil, annots,
			fmt.Errorf("cannot convert exponential to native histogram."+
				" Scale must be >= -4, was %d", scale)
	}

	var scaleDown int32
	if scale > 8 {
		scaleDown = scale - 8
		scale = 8
	}

	pSpans, pDeltas := convertBucketsLayoutTSDB(p.Positive().BucketCounts().AsRaw(), p.Positive().Offset(), scaleDown, true)
	nSpans, nDeltas := convertBucketsLayoutTSDB(p.Negative().BucketCounts().AsRaw(), p.Negative().Offset(), scaleDown, true)

	// The counter reset detection must be compatible with Prometheus to
	// safely set ResetHint to NO. This is not ensured currently.
	// Sending a sample that triggers counter reset but with ResetHint==NO
	// would lead to Prometheus panic as it does not double check the hint.
	// Thus we're explicitly saying UNKNOWN here, which is always safe.
	// TODO: using created time stamp should be accurate, but we
	// need to know here if it was used for the detection.
	// Ref: https://github.com/open-telemetry/opentelemetry-collector-contrib/pull/28663#issuecomment-1810577303
	// Counter reset detection in Prometheus: https://github.com/prometheus/prometheus/blob/f997c72f294c0f18ca13fa06d51889af04135195/tsdb/chunkenc/histogram.go#L232
	resetHint := histogram.UnknownCounterReset

	if temporality == pmetric.AggregationTemporalityDelta {
		// If the histogram has delta temporality, set the reset hint to gauge to avoid unnecessary chunk cutting.
		// We're in an early phase of implementing delta support (proposal: https://github.com/prometheus/proposals/pull/48/).
		// This might be changed to a different hint name as gauge type might be misleading for samples that should be
		// summed over time.
		resetHint = histogram.GaugeType
	}

	h := &histogram.Histogram{
		CounterResetHint: resetHint,
		Schema:           scale,

		ZeroCount:     p.ZeroCount(),
		ZeroThreshold: p.ZeroThreshold(),

		PositiveSpans:   pSpans,
		PositiveBuckets: pDeltas,
		NegativeSpans:   nSpans,
		NegativeBuckets: nDeltas,
	}

	if p.Flags().NoRecordedValue() {
		h.Sum = math.Float64frombits(value.StaleNaN)
	} else {
		if p.HasSum() {
			h.Sum = p.Sum()
		}
		h.Count = p.Count()
		if p.Count() == 0 && h.Sum != 0 {
			annots.Add(fmt.Errorf("exponential histogram data point has zero count, but non-zero sum: %f", h.Sum))
		}
	}

	return h, annots, nil
}

// func getBucketOffset(buckets []uint64) (offset int) {
// 	for offset < len(buckets) && buckets[offset] == 0 {
// 		offset++
// 	}
// 	return offset
// }

// appendSample appends a sample to the appender and returns the series reference.
func (c *OTLPToAppender) appendSample(ref storage.SeriesRef, sLabels labels.Labels, ts int64, v float64) (storage.SeriesRef, error) {
	if sLabels.Len() == 0 {
		// This shouldn't happen
		return 0, nil
	}

	return c.app.Append(ref, sLabels, ts, v)
}

func (c *OTLPToAppender) appendHistogram(ref storage.SeriesRef, sLabels labels.Labels, ts int64, h *histogram.Histogram) (storage.SeriesRef, error) {
	if h == nil || sLabels.Len() == 0 {
		// This shouldn't happen
		return 0, nil
	}

	return c.app.AppendHistogram(ref, sLabels, ts, h, nil)
}

func (c *OTLPToAppender) appendExemplars(ctx context.Context, everyN *everyNTimes, ref storage.SeriesRef, sLabels labels.Labels, exemplars []exemplar.Exemplar) error {
	for _, exemplar := range exemplars {
		if err := everyN.checkContext(ctx); err != nil {
			return err
		}
		var err error
		ref, err = c.app.AppendExemplar(ref, sLabels, exemplar)
		if err != nil {
			return fmt.Errorf("failed to append exemplar: %w", err)
		}
	}

	return nil
}

// toLabels takes a metric name and a map of labels, and returns a labels.Labels object.
func (c *OTLPToAppender) toLabels(metricName string, labels map[string]string) labels.Labels {
	c.builder.Reset()
	for name, value := range labels {
		c.builder.Add(name, value)
	}
	c.builder.Add(model.MetricNameLabel, metricName)
	c.builder.Sort()
	return c.builder.Labels()
}

// loadAttributes loads OTLP attributes into the labels.ScratchBuilder.
// Unpaired string values are ignored. String pairs overwrite OTLP labels if collisions happen and
// if logOnOverwrite is true, the overwrite is logged. Resulting label names are sanitized.
// If settings.PromoteResourceAttributes is not empty, it's a set of resource attributes that should be promoted to labels.
func loadAttributes(resource pcommon.Resource, attributes pcommon.Map, scope scope, settings Settings, ignoreAttrs []string, logOnOverwrite bool) map[string]string {
	resourceAttrs := resource.Attributes()
	serviceName, haveServiceName := resourceAttrs.Get(conventions.AttributeServiceName)
	instance, haveInstanceID := resourceAttrs.Get(conventions.AttributeServiceInstanceID)

	promotedAttrs := settings.PromoteResourceAttributes.promotedAttributes(resourceAttrs)

	promoteScope := settings.PromoteScopeMetadata && scope.name != ""
	scopeLabelCount := 0
	if promoteScope {
		// Include name, version and schema URL.
		scopeLabelCount = scope.attributes.Len() + 3
	}
	// Calculate the maximum possible number of labels we could return so we can preallocate l.
	maxLabelCount := attributes.Len() + len(settings.ExternalLabels) + len(promotedAttrs) + scopeLabelCount

	if haveServiceName {
		maxLabelCount++
	}
	if haveInstanceID {
		maxLabelCount++
	}

	// Ensure attributes are sorted by key for consistent merging of keys which
	// collide when sanitized.
	// labels := make([]prompb.Label, 0, maxLabelCount)

	// XXX: Should we always drop service namespace/service name/service instance ID from the labels
	// (as they get mapped to other Prometheus labels)?
	l := make(map[string]string, maxLabelCount)
	attributes.Range(func(key string, value pcommon.Value) bool {
		if !slices.Contains(ignoreAttrs, key) {
			finalKey := key
			if !settings.AllowUTF8 {
				finalKey = otlptranslator.NormalizeLabel(finalKey)
			}
			if existingValue, alreadyExists := l[finalKey]; alreadyExists {
				l[finalKey] = existingValue + ";" + value.AsString()
			} else {
				l[finalKey] = value.AsString()
			}
		}
		return true
	})

	for _, lbl := range promotedAttrs {
		normalized := lbl.Name
		if !settings.AllowUTF8 {
			normalized = otlptranslator.NormalizeLabel(normalized)
		}
		if _, exists := l[normalized]; !exists {
			l[normalized] = lbl.Value
		}
	}
	if promoteScope {
		l["otel_scope_name"] = scope.name
		l["otel_scope_version"] = scope.version
		l["otel_scope_schema_url"] = scope.schemaURL
		scope.attributes.Range(func(k string, v pcommon.Value) bool {
			name := "otel_scope_" + k
			if !settings.AllowUTF8 {
				name = otlptranslator.NormalizeLabel(name)
			}
			l[name] = v.AsString()
			return true
		})
	}

	// Map service.name + service.namespace to job.
	if haveServiceName {
		val := serviceName.AsString()
		if serviceNamespace, ok := resourceAttrs.Get(conventions.AttributeServiceNamespace); ok {
			val = fmt.Sprintf("%s/%s", serviceNamespace.AsString(), val)
		}
		l[model.JobLabel] = val
	}
	// Map service.instance.id to instance.
	if haveInstanceID {
		l[model.InstanceLabel] = instance.AsString()
	}
	for key, value := range settings.ExternalLabels {
		// External labels have already been sanitized.
		if _, alreadyExists := l[key]; alreadyExists {
			// Skip external labels if they are overridden by metric attributes.
			continue
		}
		l[key] = value
	}

	// for i := 0; i < len(extras); i += 2 {
	// 	if i+1 >= len(extras) {
	// 		break
	// 	}

	// 	name := extras[i]
	// 	_, found := l[name]
	// 	if found && logOnOverwrite {
	// 		log.Println("label " + name + " is overwritten. Check if Prometheus reserved labels are used.")
	// 	}
	// 	// internal labels should be maintained.
	// 	if !settings.AllowUTF8 && (len(name) <= 4 || name[:2] != "__" || name[len(name)-2:] != "__") {
	// 		name = otlptranslator.NormalizeLabel(name)
	// 	}
	// 	l[name] = extras[i+1]
	// }

	return l
}

func loadExemplars[T exemplarType](c *OTLPToAppender, ctx context.Context, everyN *everyNTimes, pt T) ([]exemplar.Exemplar, error) {
	result := make([]exemplar.Exemplar, 0, pt.Exemplars().Len())
	for i := 0; i < pt.Exemplars().Len(); i++ {
		if err := everyN.checkContext(ctx); err != nil {
			return nil, err
		}

		ptExemplar := pt.Exemplars().At(i)
		exemplarRunes := 0

		e := exemplar.Exemplar{
			Ts: timestamp.FromTime(ptExemplar.Timestamp().AsTime()),
		}
		switch ptExemplar.ValueType() {
		case pmetric.ExemplarValueTypeInt:
			e.Value = float64(ptExemplar.IntValue())
		case pmetric.ExemplarValueTypeDouble:
			e.Value = ptExemplar.DoubleValue()
		default:
			return nil, fmt.Errorf("unsupported exemplar value type: %v", ptExemplar.ValueType())
		}

		c.builder.Reset()
		if traceID := ptExemplar.TraceID(); !traceID.IsEmpty() {
			val := hex.EncodeToString(traceID[:])
			exemplarRunes += utf8.RuneCountInString(traceIDKey) + utf8.RuneCountInString(val)
			c.builder.Add(traceIDKey, val)

		}
		if spanID := ptExemplar.SpanID(); !spanID.IsEmpty() {
			val := hex.EncodeToString(spanID[:])
			exemplarRunes += utf8.RuneCountInString(spanIDKey) + utf8.RuneCountInString(val)
			c.builder.Add(spanIDKey, val)
		}

		attrs := ptExemplar.FilteredAttributes()
		labelsFromAttributes := make([]labels.Label, 0, attrs.Len())
		attrs.Range(func(key string, value pcommon.Value) bool {
			val := value.AsString()
			exemplarRunes += utf8.RuneCountInString(key) + utf8.RuneCountInString(val)
			labelsFromAttributes = append(labelsFromAttributes, labels.Label{
				Name:  key,
				Value: val,
			})
			return true
		})
		if exemplarRunes <= maxExemplarRunes {
			// only append filtered attributes if it does not cause exemplar
			// labels to exceed the max number of runes
			for _, label := range labelsFromAttributes {
				c.builder.Add(label.Name, label.Value)
			}
		}
		c.builder.Sort()
		e.Labels = c.builder.Labels()
		result = append(result, e)
	}

	return result, nil
}

// addResourceTargetInfo converts the resource to the target info metric.
func addResourceTargetInfoTSDB(resource pcommon.Resource, settings Settings, timestamp pcommon.Timestamp, converter *OTLPToAppender) {
	if settings.DisableTargetInfo || timestamp == 0 {
		return
	}

	attributes := resource.Attributes()
	identifyingAttrs := []string{
		conventions.AttributeServiceNamespace,
		conventions.AttributeServiceName,
		conventions.AttributeServiceInstanceID,
	}
	nonIdentifyingAttrsCount := attributes.Len()
	for _, a := range identifyingAttrs {
		_, haveAttr := attributes.Get(a)
		if haveAttr {
			nonIdentifyingAttrsCount--
		}
	}
	if nonIdentifyingAttrsCount == 0 {
		// If we only have job + instance, then target_info isn't useful, so don't add it.
		return
	}

	name := targetMetricName
	if len(settings.Namespace) > 0 {
		name = settings.Namespace + "_" + name
	}

	settings.PromoteResourceAttributes = nil
	if settings.KeepIdentifyingResourceAttributes {
		// Do not pass identifying attributes as ignoreAttrs below.
		identifyingAttrs = nil
	}
	labels := loadAttributes(resource, attributes, scope{}, settings, identifyingAttrs, false)
	haveIdentifier := false
	for k := range labels {
		if k == model.JobLabel || k == model.InstanceLabel {
			haveIdentifier = true
			break
		}
	}

	if !haveIdentifier {
		// We need at least one identifying label to generate target_info.
		return
	}

	_, _ = converter.appendSample(0, converter.toLabels(name, labels), convertTimeStamp(timestamp), 1)
}
