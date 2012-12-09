// Copyright 2012 Prometheus Team
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
	"github.com/matttproud/prometheus/model"
)

// MetricPersistence is a system for storing metric samples in a persistence
// layer.
type MetricPersistence interface {
	// A storage system may rely on external resources and thusly should be
	// closed when finished.
	Close() error

	// Record a new sample in the storage layer.
	AppendSample(sample *model.Sample) error

	// Get all of the metric fingerprints that are associated with the provided
	// label set.
	GetFingerprintsForLabelSet(labelSet *model.LabelSet) ([]*model.Fingerprint, error)

	// Get all of the metric fingerprints that are associated for a given label
	// name.
	GetFingerprintsForLabelName(labelName *model.LabelName) ([]*model.Fingerprint, error)

	GetMetricForFingerprint(f *model.Fingerprint) (*model.Metric, error)

	GetAllLabelNames() ([]string, error)
	GetAllLabelPairs() ([]model.LabelSet, error)
	GetAllMetrics() ([]model.LabelSet, error)

	// // BEGIN QUERY PRIMITIVES
	//
	// GetMetricForFingerprint()
	// GetMetricWatermarks(metrics ...) (watermarks ...)
	// GetMetricValuesForIntervals(metric, interval) (values ...)
	// GetMetricValueLast()
	// // END QUERY PRIMITIVES

}
