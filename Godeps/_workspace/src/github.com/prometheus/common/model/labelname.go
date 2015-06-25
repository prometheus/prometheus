// Copyright 2013 The Prometheus Authors
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

package model

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
)

const (
	// ExportedLabelPrefix is the prefix to prepend to the label names present in
	// exported metrics if a label of the same name is added by the server.
	ExportedLabelPrefix LabelName = "exported_"

	// MetricNameLabel is the label name indicating the metric name of a
	// timeseries.
	MetricNameLabel LabelName = "__name__"

	// AddressLabel is the name of the label that holds the address of
	// a scrape target.
	AddressLabel LabelName = "__address__"

	// MetricsPathLabel is the name of the label that holds the path on which to
	// scrape a target.
	MetricsPathLabel LabelName = "__metrics_path__"

	// ReservedLabelPrefix is a prefix which is not legal in user-supplied
	// label names.
	ReservedLabelPrefix = "__"

	// MetaLabelPrefix is a prefix for labels that provide meta information.
	// Labels with this prefix are used for intermediate label processing and
	// will not be attached to time series.
	MetaLabelPrefix = "__meta_"

	// JobLabel is the label name indicating the job from which a timeseries
	// was scraped.
	JobLabel LabelName = "job"

	// InstanceLabel is the label name used for the instance label.
	InstanceLabel LabelName = "instance"

	// BucketLabel is used for the label that defines the upper bound of a
	// bucket of a histogram ("le" -> "less or equal").
	BucketLabel = "le"

	// QuantileLabel is used for the label that defines the quantile in a
	// summary.
	QuantileLabel = "quantile"
)

// LabelNameRE is a regular expression matching valid label names.
var LabelNameRE = regexp.MustCompile("^[a-zA-Z_][a-zA-Z0-9_]*$")

// A LabelName is a key for a LabelSet or Metric.  It has a value associated
// therewith.
type LabelName string

// UnmarshalYAML implements the yaml.Unmarshaler interface.
func (ln *LabelName) UnmarshalYAML(unmarshal func(interface{}) error) error {
	var s string
	if err := unmarshal(&s); err != nil {
		return err
	}
	if !LabelNameRE.MatchString(s) {
		return fmt.Errorf("%q is not a valid label name", s)
	}
	*ln = LabelName(s)
	return nil
}

// UnmarshalJSON implements the json.Unmarshaler interface.
func (ln *LabelName) UnmarshalJSON(b []byte) error {
	var s string
	if err := json.Unmarshal(b, &s); err != nil {
		return err
	}
	if !LabelNameRE.MatchString(s) {
		return fmt.Errorf("%q is not a valid label name", s)
	}
	*ln = LabelName(s)
	return nil
}

// LabelNames is a sortable LabelName slice. In implements sort.Interface.
type LabelNames []LabelName

func (l LabelNames) Len() int {
	return len(l)
}

func (l LabelNames) Less(i, j int) bool {
	return l[i] < l[j]
}

func (l LabelNames) Swap(i, j int) {
	l[i], l[j] = l[j], l[i]
}

func (l LabelNames) String() string {
	labelStrings := make([]string, 0, len(l))
	for _, label := range l {
		labelStrings = append(labelStrings, string(label))
	}
	return strings.Join(labelStrings, ", ")
}
