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
	"errors"
	"fmt"
	"io"
	"math"
	"slices"
	"time"

	"github.com/prometheus/common/model"

	"github.com/prometheus/prometheus/model/exemplar"
	"github.com/prometheus/prometheus/model/histogram"
	"github.com/prometheus/prometheus/model/labels"
	"github.com/prometheus/prometheus/model/textparse"
	"github.com/prometheus/prometheus/model/timestamp"
	"github.com/prometheus/prometheus/model/value"
	"github.com/prometheus/prometheus/storage"
)

// appenderWithLimits returns an appender with additional validation.
func appenderV2WithLimits(app storage.AppenderV2, sampleLimit, bucketLimit int, maxSchema int32) storage.AppenderV2 {
	app = &timeLimitAppenderV2{
		AppenderV2: app,
		maxTime:    timestamp.FromTime(time.Now().Add(maxAheadTime)),
	}

	// The sampleLimit is applied after metrics are potentially dropped via relabeling.
	if sampleLimit > 0 {
		app = &limitAppenderV2{
			AppenderV2: app,
			limit:      sampleLimit,
		}
	}

	if bucketLimit > 0 {
		app = &bucketLimitAppenderV2{
			AppenderV2: app,
			limit:      bucketLimit,
		}
	}

	if maxSchema < histogram.ExponentialSchemaMax {
		app = &maxSchemaAppenderV2{
			AppenderV2: app,
			maxSchema:  maxSchema,
		}
	}

	return app
}

func (sl *scrapeLoop) updateStaleMarkersV2(app storage.AppenderV2, defTime int64) (err error) {
	sl.cache.forEachStale(func(ref storage.SeriesRef, lset labels.Labels) bool {
		// Series no longer exposed, mark it stale.
		_, err = app.Append(ref, lset, 0, defTime, math.Float64frombits(value.StaleNaN), nil, nil, storage.AOptions{RejectOutOfOrder: true})
		switch {
		case errors.Is(err, storage.ErrOutOfOrderSample), errors.Is(err, storage.ErrDuplicateSampleForTimestamp):
			// Do not count these in logging, as this is expected if a target
			// goes away and comes back again with a new scrape loop.
			err = nil
		}
		return err == nil
	})
	return err
}

type scrapeLoopAppenderV2 struct {
	*scrapeLoop

	storage.AppenderV2
}

var _ scrapeLoopAppendAdapter = &scrapeLoopAppenderV2{}

func (sl *scrapeLoopAppenderV2) append(b []byte, contentType string, ts time.Time) (total, added, seriesAdded int, err error) {
	defTime := timestamp.FromTime(ts)

	if len(b) == 0 {
		// Empty scrape. Just update the stale makers and swap the cache (but don't flush it).
		err = sl.updateStaleMarkersV2(sl.AppenderV2, defTime)
		sl.cache.iterDone(false)
		return total, added, seriesAdded, err
	}

	p, err := textparse.New(b, contentType, sl.symbolTable, textparse.ParserOptions{
		EnableTypeAndUnitLabels:                 sl.enableTypeAndUnitLabels,
		IgnoreNativeHistograms:                  !sl.enableNativeHistogramScraping,
		ConvertClassicHistogramsToNHCB:          sl.convertClassicHistToNHCB,
		KeepClassicOnClassicAndNativeHistograms: sl.alwaysScrapeClassicHist,
		OpenMetricsSkipSTSeries:                 sl.enableSTZeroIngestion,
		FallbackContentType:                     sl.fallbackScrapeProtocol,
	})
	if p == nil {
		sl.l.Error(
			"Failed to determine correct type of scrape target.",
			"content_type", contentType,
			"fallback_media_type", sl.fallbackScrapeProtocol,
			"err", err,
		)
		return total, added, seriesAdded, err
	}
	if err != nil {
		sl.l.Debug(
			"Invalid content type on scrape, using fallback setting.",
			"content_type", contentType,
			"fallback_media_type", sl.fallbackScrapeProtocol,
			"err", err,
		)
	}
	var (
		appErrs        = appendErrors{}
		sampleLimitErr error
		bucketLimitErr error
		lset           labels.Labels     // Escapes to heap so hoisted out of loop.
		e              exemplar.Exemplar // Escapes to heap so hoisted out of loop.
		lastMeta       *metaEntry
		lastMFName     []byte
	)

	exemplars := make([]exemplar.Exemplar, 0, 1)

	// Take an appender with limits.
	app := appenderV2WithLimits(sl.AppenderV2, sl.sampleLimit, sl.bucketLimit, sl.maxSchema)

	defer func() {
		if err != nil {
			return
		}
		// Flush and swap the cache as the scrape was non-empty.
		sl.cache.iterDone(true)
	}()

loop:
	for {
		var (
			et                       textparse.Entry
			sampleAdded, isHistogram bool
			met                      []byte
			parsedTimestamp          *int64
			val                      float64
			h                        *histogram.Histogram
			fh                       *histogram.FloatHistogram
		)
		if et, err = p.Next(); err != nil {
			if errors.Is(err, io.EOF) {
				err = nil
			}
			break
		}
		switch et {
		// TODO(bwplotka): Consider changing parser to give metadata at once instead of type, help and unit in separation, ideally on `Series()/Histogram()
		// otherwise we can expose metadata without series on metadata API.
		case textparse.EntryType:
			// TODO(bwplotka): Build meta entry directly instead of locking and updating the map. This will
			// allow to properly update metadata when e.g unit was added, then removed;
			lastMFName, lastMeta = sl.cache.setType(p.Type())
			continue
		case textparse.EntryHelp:
			lastMFName, lastMeta = sl.cache.setHelp(p.Help())
			continue
		case textparse.EntryUnit:
			lastMFName, lastMeta = sl.cache.setUnit(p.Unit())
			continue
		case textparse.EntryComment:
			continue
		case textparse.EntryHistogram:
			isHistogram = true
		default:
		}
		total++

		t := defTime
		if isHistogram {
			met, parsedTimestamp, h, fh = p.Histogram()
		} else {
			met, parsedTimestamp, val = p.Series()
		}
		if !sl.honorTimestamps {
			parsedTimestamp = nil
		}
		if parsedTimestamp != nil {
			t = *parsedTimestamp
		}

		if sl.cache.getDropped(met) {
			continue
		}
		ce, seriesCached, seriesAlreadyScraped := sl.cache.get(met)
		var (
			ref  storage.SeriesRef
			hash uint64
		)

		if seriesCached {
			ref = ce.ref
			lset = ce.lset
			hash = ce.hash
		} else {
			p.Labels(&lset)
			hash = lset.Hash()

			// Hash label set as it is seen local to the target. Then add target labels
			// and relabeling and store the final label set.
			lset = sl.sampleMutator(lset)

			// The label set may be set to empty to indicate dropping.
			if lset.IsEmpty() {
				sl.cache.addDropped(met)
				continue
			}

			if !lset.Has(model.MetricNameLabel) {
				err = errNameLabelMandatory
				break loop
			}
			if !lset.IsValid(sl.validationScheme) {
				err = fmt.Errorf("invalid metric name or label names: %s", lset.String())
				break loop
			}

			// If any label limits is exceeded the scrape should fail.
			if err = verifyLabelLimits(lset, sl.labelLimits); err != nil {
				sl.metrics.targetScrapePoolExceededLabelLimits.Inc()
				break loop
			}
		}

		exemplars = exemplars[:0] // Reset and reuse the exemplar slice.

		if seriesAlreadyScraped && parsedTimestamp == nil {
			err = storage.ErrDuplicateSampleForTimestamp
		} else {
			// Double check we don't append float 0 for
			// histogram case where parser returns bad data.
			// This can only happen when parser has a bug.
			if isHistogram && h == nil && fh == nil {
				err = fmt.Errorf("parser returned nil histogram/float histogram for a histogram entry type for %v series; parser bug; aborting", lset.String())
				break loop
			}

			st := int64(0)
			if sl.enableSTZeroIngestion {
				// p.StartTimestamp() tend to be expensive (e.g. OM1). Do it only if we care.
				st = p.StartTimestamp()
			}

			for hasExemplar := p.Exemplar(&e); hasExemplar; hasExemplar = p.Exemplar(&e) {
				if !e.HasTs {
					if isHistogram {
						// We drop exemplars for native histograms if they don't have a timestamp.
						// Missing timestamps are deliberately not supported as we want to start
						// enforcing timestamps for exemplars as otherwise proper deduplication
						// is inefficient and purely based on heuristics: we cannot distinguish
						// between repeated exemplars and new instances with the same values.
						// This is done silently without logs as it is not an error but out of spec.
						// This does not affect classic histograms so that behaviour is unchanged.
						e = exemplar.Exemplar{} // Reset for the next fetch.
						continue
					}
					e.Ts = t
				}
				exemplars = append(exemplars, e)
				e = exemplar.Exemplar{} // Reset for the next fetch.
			}

			// Prepare append call.
			appOpts := storage.AOptions{}
			if len(exemplars) > 0 {
				// Sort so that checking for duplicates / out of order is more efficient during validation.
				slices.SortFunc(exemplars, exemplar.Compare)
				appOpts.Exemplars = exemplars
			}

			// Metadata path mimicks the scrape appender V1 flow. Once we remove v2
			// flow we should rename "appendMetadataToWAL" flag to "passMetadata" because for v2 flow
			// the metadata storage detail is behind the appendableV2 contract. V2 also means we always pass the metadata,
			// we don't check if it changed (that code can be removed).
			//
			// Long term, we should always attach the metadata without any flag. Unfortunately because of the limitation
			// of the TEXT and OpenMetrics 1.0 (hopefully fixed in OpenMetrics 2.0) there are edge cases around unknown
			// metadata + suffixes that is expensive (isSeriesPartOfFamily) or in some cases impossible to detect. For this
			// reason metadata (appendMetadataToWAL=true) appender V2 flow scrape might taking ~3% more CPU in our benchmarks.
			//
			// TODO(https://github.com/prometheus/prometheus/issues/17900): Optimize this, notably move this check to parsers that require this (ensuring parser
			// interface always yields correct metadata), deliver OpenMetrics 2.0 that removes suffixes.
			if sl.appendMetadataToWAL && lastMeta != nil {
				// In majority cases we can trust that the current series/histogram is matching the lastMeta and lastMFName.
				// However, optional TYPE, etc metadata and broken OM text can break this, detect those cases here.
				if !isSeriesPartOfFamily(lset.Get(model.MetricNameLabel), lastMFName, lastMeta.Type) {
					lastMeta = nil // Don't pass knowingly broken metadata, now, nor on the next line.
				}
				if lastMeta != nil {
					// Metric family name has the same source as metadata.
					appOpts.MetricFamilyName = yoloString(lastMFName)
					appOpts.Metadata = lastMeta.Metadata
				}
			}

			// Append sample to the storage.
			ref, err = app.Append(ref, lset, st, t, val, h, fh, appOpts)
		}
		sampleAdded, err = sl.checkAddError(met, exemplars, err, &sampleLimitErr, &bucketLimitErr, &appErrs)
		if err != nil {
			if !errors.Is(err, storage.ErrNotFound) {
				sl.l.Debug("Unexpected error", "series", string(met), "err", err)
			}
			break loop
		}
		if (parsedTimestamp == nil || sl.trackTimestampsStaleness) && ce != nil {
			sl.cache.trackStaleness(ce.ref, ce)
		}

		// If series wasn't cached (is new, not seen on previous scrape) we need to add it to the scrape cache.
		// But we only do this for series that were appended to TSDB without errors.
		// If a series was new, but we didn't append it due to sample_limit or other errors then we don't need
		// it in the scrape cache because we don't need to emit StaleNaNs for it when it disappears.
		if !seriesCached && sampleAdded {
			ce = sl.cache.addRef(met, ref, lset, hash)
			if ce != nil && (parsedTimestamp == nil || sl.trackTimestampsStaleness) {
				// Bypass staleness logic if there is an explicit timestamp.
				// But make sure we only do this if we have a cache entry (ce) for our series.
				sl.cache.trackStaleness(ref, ce)
			}
			if sampleLimitErr == nil && bucketLimitErr == nil {
				seriesAdded++
			}
		}

		// Increment added even if there's an error so we correctly report the
		// number of samples remaining after relabeling.
		// We still report duplicated samples here since this number should be the exact number
		// of time series exposed on a scrape after relabelling.
		added++
	}
	if sampleLimitErr != nil {
		if err == nil {
			err = sampleLimitErr
		}
		// We only want to increment this once per scrape, so this is Inc'd outside the loop.
		sl.metrics.targetScrapeSampleLimit.Inc()
	}
	if bucketLimitErr != nil {
		if err == nil {
			err = bucketLimitErr // If sample limit is hit, that error takes precedence.
		}
		// We only want to increment this once per scrape, so this is Inc'd outside the loop.
		sl.metrics.targetScrapeNativeHistogramBucketLimit.Inc()
	}
	if appErrs.numOutOfOrder > 0 {
		sl.l.Warn("Error on ingesting out-of-order samples", "num_dropped", appErrs.numOutOfOrder)
	}
	if appErrs.numDuplicates > 0 {
		sl.l.Warn("Error on ingesting samples with different value but same timestamp", "num_dropped", appErrs.numDuplicates)
	}
	if appErrs.numOutOfBounds > 0 {
		sl.l.Warn("Error on ingesting samples that are too old or are too far into the future", "num_dropped", appErrs.numOutOfBounds)
	}
	if appErrs.numExemplarOutOfOrder > 0 {
		sl.l.Warn("Error on ingesting out-of-order exemplars", "num_dropped", appErrs.numExemplarOutOfOrder)
	}
	if err == nil {
		err = sl.updateStaleMarkersV2(app, defTime)
	}
	return total, added, seriesAdded, err
}

func (sl *scrapeLoopAppenderV2) addReportSample(s reportSample, t int64, v float64, b *labels.Builder, rejectOOO bool) (err error) {
	ce, ok, _ := sl.cache.get(s.name)
	var ref storage.SeriesRef
	var lset labels.Labels
	if ok {
		ref = ce.ref
		lset = ce.lset
	} else {
		// The constants are suffixed with the invalid \xff unicode rune to avoid collisions
		// with scraped metrics in the cache.
		// We have to drop it when building the actual metric.
		b.Reset(labels.EmptyLabels())
		b.Set(model.MetricNameLabel, string(s.name[:len(s.name)-1]))
		lset = sl.reportSampleMutator(b.Labels())
	}

	ref, err = sl.Append(ref, lset, 0, t, v, nil, nil, storage.AOptions{
		MetricFamilyName: yoloString(s.name),
		Metadata:         s.Metadata,
		RejectOutOfOrder: rejectOOO,
	})
	switch {
	case err == nil:
		if !ok {
			sl.cache.addRef(s.name, ref, lset, lset.Hash())
		}
		return nil
	case errors.Is(err, storage.ErrOutOfOrderSample), errors.Is(err, storage.ErrDuplicateSampleForTimestamp):
		// Do not log here, as this is expected if a target goes away and comes back
		// again with a new scrape loop.
		return nil
	default:
		return err
	}
}
