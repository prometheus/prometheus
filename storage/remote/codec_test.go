// Copyright 2017 The Prometheus Authors
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

package remote

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/prometheus/prometheus/pkg/labels"
	"github.com/prometheus/prometheus/prompb"
	"github.com/prometheus/prometheus/storage"
	"github.com/prometheus/prometheus/util/testutil"
)

func TestValidateLabelsAndMetricName(t *testing.T) {
	tests := []struct {
		input       labels.Labels
		expectedErr string
		shouldPass  bool
	}{
		{
			input: labels.FromStrings(
				"__name__", "name",
				"labelName", "labelValue",
			),
			expectedErr: "",
			shouldPass:  true,
		},
		{
			input: labels.FromStrings(
				"__name__", "name",
				"_labelName", "labelValue",
			),
			expectedErr: "",
			shouldPass:  true,
		},
		{
			input: labels.FromStrings(
				"__name__", "name",
				"@labelName", "labelValue",
			),
			expectedErr: "Invalid label name: @labelName",
			shouldPass:  false,
		},
		{
			input: labels.FromStrings(
				"__name__", "name",
				"123labelName", "labelValue",
			),
			expectedErr: "Invalid label name: 123labelName",
			shouldPass:  false,
		},
		{
			input: labels.FromStrings(
				"__name__", "name",
				"", "labelValue",
			),
			expectedErr: "Invalid label name: ",
			shouldPass:  false,
		},
		{
			input: labels.FromStrings(
				"__name__", "name",
				"labelName", string([]byte{0xff}),
			),
			expectedErr: "Invalid label value: " + string([]byte{0xff}),
			shouldPass:  false,
		},
		{
			input: labels.FromStrings(
				"__name__", "@invalid_name",
			),
			expectedErr: "Invalid metric name: @invalid_name",
			shouldPass:  false,
		},
	}

	for _, test := range tests {
		err := validateLabelsAndMetricName(test.input)
		if test.shouldPass != (err == nil) {
			if test.shouldPass {
				t.Fatalf("Test should pass, got unexpected error: %v", err)
			} else {
				t.Fatalf("Test should fail, unexpected error, got: %v, expected: %v", err, test.expectedErr)
			}
		}
	}
}

func TestConcreteSeriesSet(t *testing.T) {
	series1 := &concreteSeries{
		labels:  labels.FromStrings("foo", "bar"),
		samples: []prompb.Sample{{Value: 1, Timestamp: 2}},
	}
	series2 := &concreteSeries{
		labels:  labels.FromStrings("foo", "baz"),
		samples: []prompb.Sample{{Value: 3, Timestamp: 4}},
	}
	c := &concreteSeriesSet{
		series: []storage.Series{series1, series2},
	}
	if !c.Next() {
		t.Fatalf("Expected Next() to be true.")
	}
	if c.At() != series1 {
		t.Fatalf("Unexpected series returned.")
	}
	if !c.Next() {
		t.Fatalf("Expected Next() to be true.")
	}
	if c.At() != series2 {
		t.Fatalf("Unexpected series returned.")
	}
	if c.Next() {
		t.Fatalf("Expected Next() to be false.")
	}
}

func TestConcreteSeriesClonesLabels(t *testing.T) {
	lbls := labels.Labels{
		labels.Label{Name: "a", Value: "b"},
		labels.Label{Name: "c", Value: "d"},
	}
	cs := concreteSeries{
		labels: labels.New(lbls...),
	}

	gotLabels := cs.Labels()
	require.Equal(t, lbls, gotLabels)

	gotLabels[0].Value = "foo"
	gotLabels[1].Value = "bar"

	gotLabels = cs.Labels()
	require.Equal(t, lbls, gotLabels)
}

func TestDuplicateLabelsSimple(t *testing.T) {
	labels := []prompb.Label{
		prompb.Label{Name: "foo", Value: "bar"},
		prompb.Label{Name: "foo", Value: "notbar"},
	}
	uniqueLabels := uniqueLabelNameProtos(labels)

	testutil.Assert(t, len(uniqueLabels) == 1, fmt.Sprintf("Expected len(uniqueLabels) to be 1 but got %d instead", len(uniqueLabels)))
	testutil.Assert(t, uniqueLabels[0].Name == "foo", "uniqueLabels has wrong label name")
}

func TestDuplicateLabelsDoNothing(t *testing.T) {
	label1 := prompb.Label{Name: "foo", Value: "bar"}
	label2 := prompb.Label{Name: "abc", Value: "def"}
	labels := []prompb.Label{
		label1,
		label2,
	}
	uniqueLabels := uniqueLabelNameProtos(labels)

	testutil.Assert(t, len(uniqueLabels) == 2, fmt.Sprintf("Expected len(uniqueLabels) to be 2 but got %d instead", len(uniqueLabels)))

	numMatched := 0
	for i := range labels {
		if (labels[i].Name == label1.Name && labels[i].Value == label1.Value) || (labels[i].Name == label2.Name && labels[i].Value == label2.Value) {
			numMatched++
		}
	}

	testutil.Assert(t, numMatched == 2, "Expected labels to not be changed when there is no duplicate")
}

func TestFromQueryResultWithDuplicates(t *testing.T) {
	ts1 := prompb.TimeSeries{
		Labels: []prompb.Label{
			prompb.Label{Name: "foo", Value: "bar"},
			prompb.Label{Name: "foo", Value: "def"},
		},
		Samples: []prompb.Sample{
			prompb.Sample{Value: 0.0, Timestamp: 0},
		},
	}

	res := prompb.QueryResult{
		Timeseries: []*prompb.TimeSeries{
			&ts1,
		},
	}

	series := FromQueryResult(&res)

	testutil.Assert(t, series.Next(), "Expected there to be at least 1 series, but got none instead")
	testutil.Assert(t, len(series.At().Labels()) == 1, "Expected 1 label in series, but got %d instead")
	testutil.Assert(t, series.At().Labels()[0].Name == "foo", "Expected label value and name to not change")
}
