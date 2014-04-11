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

	"github.com/prometheus/prometheus/stats"
)

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

type PreloadingMetricPersistence interface {
	MetricPersistence
	NewViewRequestBuilder() ViewRequestBuilder
	MakeView(builder ViewRequestBuilder, deadline time.Duration, queryStats *stats.TimerGroup) (View, error)
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

// ViewRequestBuilder represents the summation of all datastore queries that
// shall be performed to extract values. Call the Get... methods to record the
// queries. Once done, use HasOp and PopOp to retrieve the resulting
// operations. The operations are sorted by their fingerprint (and, for equal
// fingerprints, by the StartsAt timestamp of their operation).
type ViewRequestBuilder interface {
	// GetMetricAtTime records a query to get, for the given Fingerprint,
	// either the value at that time if there is a match or the one or two
	// values adjacent thereto.
	GetMetricAtTime(fingerprint *clientmodel.Fingerprint, time clientmodel.Timestamp)
	// GetMetricAtInterval records a query to get, for the given
	// Fingerprint, either the value at that interval from From through
	// Through if there is a match or the one or two values adjacent for
	// each point.
	GetMetricAtInterval(fingerprint *clientmodel.Fingerprint, from, through clientmodel.Timestamp, interval time.Duration)
	// GetMetricRange records a query to get, for the given Fingerprint, the
	// values that occur inclusively from From through Through.
	GetMetricRange(fingerprint *clientmodel.Fingerprint, from, through clientmodel.Timestamp)
	// GetMetricRangeAtInterval records a query to get value ranges at
	// intervals for the given Fingerprint:
	//
	//   |----|       |----|       |----|       |----|
	//   ^    ^            ^       ^    ^            ^
	//   |    \------------/       \----/            |
	//  from     interval       rangeDuration     through
	GetMetricRangeAtInterval(fp *clientmodel.Fingerprint, from, through clientmodel.Timestamp, interval, rangeDuration time.Duration)
	// PopOp emits the next operation in the queue (sorted by
	// fingerprint). If called while HasOps returns false, the
	// behavior is undefined.
	PopOp() Op
	// HasOp returns true if there is at least one more operation in the
	// queue.
	HasOp() bool
}

// Op encapsulates a primitive query operation.
type Op interface {
	// Fingerprint returns the fingerprint of the metric this operation
	// operates on.
	Fingerprint() *clientmodel.Fingerprint
	// ExtractSamples extracts samples from a stream of values and advances
	// the operation time.
	ExtractSamples(Values) Values
	// Consumed returns whether the operator has consumed all data it needs.
	Consumed() bool
	// CurrentTime gets the current operation time. In a newly created op,
	// this is the starting time of the operation. During ongoing execution
	// of the op, the current time is advanced accordingly. Once no
	// subsequent work associated with the operation remains, nil is
	// returned.
	CurrentTime() clientmodel.Timestamp
}

// CurationState contains high-level curation state information for the
// heads-up-display.
type CurationState struct {
	Active      bool
	Name        string
	Limit       time.Duration
	Fingerprint *clientmodel.Fingerprint
}
