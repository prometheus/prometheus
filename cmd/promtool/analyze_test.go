// Copyright 2023 The Prometheus Authors
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

package main

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/prometheus/common/model"
)

var (
	exampleMatrix = model.Matrix{
		&model.SampleStream{
			Metric: model.Metric{
				"le": "+Inf",
			},
			Values: []model.SamplePair{
				{
					Value:     31,
					Timestamp: 100,
				},
				{
					Value:     32,
					Timestamp: 200,
				},
				{
					Value:     40,
					Timestamp: 300,
				},
			},
		},
		&model.SampleStream{
			Metric: model.Metric{
				"le": "0.5",
			},
			Values: []model.SamplePair{
				{
					Value:     10,
					Timestamp: 100,
				},
				{
					Value:     11,
					Timestamp: 200,
				},
				{
					Value:     11,
					Timestamp: 300,
				},
			},
		},
		&model.SampleStream{
			Metric: model.Metric{
				"le": "10",
			},
			Values: []model.SamplePair{
				{
					Value:     30,
					Timestamp: 100,
				},
				{
					Value:     31,
					Timestamp: 200,
				},
				{
					Value:     37,
					Timestamp: 300,
				},
			},
		},
		&model.SampleStream{
			Metric: model.Metric{
				"le": "2",
			},
			Values: []model.SamplePair{
				{
					Value:     25,
					Timestamp: 100,
				},
				{
					Value:     26,
					Timestamp: 200,
				},
				{
					Value:     27,
					Timestamp: 300,
				},
			},
		},
	}
	exampleMatrixLength = len(exampleMatrix)
)

func init() {
	sortMatrix(exampleMatrix)
}

func TestGetBucketCountsAtTime(t *testing.T) {
	cases := []struct {
		matrix   model.Matrix
		length   int
		timeIdx  int
		expected []int
	}{
		{
			exampleMatrix,
			exampleMatrixLength,
			0,
			[]int{10, 15, 5, 1},
		},
		{
			exampleMatrix,
			exampleMatrixLength,
			1,
			[]int{11, 15, 5, 1},
		},
		{
			exampleMatrix,
			exampleMatrixLength,
			2,
			[]int{11, 16, 10, 3},
		},
	}

	for _, c := range cases {
		t.Run(fmt.Sprintf("exampleMatrix@%d", c.timeIdx), func(t *testing.T) {
			res, err := getBucketCountsAtTime(c.matrix, c.length, c.timeIdx)
			require.NoError(t, err)
			require.Equal(t, c.expected, res)
		})
	}
}

func TestCalcClassicBucketStatistics(t *testing.T) {
	cases := []struct {
		matrix   model.Matrix
		expected *statistics
	}{
		{
			exampleMatrix,
			&statistics{
				minPop: 4,
				avgPop: 4,
				maxPop: 4,
				total:  4,
			},
		},
	}

	for i, c := range cases {
		t.Run(fmt.Sprintf("case %d", i), func(t *testing.T) {
			res, err := calcClassicBucketStatistics(c.matrix)
			require.NoError(t, err)
			require.Equal(t, c.expected, res)
		})
	}
}
