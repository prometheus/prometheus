// Copyright 2022 The Prometheus Authors
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

package promql

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/prometheus/prometheus/model/labels"
)

func TestVector_ContainsSameLabelset(t *testing.T) {
	for name, tc := range map[string]struct {
		vector   Vector
		expected bool
	}{
		"empty vector": {
			vector:   Vector{},
			expected: false,
		},
		"vector with one series": {
			vector: Vector{
				{Metric: labels.FromStrings("lbl", "a")},
			},
			expected: false,
		},
		"vector with two different series": {
			vector: Vector{
				{Metric: labels.FromStrings("lbl", "a")},
				{Metric: labels.FromStrings("lbl", "b")},
			},
			expected: false,
		},
		"vector with two equal series": {
			vector: Vector{
				{Metric: labels.FromStrings("lbl", "a")},
				{Metric: labels.FromStrings("lbl", "a")},
			},
			expected: true,
		},
		"vector with three series, two equal": {
			vector: Vector{
				{Metric: labels.FromStrings("lbl", "a")},
				{Metric: labels.FromStrings("lbl", "b")},
				{Metric: labels.FromStrings("lbl", "a")},
			},
			expected: true,
		},
	} {
		t.Run(name, func(t *testing.T) {
			require.Equal(t, tc.expected, tc.vector.ContainsSameLabelset())
		})
	}
}

func TestMatrix_ContainsSameLabelset(t *testing.T) {
	for name, tc := range map[string]struct {
		matrix   Matrix
		expected bool
	}{
		"empty matrix": {
			matrix:   Matrix{},
			expected: false,
		},
		"matrix with one series": {
			matrix: Matrix{
				{Metric: labels.FromStrings("lbl", "a")},
			},
			expected: false,
		},
		"matrix with two different series": {
			matrix: Matrix{
				{Metric: labels.FromStrings("lbl", "a")},
				{Metric: labels.FromStrings("lbl", "b")},
			},
			expected: false,
		},
		"matrix with two equal series": {
			matrix: Matrix{
				{Metric: labels.FromStrings("lbl", "a")},
				{Metric: labels.FromStrings("lbl", "a")},
			},
			expected: true,
		},
		"matrix with three series, two equal": {
			matrix: Matrix{
				{Metric: labels.FromStrings("lbl", "a")},
				{Metric: labels.FromStrings("lbl", "b")},
				{Metric: labels.FromStrings("lbl", "a")},
			},
			expected: true,
		},
	} {
		t.Run(name, func(t *testing.T) {
			require.Equal(t, tc.expected, tc.matrix.ContainsSameLabelset())
		})
	}
}
