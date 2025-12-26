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

// TODO(krajorama): rename this package to otlpappender or similar, as it is
// not specific to Prometheus remote write anymore.
// Note otlptranslator is already used by prometheus/otlptranslator repo.
package prometheusremotewrite

import (
	"errors"
	"fmt"
	"log/slog"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"github.com/prometheus/common/model"
	"go.opentelemetry.io/collector/pdata/pcommon"
	"go.opentelemetry.io/collector/pdata/xpdata/entity"

	"github.com/prometheus/prometheus/model/exemplar"
	"github.com/prometheus/prometheus/model/histogram"
	"github.com/prometheus/prometheus/model/labels"
	"github.com/prometheus/prometheus/model/metadata"
	"github.com/prometheus/prometheus/storage"
	"github.com/prometheus/prometheus/tsdb/seriesmetadata"
)

// Metadata extends metadata.Metadata with the metric family name.
// OTLP calculates the metric family name for all metrics and uses
// it for generating summary, histogram series by adding the magic
// suffixes. The metric family name is passed down to the appender
// in case the storage needs it for metadata updates.
// Known user is Mimir that implements /api/v1/metadata and uses
// Remote-Write 1.0 for this. Might be removed later if no longer
// needed by any downstream project.
type Metadata struct {
	metadata.Metadata
	MetricFamilyName string
}

// CombinedAppender is similar to storage.Appender, but combines updates to
// metadata, created timestamps, exemplars and samples into a single call.
type CombinedAppender interface {
	// AppendSample appends a sample and related exemplars, metadata, and
	// created timestamp to the storage.
	AppendSample(ls labels.Labels, meta Metadata, st, t int64, v float64, es []exemplar.Exemplar) error
	// AppendHistogram appends a histogram and related exemplars, metadata, and
	// created timestamp to the storage.
	AppendHistogram(ls labels.Labels, meta Metadata, st, t int64, h *histogram.Histogram, es []exemplar.Exemplar) error
	// SetResourceContext sets the current OTel resource context.
	// Entity refs and attributes are extracted from the resource and used
	// to persist entity information for each series appended until
	// the context is changed or cleared.
	SetResourceContext(resource pcommon.Resource)
}

// CombinedAppenderMetrics is for the metrics observed by the
// combinedAppender implementation.
type CombinedAppenderMetrics struct {
	samplesAppendedWithoutMetadata prometheus.Counter
	outOfOrderExemplars            prometheus.Counter
}

func NewCombinedAppenderMetrics(reg prometheus.Registerer) CombinedAppenderMetrics {
	return CombinedAppenderMetrics{
		samplesAppendedWithoutMetadata: promauto.With(reg).NewCounter(prometheus.CounterOpts{
			Namespace: "prometheus",
			Subsystem: "api",
			Name:      "otlp_appended_samples_without_metadata_total",
			Help:      "The total number of samples ingested from OTLP without corresponding metadata.",
		}),
		outOfOrderExemplars: promauto.With(reg).NewCounter(prometheus.CounterOpts{
			Namespace: "prometheus",
			Subsystem: "api",
			Name:      "otlp_out_of_order_exemplars_total",
			Help:      "The total number of received OTLP exemplars which were rejected because they were out of order.",
		}),
	}
}

// NewCombinedAppender creates a combined appender that sets start times and
// updates metadata for each series only once, and appends samples and
// exemplars for each call.
func NewCombinedAppender(app storage.Appender, logger *slog.Logger, ingestSTZeroSample, appendMetadata bool, metrics CombinedAppenderMetrics) CombinedAppender {
	return NewCombinedAppenderWithResourceAttrs(app, logger, ingestSTZeroSample, appendMetadata, false, metrics)
}

// NewCombinedAppenderWithResourceAttrs creates a combined appender with optional
// resource attributes persistence support.
func NewCombinedAppenderWithResourceAttrs(app storage.Appender, logger *slog.Logger, ingestSTZeroSample, appendMetadata, persistResourceAttrs bool, metrics CombinedAppenderMetrics) CombinedAppender {
	return &combinedAppender{
		app:                            app,
		logger:                         logger,
		ingestSTZeroSample:             ingestSTZeroSample,
		appendMetadata:                 appendMetadata,
		persistResourceAttrs:           persistResourceAttrs,
		refs:                           make(map[uint64]seriesRef),
		samplesAppendedWithoutMetadata: metrics.samplesAppendedWithoutMetadata,
		outOfOrderExemplars:            metrics.outOfOrderExemplars,
	}
}

type seriesRef struct {
	ref  storage.SeriesRef
	st   int64
	ls   labels.Labels
	meta metadata.Metadata
}

type combinedAppender struct {
	app                            storage.Appender
	logger                         *slog.Logger
	samplesAppendedWithoutMetadata prometheus.Counter
	outOfOrderExemplars            prometheus.Counter
	ingestSTZeroSample             bool
	appendMetadata                 bool
	persistResourceAttrs           bool
	// Used to ensure we only update metadata and created timestamps once, and to share storage.SeriesRefs.
	// To detect hash collision it also stores the labels.
	// There is no overflow/conflict list, the TSDB will handle that part.
	refs map[uint64]seriesRef
	// resourceAttrs holds the current OTel resource attributes context.
	// Set via SetResourceContext and used to persist attributes for each series.
	resourceAttrs map[string]string
	// entities holds the current OTel entities extracted from entity_refs.
	// If empty, a default entity is created from resource attributes.
	entities []storage.EntityData
}

func (b *combinedAppender) AppendSample(ls labels.Labels, meta Metadata, st, t int64, v float64, es []exemplar.Exemplar) (err error) {
	return b.appendFloatOrHistogram(ls, meta.Metadata, st, t, v, nil, es)
}

func (b *combinedAppender) AppendHistogram(ls labels.Labels, meta Metadata, st, t int64, h *histogram.Histogram, es []exemplar.Exemplar) (err error) {
	if h == nil {
		// Sanity check, we should never get here with a nil histogram.
		b.logger.Error("Received nil histogram in CombinedAppender.AppendHistogram", "series", ls.String())
		return errors.New("internal error, attempted to append nil histogram")
	}
	return b.appendFloatOrHistogram(ls, meta.Metadata, st, t, 0, h, es)
}

func (b *combinedAppender) SetResourceContext(resource pcommon.Resource) {
	// Extract attributes as a map
	b.resourceAttrs = resourceAttrsToMap(resource.Attributes())

	// Extract entities from entity_refs
	b.entities = extractEntities(resource, b.resourceAttrs)
}

// extractEntities extracts entities from OTLP entity_refs.
// Returns nil if no entity_refs are present.
func extractEntities(resource pcommon.Resource, attrs map[string]string) []storage.EntityData {
	entityRefs := entity.ResourceEntityRefs(resource)

	if entityRefs.Len() == 0 {
		// No entity_refs: return nil (no synthetic entities)
		return nil
	}

	entities := make([]storage.EntityData, 0, entityRefs.Len())
	for i := 0; i < entityRefs.Len(); i++ {
		ref := entityRefs.At(i)
		entityType := ref.Type()
		if entityType == "" {
			entityType = seriesmetadata.EntityTypeResource
		}

		// Extract identifying attributes by looking up the id_keys in the attributes map
		idKeys := ref.IdKeys()
		id := make(map[string]string, idKeys.Len())
		for j := 0; j < idKeys.Len(); j++ {
			key := idKeys.At(j)
			if val, ok := attrs[key]; ok {
				id[key] = val
			}
		}

		// Extract descriptive attributes by looking up the description_keys in the attributes map
		descKeys := ref.DescriptionKeys()
		desc := make(map[string]string, descKeys.Len())
		for j := 0; j < descKeys.Len(); j++ {
			key := descKeys.At(j)
			if val, ok := attrs[key]; ok {
				desc[key] = val
			}
		}

		entities = append(entities, storage.EntityData{
			Type:        entityType,
			ID:          id,
			Description: desc,
		})
	}

	return entities
}

func (b *combinedAppender) appendFloatOrHistogram(ls labels.Labels, meta metadata.Metadata, st, t int64, v float64, h *histogram.Histogram, es []exemplar.Exemplar) (err error) {
	hash := ls.Hash()
	series, exists := b.refs[hash]
	ref := series.ref
	if exists && !labels.Equal(series.ls, ls) {
		// Hash collision. The series reference we stored is pointing to a
		// different series so we cannot use it, we need to reset the
		// reference and cache.
		// Note: we don't need to keep track of conflicts here,
		// the TSDB will handle that part when we pass 0 reference.
		exists = false
		ref = 0
	}
	updateRefs := !exists || series.st != st
	if updateRefs && st != 0 && st < t && b.ingestSTZeroSample {
		var newRef storage.SeriesRef
		if h != nil {
			newRef, err = b.app.AppendHistogramSTZeroSample(ref, ls, t, st, h, nil)
		} else {
			newRef, err = b.app.AppendSTZeroSample(ref, ls, t, st)
		}
		if err != nil {
			if !errors.Is(err, storage.ErrOutOfOrderST) && !errors.Is(err, storage.ErrDuplicateSampleForTimestamp) {
				// Even for the first sample OOO is a common scenario because
				// we can't tell if a ST was already ingested in a previous request.
				// We ignore the error.
				// ErrDuplicateSampleForTimestamp is also a common scenario because
				// unknown start times in Opentelemetry are indicated by setting
				// the start time to the same as the first sample time.
				// https://opentelemetry.io/docs/specs/otel/metrics/data-model/#cumulative-streams-handling-unknown-start-time
				b.logger.Warn("Error when appending ST from OTLP", "err", err, "series", ls.String(), "start_timestamp", st, "timestamp", t, "sample_type", sampleType(h))
			}
		} else {
			// We only use the returned reference on success as otherwise an
			// error of ST append could invalidate the series reference.
			ref = newRef
		}
	}
	{
		var newRef storage.SeriesRef
		if h != nil {
			newRef, err = b.app.AppendHistogram(ref, ls, t, h, nil)
		} else {
			newRef, err = b.app.Append(ref, ls, t, v)
		}
		if err != nil {
			// Although Append does not currently return ErrDuplicateSampleForTimestamp there is
			// a note indicating its inclusion in the future.
			if errors.Is(err, storage.ErrOutOfOrderSample) ||
				errors.Is(err, storage.ErrOutOfBounds) ||
				errors.Is(err, storage.ErrDuplicateSampleForTimestamp) {
				b.logger.Error("Error when appending sample from OTLP", "err", err.Error(), "series", ls.String(), "timestamp", t, "sample_type", sampleType(h))
			}
		} else {
			// If the append was successful, we can use the returned reference.
			ref = newRef
		}
	}

	if ref == 0 {
		// We cannot update metadata or add exemplars on non existent series.
		return err
	}

	metadataChanged := exists && (series.meta.Help != meta.Help || series.meta.Type != meta.Type || series.meta.Unit != meta.Unit)

	// Update cache if references changed or metadata changed.
	if updateRefs || metadataChanged {
		b.refs[hash] = seriesRef{
			ref:  ref,
			st:   st,
			ls:   ls,
			meta: meta,
		}
	}

	// Update metadata in storage if enabled and needed.
	if b.appendMetadata && (!exists || metadataChanged) {
		// Only update metadata in WAL if the metadata-wal-records feature is enabled.
		// Without this feature, metadata is not persisted to WAL.
		_, err := b.app.UpdateMetadata(ref, ls, meta)
		if err != nil {
			b.samplesAppendedWithoutMetadata.Add(1)
			b.logger.Warn("Error while updating metadata from OTLP", "err", err)
		}
	}

	// Update resource in storage if enabled and we have attributes.
	// Skip target_info series since it's synthesized from resource attributes and storing
	// resource attributes for it would be redundant.
	if b.persistResourceAttrs && len(b.resourceAttrs) > 0 && !exists && ls.Get(model.MetricNameLabel) != targetMetricName {
		// Only update resource for new series (not seen before in this batch).
		// The timestamp t is used to track when this resource was active.

		// Split resource attributes into identifying and descriptive
		identifying := make(map[string]string)
		descriptive := make(map[string]string, len(b.resourceAttrs))
		for k, v := range b.resourceAttrs {
			if seriesmetadata.IsIdentifyingAttribute(k) {
				identifying[k] = v
			} else {
				descriptive[k] = v
			}
		}

		// Update resource with both attributes and entities in a single call
		_, err := b.app.UpdateResource(ref, ls, identifying, descriptive, b.entities, t)
		if err != nil {
			b.logger.Warn("Error while updating resource from OTLP", "err", err)
		}
	}

	b.appendExemplars(ref, ls, es)

	return err
}

func sampleType(h *histogram.Histogram) string {
	if h == nil {
		return "float"
	}
	return "histogram"
}

func (b *combinedAppender) appendExemplars(ref storage.SeriesRef, ls labels.Labels, es []exemplar.Exemplar) storage.SeriesRef {
	var err error
	for _, e := range es {
		if ref, err = b.app.AppendExemplar(ref, ls, e); err != nil {
			switch {
			case errors.Is(err, storage.ErrOutOfOrderExemplar):
				b.outOfOrderExemplars.Add(1)
				b.logger.Debug("Out of order exemplar from OTLP", "series", ls.String(), "exemplar", fmt.Sprintf("%+v", e))
			default:
				// Since exemplar storage is still experimental, we don't fail the request on ingestion errors
				b.logger.Debug("Error while adding exemplar from OTLP", "series", ls.String(), "exemplar", fmt.Sprintf("%+v", e), "err", err)
			}
		}
	}
	return ref
}
