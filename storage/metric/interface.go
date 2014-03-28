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

import clientmodel "github.com/prometheus/client_golang/model"

// AppendBatch models a batch of samples to be stored.
type AppendBatch map[clientmodel.Fingerprint]SampleSet

// MetricPersistence is a system for storing metric samples in a persistence
// layer.
type MetricPersistence interface {
	// A storage system may rely on external resources and thusly should be
	// closed when finished.
	Close()

	// Record a group of new samples in the storage layer.
	AppendSamples(clientmodel.Samples) error

	// Get all of the metric fingerprints that are associated with the
	// provided label matchers.
	GetFingerprintsForLabelMatchers(LabelMatchers) (clientmodel.Fingerprints, error)

	// Get all of the label values that are associated with a given label name.
	GetLabelValuesForLabelName(clientmodel.LabelName) (clientmodel.LabelValues, error)

	// Get the metric associated with the provided fingerprint.
	GetMetricForFingerprint(*clientmodel.Fingerprint) (clientmodel.Metric, error)

	// Get all label values that are associated with a given label name.
	GetAllValuesForLabel(clientmodel.LabelName) (clientmodel.LabelValues, error)
}

// View provides a view of the values in the datastore subject to the request
// of a preloading operation.
type View interface {
	// Get the two values that are immediately adjacent to a given time.
	GetValueAtTime(*clientmodel.Fingerprint, clientmodel.Timestamp) Values
	// Get the boundary values of an interval: the first value older than
	// the interval start, and the first value younger than the interval
	// end.
	GetBoundaryValues(*clientmodel.Fingerprint, Interval) Values
	// Get all values contained within a provided interval.
	GetRangeValues(*clientmodel.Fingerprint, Interval) Values
}

// ViewableMetricPersistence is a MetricPersistence that is able to present the
// samples it has stored as a View.
type ViewableMetricPersistence interface {
	MetricPersistence
	View
}
