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

// This file contains Appender v1 flow for the temporary compatibility with downstream
// scrape.NewManager users (e.g. OpenTelemetry).
//
// No new changes should be added here. Prometheus do NOT use this code.
// TODO(bwplotka): Remove once Otel has migrated (add issue).

type scrapeLoopAppenderV1 struct {
	*scrapeLoop

	storage.Appender
}

var _ scrapeLoopAppender = &scrapeLoopAppenderV1{}

func (sl *scrapeLoop) updateStaleMarkersV1(app storage.Appender, defTime int64) (err error) {
	sl.cache.forEachStale(func(ref storage.SeriesRef, lset labels.Labels) bool {
		// Series no longer exposed, mark it stale.
		app.SetOptions(&storage.AppendOptions{DiscardOutOfOrder: true})
		_, err = app.Append(ref, lset, defTime, math.Float64frombits(value.StaleNaN))
		app.SetOptions(nil)
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

// appenderV1 returns an appender for ingested samples from the target.
func appenderV1(app storage.Appender, sampleLimit, bucketLimit int, maxSchema int32) storage.Appender {
	app = &timeLimitAppenderV1{
		Appender: app,
		maxTime:  timestamp.FromTime(time.Now().Add(maxAheadTime)),
	}

	// The sampleLimit is applied after metrics are potentially dropped via relabeling.
	if sampleLimit > 0 {
		app = &limitAppenderV1{
			Appender: app,
			limit:    sampleLimit,
		}
	}

	if bucketLimit > 0 {
		app = &bucketLimitAppenderV1{
			Appender: app,
			limit:    bucketLimit,
		}
	}

	if maxSchema < histogram.ExponentialSchemaMax {
		app = &maxSchemaAppenderV1{
			Appender:  app,
			maxSchema: maxSchema,
		}
	}

	return app
}

func (sl *scrapeLoopAppenderV1) addReportSample(s reportSample, t int64, v float64, b *labels.Builder, rejectOOO bool) error {
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

	opt := storage.AppendOptions{DiscardOutOfOrder: rejectOOO}
	sl.SetOptions(&opt)
	ref, err := sl.Append(ref, lset, t, v)
	opt.DiscardOutOfOrder = false
	sl.SetOptions(&opt)
	switch {
	case err == nil:
		if !ok {
			sl.cache.addRef(s.name, ref, lset, lset.Hash())
			// We only need to add metadata once a scrape target appears.
			if sl.appendMetadataToWAL {
				if _, merr := sl.UpdateMetadata(ref, lset, s.Metadata); merr != nil {
					sl.l.Debug("Error when appending metadata in addReportSample", "ref", fmt.Sprintf("%d", ref), "metadata", fmt.Sprintf("%+v", s.Metadata), "err", merr)
				}
			}
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

// append for the deprecated storage.Appender flow.
// This is only for downstream project migration purposes and will be removed soon.
func (sl *scrapeLoopAppenderV1) append(b []byte, contentType string, ts time.Time) (total, added, seriesAdded int, err error) {
	defTime := timestamp.FromTime(ts)

	if len(b) == 0 {
		// Empty scrape. Just update the stale makers and swap the cache (but don't flush it).
		err = sl.updateStaleMarkersV1(sl.Appender, defTime)
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
	app := appenderV1(sl.Appender, sl.sampleLimit, sl.bucketLimit, sl.maxSchema)

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

			if !lset.Has(labels.MetricName) {
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

		if seriesAlreadyScraped && parsedTimestamp == nil {
			err = storage.ErrDuplicateSampleForTimestamp
		} else {
			if sl.enableSTZeroIngestion {
				if stMs := p.StartTimestamp(); stMs != 0 {
					if isHistogram {
						if h != nil {
							ref, err = app.AppendHistogramSTZeroSample(ref, lset, t, stMs, h, nil)
						} else {
							ref, err = app.AppendHistogramSTZeroSample(ref, lset, t, stMs, nil, fh)
						}
					} else {
						ref, err = app.AppendSTZeroSample(ref, lset, t, stMs)
					}
					if err != nil && !errors.Is(err, storage.ErrOutOfOrderST) { // OOO is a common case, ignoring completely for now.
						// ST is an experimental feature. For now, we don't need to fail the
						// scrape on errors updating the created timestamp, log debug.
						sl.l.Debug("Error when appending ST in scrape loop", "series", string(met), "ct", stMs, "t", t, "err", err)
					}
				}
			}

			if isHistogram {
				if h != nil {
					ref, err = app.AppendHistogram(ref, lset, t, h, nil)
				} else {
					ref, err = app.AppendHistogram(ref, lset, t, nil, fh)
				}
			} else {
				ref, err = app.Append(ref, lset, t, val)
			}
		}

		if err == nil {
			if (parsedTimestamp == nil || sl.trackTimestampsStaleness) && ce != nil {
				sl.cache.trackStaleness(ce.ref, ce)
			}
		}

		// NOTE: exemplars are nil here for v1 appender flow. We append and check the exemplar errors later on.
		sampleAdded, err = sl.checkAddError(met, nil, err, &sampleLimitErr, &bucketLimitErr, &appErrs)
		if err != nil {
			if !errors.Is(err, storage.ErrNotFound) {
				sl.l.Debug("Unexpected error", "series", string(met), "err", err)
			}
			break loop
		}

		// If series wasn't cached (is new, not seen on previous scrape) we need need to add it to the scrape cache.
		// But we only do this for series that were appended to TSDB without errors.
		// If a series was new but we didn't append it due to sample_limit or other errors then we don't need
		// it in the scrape cache because we don't need to emit StaleNaNs for it when it disappears.
		if !seriesCached && sampleAdded {
			ce = sl.cache.addRef(met, ref, lset, hash)
			if ce != nil && (parsedTimestamp == nil || sl.trackTimestampsStaleness) {
				// Bypass staleness logic if there is an explicit timestamp.
				// But make sure we only do this if we have a cache entry (ce) for our series.
				sl.cache.trackStaleness(ref, ce)
			}
			if sampleAdded && sampleLimitErr == nil && bucketLimitErr == nil {
				seriesAdded++
			}
		}

		// Increment added even if there's an error so we correctly report the
		// number of samples remaining after relabeling.
		// We still report duplicated samples here since this number should be the exact number
		// of time series exposed on a scrape after relabelling.
		added++
		exemplars = exemplars[:0] // Reset and reuse the exemplar slice.
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
					e = exemplar.Exemplar{} // Reset for next time round loop.
					continue
				}
				e.Ts = t
			}
			exemplars = append(exemplars, e)
			e = exemplar.Exemplar{} // Reset for next time round loop.
		}
		// Sort so that checking for duplicates / out of order is more efficient during validation.
		slices.SortFunc(exemplars, exemplar.Compare)
		outOfOrderExemplars := 0
		for _, e := range exemplars {
			_, exemplarErr := app.AppendExemplar(ref, lset, e)
			switch {
			case exemplarErr == nil:
				// Do nothing.
			case errors.Is(exemplarErr, storage.ErrOutOfOrderExemplar):
				outOfOrderExemplars++
			default:
				// Since exemplar storage is still experimental, we don't fail the scrape on ingestion errors.
				sl.l.Debug("Error while adding exemplar in AddExemplar", "exemplar", fmt.Sprintf("%+v", e), "err", exemplarErr)
			}
		}
		if outOfOrderExemplars > 0 && outOfOrderExemplars == len(exemplars) {
			// Only report out of order exemplars if all are out of order, otherwise this was a partial update
			// to some existing set of exemplars.
			appErrs.numExemplarOutOfOrder += outOfOrderExemplars
			sl.l.Debug("Out of order exemplars", "count", outOfOrderExemplars, "latest", fmt.Sprintf("%+v", exemplars[len(exemplars)-1]))
			sl.metrics.targetScrapeExemplarOutOfOrder.Add(float64(outOfOrderExemplars))
		}

		if sl.appendMetadataToWAL && lastMeta != nil {
			// Is it new series OR did metadata change for this family?
			if !seriesCached || lastMeta.lastIterChange == sl.cache.iter {
				// In majority cases we can trust that the current series/histogram is matching the lastMeta and lastMFName.
				// However, optional TYPE etc metadata and broken OM text can break this, detect those cases here.
				// TODO(bwplotka): Consider moving this to parser as many parser users end up doing this (e.g. ST and NHCB parsing).
				if isSeriesPartOfFamily(lset.Get(model.MetricNameLabel), lastMFName, lastMeta.Type) {
					if _, merr := app.UpdateMetadata(ref, lset, lastMeta.Metadata); merr != nil {
						// No need to fail the scrape on errors appending metadata.
						sl.l.Debug("Error when appending metadata in scrape loop", "ref", fmt.Sprintf("%d", ref), "metadata", fmt.Sprintf("%+v", lastMeta.Metadata), "err", merr)
					}
				}
			}
		}
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
		err = sl.updateStaleMarkersV1(app, defTime)
	}
	return total, added, seriesAdded, err
}

// limitAppenderV1 limits the number of total appended samples in a batch.
type limitAppenderV1 struct {
	storage.Appender

	limit int
	i     int
}

func (app *limitAppenderV1) Append(ref storage.SeriesRef, lset labels.Labels, t int64, v float64) (storage.SeriesRef, error) {
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

func (app *limitAppenderV1) AppendHistogram(ref storage.SeriesRef, lset labels.Labels, t int64, h *histogram.Histogram, fh *histogram.FloatHistogram) (storage.SeriesRef, error) {
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

type timeLimitAppenderV1 struct {
	storage.Appender

	maxTime int64
}

func (app *timeLimitAppenderV1) Append(ref storage.SeriesRef, lset labels.Labels, t int64, v float64) (storage.SeriesRef, error) {
	if t > app.maxTime {
		return 0, storage.ErrOutOfBounds
	}

	ref, err := app.Appender.Append(ref, lset, t, v)
	if err != nil {
		return 0, err
	}
	return ref, nil
}

// bucketLimitAppenderV1 limits the number of total appended samples in a batch.
type bucketLimitAppenderV1 struct {
	storage.Appender

	limit int
}

func (app *bucketLimitAppenderV1) AppendHistogram(ref storage.SeriesRef, lset labels.Labels, t int64, h *histogram.Histogram, fh *histogram.FloatHistogram) (storage.SeriesRef, error) {
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

type maxSchemaAppenderV1 struct {
	storage.Appender

	maxSchema int32
}

func (app *maxSchemaAppenderV1) AppendHistogram(ref storage.SeriesRef, lset labels.Labels, t int64, h *histogram.Histogram, fh *histogram.FloatHistogram) (storage.SeriesRef, error) {
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
