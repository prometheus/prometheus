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
	"bytes"
	"errors"
	"fmt"
	"io"
	"math"
	"slices"
	"strings"
	"time"
	"unique"

	"github.com/prometheus/common/model"

	"github.com/prometheus/prometheus/model/exemplar"
	"github.com/prometheus/prometheus/model/histogram"
	"github.com/prometheus/prometheus/model/labels"
	"github.com/prometheus/prometheus/model/metadata"
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
		OpenMetricsSkipSTSeries:                 sl.parseST,
		EnableOpenMetrics2:                      sl.enableOpenMetrics2,
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
		appErrs          = appendErrors{}
		sampleLimitErr   error
		bucketLimitErr   error
		lset             labels.Labels     // Escapes to heap so hoisted out of loop.
		e                exemplar.Exemplar // Escapes to heap so hoisted out of loop.
		lastMeta         *metaEntry
		lastMetaMD       metadata.Metadata
		lastMetaFlushed  bool
		lastMFName       []byte
		parsedMeta       metadata.Metadata
		parsedMFName     []byte
		parsedHelp       []byte
		parsedUnit       []byte
		hasParsedMeta    bool
		parsedMetaDirty  bool
		origMetaHandle   unique.Handle[metadata.Metadata]
		origMFNameHandle unique.Handle[string]
		lastSetMFName    unique.Handle[string]
		lastSetMeta      unique.Handle[metadata.Metadata]
		hasMRC           = len(sl.mrc) > 0
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
			st                       int64
			met                      []byte
			parsedTimestamp          *int64
			val                      float64
			h                        *histogram.Histogram
			fh                       *histogram.FloatHistogram
			skipAppend               bool
			stCache                  *stCache
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
			if !hasMRC {
				lastMFName, lastMeta = sl.cache.setType(p.Type())
				lastMetaFlushed = false
				continue
			}
			mfName, typ := p.Type()
			setParsedType(mfName, typ, &parsedMFName, &parsedMeta, &hasParsedMeta)
			parsedMetaDirty = true
			continue
		case textparse.EntryHelp:
			if !hasMRC {
				lastMFName, lastMeta = sl.cache.setHelp(p.Help())
				lastMetaFlushed = false
				continue
			}
			mfName, help := p.Help()
			setParsedHelp(mfName, help, &parsedMFName, &parsedHelp, &parsedMeta, &hasParsedMeta)
			parsedMetaDirty = true
			continue
		case textparse.EntryUnit:
			if !hasMRC {
				lastMFName, lastMeta = sl.cache.setUnit(p.Unit())
				lastMetaFlushed = false
				continue
			}
			mfName, unit := p.Unit()
			setParsedUnit(mfName, unit, &parsedMFName, &parsedUnit, &parsedMeta, &hasParsedMeta)
			parsedMetaDirty = true
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

		var (
			ref                storage.SeriesRef
			sampleMeta         metadata.Metadata
			sampleMFName       string
			sampleMetaHandle   unique.Handle[metadata.Metadata]
			sampleMFNameHandle unique.Handle[string]
		)

		metHash, ce, seriesCached, seriesAlreadyScraped := sl.cache.get(met)

		if !hasMRC {
			if !seriesCached && sl.cache.getDropped(metHash) {
				continue
			}
			if seriesCached {
				ref = ce.ref
				lset = ce.lset
			} else {
				p.Labels(&lset)

				// Add target labels and apply relabeling before storing the final label set.
				lset = sl.sampleMutator(lset)

				// The label set may be set to empty to indicate dropping.
				if lset.IsEmpty() {
					sl.cache.addDropped(metHash)
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
		} else {
			var (
				sm            seriesMetaEntry
				hasCachedMeta bool
			)
			if seriesCached {
				if sl.cache.seriesMeta != nil {
					sm, hasCachedMeta = sl.cache.seriesMeta[metHash]
				}
			} else if sl.cache.droppedSeriesMeta != nil {
				sm, hasCachedMeta = sl.cache.droppedSeriesMeta[metHash]
			}
			// Intern the current family's parsed metadata and name once for all series in the family.
			// On warm scrapes where the series already has cached handles matching parsedMeta and parsedMFName,
			// reuse those handles directly to avoid calling unique.Make.
			if parsedMetaDirty {
				if hasCachedMeta && stringValue(sm.origMFName) == yoloString(parsedMFName) && metadataValue(sm.origMeta).Equals(parsedMeta) {
					origMFNameHandle = sm.origMFName
					origMetaHandle = sm.origMeta
				} else {
					origMetaHandle = unique.Make(parsedMeta)
					origMFNameHandle = internBytes(parsedMFName)
				}
				parsedMetaDirty = false
			}
			if !seriesCached && sl.cache.getDroppedWithMeta(metHash, origMFNameHandle, origMetaHandle) {
				continue
			}

			if seriesCached && hasCachedMeta && sm.origMFName == origMFNameHandle && sm.origMeta == origMetaHandle {
				ref = ce.ref
				lset = ce.lset
				sampleMetaHandle = sm.meta
				sampleMFNameHandle = sm.mfName
				sampleMeta = metadataValue(sampleMetaHandle)
				sampleMFName = stringValue(sampleMFNameHandle)
				if sampleMFName != "" && (sampleMFNameHandle != lastSetMFName || sampleMetaHandle != lastSetMeta) {
					sl.cache.setMetadata(sampleMFNameHandle, sampleMetaHandle)
					lastSetMFName = sampleMFNameHandle
					lastSetMeta = sampleMetaHandle
				}
			} else {
				p.Labels(&lset)

				// Add target labels and apply relabeling before storing the final label set and metadata.
				lset, sampleMetaHandle, sampleMFNameHandle = sl.mutateSample(lset, origMetaHandle, origMFNameHandle)
				sampleMeta = metadataValue(sampleMetaHandle)
				sampleMFName = stringValue(sampleMFNameHandle)

				// The label set may be set to empty to indicate dropping.
				if lset.IsEmpty() {
					if seriesCached {
						sl.cache.deleteSeries(metHash)
					}
					sl.cache.addDroppedWithMeta(metHash, origMFNameHandle, origMetaHandle)
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

				if sampleMFName != "" && (sampleMFNameHandle != lastSetMFName || sampleMetaHandle != lastSetMeta) {
					sl.cache.setMetadata(sampleMFNameHandle, sampleMetaHandle)
					lastSetMFName = sampleMFNameHandle
					lastSetMeta = sampleMetaHandle
				}
				if seriesCached {
					if ce.lset.Hash() == lset.Hash() && labels.Equal(ce.lset, lset) {
						ref = ce.ref
						sl.cache.setSeriesMeta(metHash, origMetaHandle, sampleMetaHandle, origMFNameHandle, sampleMFNameHandle)
					} else {
						sl.cache.deleteSeries(metHash)
						ce = cacheEntry{}
						seriesCached = false
					}
				}
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

			if sl.parseST {
				// p.StartTimestamp() tend to be expensive (e.g. OM1). Do it only if we care.
				st = p.StartTimestamp()
			}

			// Prepare append call.
			appOpts := storage.AOptions{}

			// TODO(ridwanmsharif): Consider injecting a 0 value with the st == t here, instead of skipping an append.
			if sl.synthesizeST && st == 0 {
				if !hasMRC {
					var stType model.MetricType
					if lastMeta != nil {
						if !lastMetaFlushed {
							lastMetaMD = sl.cache.metadataOf(lastMeta)
							lastMetaFlushed = true
						}
						stType = lastMetaMD.Type
					}
					st, val, h, fh, skipAppend, stCache = sl.checkAndSynthesizeStartTime(st, lset, ce.st, yoloString(lastMFName), stType, val, h, fh, t)
				} else {
					st, val, h, fh, skipAppend, stCache = sl.checkAndSynthesizeStartTime(st, lset, ce.st, sampleMFName, sampleMeta.Type, val, h, fh, t)
				}
				appOpts.RejectOutOfOrder = true
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

			if len(exemplars) > 0 {
				// Sort so that checking for duplicates / out of order is more efficient during validation.
				slices.SortFunc(exemplars, exemplar.Compare)
				appOpts.Exemplars = exemplars
			}

			// Metadata path mimics the scrape appender V1 flow. Once we remove v2
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
			if sl.appendMetadataToWAL {
				if !hasMRC {
					if lastMeta != nil {
						if !lastMetaFlushed {
							lastMetaMD = sl.cache.metadataOf(lastMeta)
							lastMetaFlushed = true
						}
						// In majority cases we can trust that the current series/histogram is matching the lastMeta and lastMFName.
						// However, optional TYPE, etc metadata and broken OM text can break this, detect those cases here.
						if !isSeriesPartOfFamily(lset.Get(model.MetricNameLabel), lastMFName, lastMetaMD.Type) {
							lastMeta = nil // Don't pass knowingly broken metadata, now, nor on the next line.
						} else {
							// Metric family name has the same source as metadata.
							appOpts.MetricFamilyName = yoloString(lastMFName)
							appOpts.Metadata = lastMetaMD
						}
					}
				} else if sampleMFName != "" {
					appOpts.MetricFamilyName = sampleMFName
					appOpts.Metadata = sampleMeta
				}
			}

			// Note that even if append is skipped, we may still add to the cache to make sure we save stCache.
			if !skipAppend {
				// Append sample to the storage.
				ref, err = app.Append(ref, lset, st, t, val, h, fh, appOpts)
				if err == nil && seriesCached && ref != 0 {
					sl.cache.updateRef(metHash, &ce, ref)
				}
			}
		}
		sampleAdded, err = sl.checkAddError(met, exemplars, err, &sampleLimitErr, &bucketLimitErr, &appErrs)
		if err != nil {
			if !errors.Is(err, storage.ErrNotFound) {
				sl.l.Debug("Unexpected error", "series", string(met), "err", err)
			}
			break loop
		}

		// If series wasn't cached (is new, not seen on previous scrape) we need to add it to the scrape cache.
		// But we only do this for series that were appended to TSDB without errors.
		// If a series was new, but we didn't append it due to sample_limit or other errors then we don't need
		// it in the scrape cache because we don't need to emit StaleNaNs for it when it disappears.
		hasCE := seriesCached
		if !seriesCached {
			if sampleAdded {
				ce = sl.cache.addRef(metHash, ref, lset, stCache)
				hasCE = true
				if hasMRC {
					sl.cache.setSeriesMeta(metHash, origMetaHandle, sampleMetaHandle, origMFNameHandle, sampleMFNameHandle)
				}

				if sampleLimitErr == nil && bucketLimitErr == nil {
					seriesAdded++
				}
			}
		} else if sl.synthesizeST {
			if sampleAdded {
				if ce.st != stCache {
					ce.st = stCache // Set it, even if it's nil (explicit reset).
					sl.cache.series[metHash] = ce
				}
			} else if ce.st != nil {
				ce.st = nil // Clear state on failure for existing series.
				sl.cache.series[metHash] = ce
			}
		}

		// We track staleness for a series to ensure that if it disappears in a future scrape,
		// we can emit a StaleNaN marker to terminate the series (which also helps in eventually cleaning up stCache).
		// We only track it if there are no errors and we have a valid storage reference (ce.ref != 0).
		// We track it if:
		// - There is no explicit timestamp in the scrape (parsedTimestamp == nil).
		// - Or we explicitly track staleness for timestamps (sl.trackTimestampsStaleness).
		// - Or we are synthesizing start times (stCache != nil), so we can clear stCache if it goes stale.
		shouldTrackStaleness := parsedTimestamp == nil || sl.trackTimestampsStaleness || stCache != nil
		if hasCE && ce.ref != 0 && shouldTrackStaleness && sampleAdded {
			sl.cache.trackStaleness(ce.ref, ce.lset)
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
	metHash, ce, ok, _ := sl.cache.get(s.name)
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
			sl.cache.addRef(metHash, ref, lset, nil)
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

func (*scrapeLoop) checkAndSynthesizeStartTime(
	st int64,
	lset labels.Labels,
	prevST *stCache,
	sampleMFName string,
	sampleType model.MetricType,
	val float64,
	h *histogram.Histogram,
	fh *histogram.FloatHistogram,
	t int64,
) (int64, float64, *histogram.Histogram, *histogram.FloatHistogram, bool, *stCache) {
	var skipAppend bool
	c := prevST

	// TODO(https://github.com/prometheus/prometheus/issues/1790): Move isSeriesPartOfFamily inside parsers.
	if c == nil {
		var mName string
		switch sampleType {
		case model.MetricTypeCounter, model.MetricTypeHistogram:
			mName = lset.Get(model.MetricNameLabel)
		case model.MetricTypeSummary:
			mName = lset.Get(model.MetricNameLabel)
			if !strings.HasSuffix(mName, "_count") && !strings.HasSuffix(mName, "_sum") {
				return st, val, h, fh, skipAppend, c
			}
		default:
			return st, val, h, fh, skipAppend, c
		}
		if !isSeriesPartOfFamilyStr(mName, sampleMFName, sampleType) {
			return st, val, h, fh, skipAppend, c
		}
		c = &stCache{}
	}

	switch {
	case fh != nil:
		fh, st, skipAppend = c.synthesizeFloatHistogram(fh, t)
	case h != nil:
		h, st, skipAppend = c.synthesizeHistogram(h, t)
	default:
		val, st, skipAppend = c.synthesizeFloat(val, t)
	}

	return st, val, h, fh, skipAppend, c
}

// metricFamilyName computes the post-relabel metric family name from the pre-relabel
// metric name, pre-relabel metric family name, post-relabel metric name, and post-relabel type.
func metricFamilyName(preName, preMFStr, postName string, postType model.MetricType) string {
	if preName == preMFStr {
		return postName
	}
	if preMFStr != "" && preName == postName {
		return preMFStr
	}
	if preMFStr != "" && strings.HasPrefix(preName, preMFStr) {
		suffix := preName[len(preMFStr):]
		if suffix != "" {
			if cut, ok := strings.CutSuffix(postName, suffix); ok {
				return cut
			}
		}
	}
	switch postType {
	case model.MetricTypeHistogram:
		for _, s := range []string{"_bucket", "_sum", "_count"} {
			if cut, ok := strings.CutSuffix(postName, s); ok {
				return cut
			}
		}
	case model.MetricTypeGaugeHistogram:
		for _, s := range []string{"_bucket", "_gsum", "_gcount"} {
			if cut, ok := strings.CutSuffix(postName, s); ok {
				return cut
			}
		}
	case model.MetricTypeSummary:
		for _, s := range []string{"_sum", "_count"} {
			if cut, ok := strings.CutSuffix(postName, s); ok {
				return cut
			}
		}
	}
	return postName
}

func setParsedType(mfName []byte, t model.MetricType, parsedMFName *[]byte, parsedMeta *metadata.Metadata, hasParsedMeta *bool) {
	if !*hasParsedMeta || !bytes.Equal(*parsedMFName, mfName) {
		*parsedMFName = append((*parsedMFName)[:0], mfName...)
		*parsedMeta = metadata.Metadata{Type: model.MetricTypeUnknown}
		*hasParsedMeta = true
	}
	parsedMeta.Type = t
}

func setParsedHelp(mfName, help []byte, parsedMFName, parsedHelp *[]byte, parsedMeta *metadata.Metadata, hasParsedMeta *bool) {
	if !*hasParsedMeta || !bytes.Equal(*parsedMFName, mfName) {
		*parsedMFName = append((*parsedMFName)[:0], mfName...)
		*parsedMeta = metadata.Metadata{Type: model.MetricTypeUnknown}
		*hasParsedMeta = true
	}
	*parsedHelp = append((*parsedHelp)[:0], help...)
	parsedMeta.Help = yoloString(*parsedHelp)
}

func setParsedUnit(mfName, unit []byte, parsedMFName, parsedUnit *[]byte, parsedMeta *metadata.Metadata, hasParsedMeta *bool) {
	if !*hasParsedMeta || !bytes.Equal(*parsedMFName, mfName) {
		*parsedMFName = append((*parsedMFName)[:0], mfName...)
		*parsedMeta = metadata.Metadata{Type: model.MetricTypeUnknown}
		*hasParsedMeta = true
	}
	*parsedUnit = append((*parsedUnit)[:0], unit...)
	parsedMeta.Unit = yoloString(*parsedUnit)
}

// hasTypeSuffixForFamily reports whether mName equals mfStr followed by a type-specific
// series suffix (_total, _bucket, _sum, _count, _gsum, _gcount, or _info).
func hasTypeSuffixForFamily(mName, mfStr string) bool {
	if mfStr == "" || !strings.HasPrefix(mName, mfStr) {
		return false
	}
	switch mName[len(mfStr):] {
	case "_total", "_bucket", "_sum", "_count", "_gsum", "_gcount", "_info":
		return true
	default:
		return false
	}
}

func (sl *scrapeLoop) mutateSample(
	lset labels.Labels,
	origMeta unique.Handle[metadata.Metadata],
	origMFName unique.Handle[string],
) (labels.Labels, unique.Handle[metadata.Metadata], unique.Handle[string]) {
	preName := lset.Get(model.MetricNameLabel)
	origMD := metadataValue(origMeta)
	origMFStr := stringValue(origMFName)
	hasFamilyMeta := !metadata.IsZeroHandle(origMeta) && isSeriesPartOfFamilyStr(preName, origMFStr, origMD.Type)
	var sampleMeta metadata.Metadata
	if hasFamilyMeta {
		sampleMeta = origMD
	} else {
		sampleMeta = metadata.Metadata{Type: model.MetricTypeUnknown}
	}

	if sl.sampleMutatorWithMetadata != nil {
		// A series like foo_total does not match a pre-relabel family named foo when the family
		// has unknown type (since _total suffix stripping only applies to counters). Run relabeling
		// with origMD's help and unit first so rules can read, transform, or clear them if
		// relabeling sets a type that makes the series belong to origMFStr; otherwise fall back to
		// empty metadata so unrelated series do not inherit origMD.
		if !hasFamilyMeta && !metadata.IsZeroHandle(origMeta) && origMD.Type == model.MetricTypeUnknown && hasTypeSuffixForFamily(preName, origMFStr) {
			candLset, candMeta := sl.sampleMutatorWithMetadata(lset, origMD)
			switch {
			case !candLset.IsEmpty() && isSeriesPartOfFamilyStr(preName, origMFStr, candMeta.Type):
				hasFamilyMeta = true
				lset, sampleMeta = candLset, candMeta
			case origMD.Help == "" && origMD.Unit == "":
				lset, sampleMeta = candLset, candMeta
			default:
				lset, sampleMeta = sl.sampleMutatorWithMetadata(lset, metadata.Metadata{Type: model.MetricTypeUnknown})
				if candLset.IsEmpty() && !lset.IsEmpty() && isSeriesPartOfFamilyStr(preName, origMFStr, sampleMeta.Type) {
					lset, sampleMeta = candLset, candMeta
				}
			}
		} else {
			lset, sampleMeta = sl.sampleMutatorWithMetadata(lset, sampleMeta)
		}
	} else if sl.sampleMutator != nil {
		lset = sl.sampleMutator(lset)
	}
	if lset.IsEmpty() || (!hasFamilyMeta && sampleMeta.IsEmpty()) {
		return lset, unique.Handle[metadata.Metadata]{}, unique.Handle[string]{}
	}

	var preMFStr string
	if hasFamilyMeta {
		preMFStr = origMFStr
	}
	postMFName := metricFamilyName(preName, preMFStr, lset.Get(model.MetricNameLabel), sampleMeta.Type)
	var sampleMetaHandle unique.Handle[metadata.Metadata]
	if hasFamilyMeta && sampleMeta.Equals(origMD) {
		sampleMetaHandle = origMeta
	} else {
		sampleMetaHandle = unique.Make(sampleMeta)
	}
	if postMFName == "" {
		return lset, sampleMetaHandle, unique.Handle[string]{}
	}
	if postMFName == origMFStr {
		return lset, sampleMetaHandle, origMFName
	}
	return lset, sampleMetaHandle, unique.Make(postMFName)
}
