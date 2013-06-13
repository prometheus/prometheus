// Copyright 2013 Prometheus Team
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

package metric

import (
	"time"

	clientmodel "github.com/prometheus/client_golang/model"

	"github.com/prometheus/prometheus/model"
	"github.com/prometheus/prometheus/storage"
)

// MetricPersistence is a system for storing metric samples in a persistence
// layer.
type MetricPersistence interface {
	// A storage system may rely on external resources and thusly should be
	// closed when finished.
	Close()

	// Commit all pending operations, if any, since some of the storage components
	// queue work on channels and operate on it in bulk.
	// Flush() error

	// Record a new sample in the storage layer.
	AppendSample(clientmodel.Sample) error

	// Record a group of new samples in the storage layer.
	AppendSamples(clientmodel.Samples) error

	// Get all of the metric fingerprints that are associated with the provided
	// label set.
	GetFingerprintsForLabelSet(model.LabelSet) (model.Fingerprints, error)

	// Get all of the metric fingerprints that are associated for a given label
	// name.
	GetFingerprintsForLabelName(model.LabelName) (model.Fingerprints, error)

	// Get the metric associated with the provided fingerprint.
	GetMetricForFingerprint(*clientmodel.Fingerprint) (clientmodel.Metric, error)

	// Get the two metric values that are immediately adjacent to a given time.
	GetValueAtTime(*clientmodel.Fingerprint, time.Time) model.Values
	// Get the boundary values of an interval: the first value older than the
	// interval start, and the first value younger than the interval end.
	GetBoundaryValues(*clientmodel.Fingerprint, model.Interval) model.Values
	// Get all values contained within a provided interval.
	GetRangeValues(*clientmodel.Fingerprint, model.Interval) model.Values
	// Get all label values that are associated with a given label name.
	GetAllValuesForLabel(model.LabelName) (model.LabelValues, error)

	// Requests the storage stack to build a materialized View of the values
	// contained therein.
	// MakeView(builder ViewRequestBuilder, deadline time.Duration) (View, error)
}

// View provides a view of the values in the datastore subject to the request
// of a preloading operation.
type View interface {
	GetValueAtTime(*clientmodel.Fingerprint, time.Time) model.Values
	GetBoundaryValues(*clientmodel.Fingerprint, model.Interval) model.Values
	GetRangeValues(*clientmodel.Fingerprint, model.Interval) model.Values

	// Destroy this view.
	Close()
}

type Series interface {
	Fingerprint() *clientmodel.Fingerprint
	Metric() clientmodel.Metric
}

type IteratorsForFingerprintBuilder interface {
	ForStream(stream *stream) (storage.RecordDecoder, storage.RecordFilter, storage.RecordOperator)
}
