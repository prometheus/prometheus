// Copyright The Prometheus Authors
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

package template

import (
	"context"
	"net/url"
	"reflect"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus/testutil"
	"github.com/stretchr/testify/require"

	"github.com/prometheus/prometheus/model/histogram"
)

func TestExpandTextPreservesDirectHistogramValues(t *testing.T) {
	value := &histogram.FloatHistogram{Count: 3, Sum: 10}
	funcMap := NewFuncMap(context.Background(), 0, nil, &url.URL{})

	got, err := ExpandText("histogram", "{{ . | value }}", &sample{Value: value}, funcMap, nil)
	require.NoError(t, err)
	require.Equal(t, value.String(), got)
}

func TestFloatToTime(t *testing.T) {
	type args struct {
		v float64
	}
	tests := []struct {
		name    string
		args    args
		want    *time.Time
		wantErr bool
	}{
		{
			name: "happy path",
			args: args{v: 1657155181},
			want: func() *time.Time {
				tm := time.Date(2022, 7, 7, 0, 53, 1, 0, time.UTC)
				return &tm
			}(),
		},
		{
			name:    "more than math.MaxInt64",
			args:    args{v: 1.79769313486231570814527423731704356798070e+300},
			wantErr: true,
		},
		{
			name:    "less than math.MinInt64",
			args:    args{v: -1.79769313486231570814527423731704356798070e+300},
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := floatToTime(tt.args.v)
			require.Equal(t, tt.wantErr, err != nil)
			require.True(t, reflect.DeepEqual(tt.want, got), "got %v, want %v", got, tt.want)
		})
	}
}

func TestBoundTemplateCountsExpansionAttempts(t *testing.T) {
	beforeTotal := testutil.ToFloat64(templateTextExpansionTotal)
	beforeFailures := testutil.ToFloat64(templateTextExpansionFailures)
	funcMap := NewFuncMap(context.Background(), 0, nil, &url.URL{})

	bound := CompileBoundTemplate("counter-test", "{{.}}", funcMap, nil)
	require.Equal(t, beforeTotal, testutil.ToFloat64(templateTextExpansionTotal))
	got, err := ExpandBoundTemplate(bound, "value")
	require.NoError(t, err)
	require.Equal(t, "value", got)

	parseFailure := CompileBoundTemplate("counter-test", "{{", funcMap, nil)
	_, err = ExpandBoundTemplate(parseFailure, nil)
	require.ErrorContains(t, err, "error parsing template counter-test")

	executionFailure := CompileBoundTemplate("counter-test", "{{index . 1}}", funcMap, nil)
	_, err = ExpandBoundTemplate(executionFailure, []string{"only"})
	require.ErrorContains(t, err, "error executing template counter-test")

	require.Equal(t, float64(3), testutil.ToFloat64(templateTextExpansionTotal)-beforeTotal)
	require.Equal(t, float64(2), testutil.ToFloat64(templateTextExpansionFailures)-beforeFailures)
}
