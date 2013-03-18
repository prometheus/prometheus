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
	"github.com/prometheus/prometheus/model"
	"github.com/prometheus/prometheus/storage"
	"time"
)

// MetricPersistence is a system for storing metric samples in a persistence
// layer.
type MetricPersistence interface {
	// A storage system may rely on external resources and thusly should be
	// closed when finished.
	Close() error

	// Commit all pending operations, if any, since some of the storage components
	// queue work on channels and operate on it in bulk.
	// Flush() error

	// Record a new sample in the storage layer.
	AppendSample(model.Sample) error

	// Record a new sample in the storage layer.
	AppendSamples(model.Samples) error

	// Get all of the metric fingerprints that are associated with the provided
	// label set.
	GetFingerprintsForLabelSet(model.LabelSet) (model.Fingerprints, error)

	// Get all of the metric fingerprints that are associated for a given label
	// name.
	GetFingerprintsForLabelName(model.LabelName) (model.Fingerprints, error)

	GetMetricForFingerprint(model.Fingerprint) (*model.Metric, error)

	GetValueAtTime(model.Fingerprint, time.Time, StalenessPolicy) (*model.Sample, error)
	GetBoundaryValues(model.Fingerprint, model.Interval, StalenessPolicy) (*model.Sample, *model.Sample, error)
	GetRangeValues(model.Fingerprint, model.Interval) (*model.SampleSet, error)

	ForEachSample(IteratorsForFingerprintBuilder) (err error)

	GetAllMetricNames() ([]string, error)

	// Requests the storage stack to build a materialized View of the values
	// contained therein.
	// MakeView(builder ViewRequestBuilder, deadline time.Duration) (View, error)
}

// Describes the lenience limits for querying the materialized View.
type StalenessPolicy struct {
	// Describes the inclusive limit at which individual points if requested will
	// be matched and subject to interpolation.
	DeltaAllowance time.Duration
}

// View provides view of the values in the datastore subject to the request of a
// preloading operation.
type View interface {
	GetValueAtTime(model.Fingerprint, time.Time) []model.SamplePair
	GetBoundaryValues(model.Fingerprint, model.Interval) []model.SamplePair
	GetRangeValues(model.Fingerprint, model.Interval) []model.SamplePair

	// Destroy this view.
	Close()
}

type Series interface {
	Fingerprint() model.Fingerprint
	Metric() model.Metric
}

type IteratorsForFingerprintBuilder interface {
	ForStream(stream stream) (storage.RecordDecoder, storage.RecordFilter, storage.RecordOperator)
}
