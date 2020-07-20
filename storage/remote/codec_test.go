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

	"github.com/prometheus/prometheus/pkg/labels"
	"github.com/prometheus/prometheus/prompb"
	"github.com/prometheus/prometheus/storage"
	"github.com/prometheus/prometheus/util/testutil"
)

func TestValidateLabelsAndMetricName(t *testing.T) {
	tests := []struct {
		input       labels.Labels
		expectedErr string
		description string
	}{
		{
			input: labels.FromStrings(
				"__name__", "name",
				"labelName", "labelValue",
			),
			expectedErr: "",
			description: "regular labels",
		},
		{
			input: labels.FromStrings(
				"__name__", "name",
				"_labelName", "labelValue",
			),
			expectedErr: "",
			description: "label name with _",
		},
		{
			input: labels.FromStrings(
				"__name__", "name",
				"@labelName", "labelValue",
			),
			expectedErr: "invalid label name: @labelName",
			description: "label name with @",
		},
		{
			input: labels.FromStrings(
				"__name__", "name",
				"123labelName", "labelValue",
			),
			expectedErr: "invalid label name: 123labelName",
			description: "label name starts with numbers",
		},
		{
			input: labels.FromStrings(
				"__name__", "name",
				"", "labelValue",
			),
			expectedErr: "invalid label name: ",
			description: "label name is empty string",
		},
		{
			input: labels.FromStrings(
				"__name__", "name",
				"labelName", string([]byte{0xff}),
			),
			expectedErr: "invalid label value: " + string([]byte{0xff}),
			description: "label value is an invalid UTF-8 value",
		},
		{
			input: labels.FromStrings(
				"__name__", "@invalid_name",
			),
			expectedErr: "invalid metric name: @invalid_name",
			description: "metric name starts with @",
		},
		{
			input: labels.FromStrings(
				"__name__", "name1",
				"__name__", "name2",
			),
			expectedErr: "duplicate label with name: __name__",
			description: "duplicate label names",
		},
		{
			input: labels.FromStrings(
				"label1", "name",
				"label2", "name",
			),
			expectedErr: "",
			description: "duplicate label values",
		},
		{
			input: labels.FromStrings(
				"", "name",
				"label2", "name",
			),
			expectedErr: "invalid label name: ",
			description: "don't report as duplicate label name",
		},
	}

	for _, test := range tests {
		t.Run(test.description, func(t *testing.T) {
			err := validateLabelsAndMetricName(test.input)
			if test.expectedErr != "" {
				testutil.NotOk(t, err)
				testutil.Equals(t, test.expectedErr, err.Error())
			} else {
				testutil.Ok(t, err)
			}
		})
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
	testutil.Assert(t, c.Next(), "Expected Next() to be true.")
	testutil.Equals(t, series1, c.At(), "Unexpected series returned.")
	testutil.Assert(t, c.Next(), "Expected Next() to be true.")
	testutil.Equals(t, series2, c.At(), "Unexpected series returned.")
	testutil.Assert(t, !c.Next(), "Expected Next() to be false.")
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
	testutil.Equals(t, lbls, gotLabels)

	gotLabels[0].Value = "foo"
	gotLabels[1].Value = "bar"

	gotLabels = cs.Labels()
	testutil.Equals(t, lbls, gotLabels)
}

func TestFromQueryResultWithDuplicates(t *testing.T) {
	ts1 := prompb.TimeSeries{
		Labels: []prompb.Label{
			{Name: "foo", Value: "bar"},
			{Name: "foo", Value: "def"},
		},
		Samples: []prompb.Sample{
			{Value: 0.0, Timestamp: 0},
		},
	}

	res := prompb.QueryResult{
		Timeseries: []*prompb.TimeSeries{
			&ts1,
		},
	}

	series := FromQueryResult(false, &res)

	errSeries, isErrSeriesSet := series.(errSeriesSet)

	testutil.Assert(t, isErrSeriesSet, "Expected resulting series to be an errSeriesSet")
	errMessage := errSeries.Err().Error()
	testutil.Assert(t, errMessage == "duplicate label with name: foo", fmt.Sprintf("Expected error to be from duplicate label, but got: %s", errMessage))
}

func TestNegotiateResponseType(t *testing.T) {
	r, err := NegotiateResponseType([]prompb.ReadRequest_ResponseType{
		prompb.ReadRequest_STREAMED_XOR_CHUNKS,
		prompb.ReadRequest_SAMPLES,
	})
	testutil.Ok(t, err)
	testutil.Equals(t, prompb.ReadRequest_STREAMED_XOR_CHUNKS, r)

	r2, err := NegotiateResponseType([]prompb.ReadRequest_ResponseType{
		prompb.ReadRequest_SAMPLES,
		prompb.ReadRequest_STREAMED_XOR_CHUNKS,
	})
	testutil.Ok(t, err)
	testutil.Equals(t, prompb.ReadRequest_SAMPLES, r2)

	r3, err := NegotiateResponseType([]prompb.ReadRequest_ResponseType{})
	testutil.Ok(t, err)
	testutil.Equals(t, prompb.ReadRequest_SAMPLES, r3)

	_, err = NegotiateResponseType([]prompb.ReadRequest_ResponseType{20})
	testutil.NotOk(t, err, "expected error due to not supported requested response types")
	testutil.Equals(t, "server does not support any of the requested response types: [20]; supported: map[SAMPLES:{} STREAMED_XOR_CHUNKS:{}]", err.Error())
}

func TestMergeLabels(t *testing.T) {
	for _, tc := range []struct {
		primary, secondary, expected []prompb.Label
	}{
		{
			primary:   []prompb.Label{{Name: "aaa", Value: "foo"}, {Name: "bbb", Value: "foo"}, {Name: "ddd", Value: "foo"}},
			secondary: []prompb.Label{{Name: "bbb", Value: "bar"}, {Name: "ccc", Value: "bar"}},
			expected:  []prompb.Label{{Name: "aaa", Value: "foo"}, {Name: "bbb", Value: "foo"}, {Name: "ccc", Value: "bar"}, {Name: "ddd", Value: "foo"}},
		},
		{
			primary:   []prompb.Label{{Name: "bbb", Value: "bar"}, {Name: "ccc", Value: "bar"}},
			secondary: []prompb.Label{{Name: "aaa", Value: "foo"}, {Name: "bbb", Value: "foo"}, {Name: "ddd", Value: "foo"}},
			expected:  []prompb.Label{{Name: "aaa", Value: "foo"}, {Name: "bbb", Value: "bar"}, {Name: "ccc", Value: "bar"}, {Name: "ddd", Value: "foo"}},
		},
	} {
		testutil.Equals(t, tc.expected, MergeLabels(tc.primary, tc.secondary))
	}
}
