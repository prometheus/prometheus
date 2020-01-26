// Copyright 2018, OpenCensus Authors
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package ocagent

import (
	"encoding/json"
	"reflect"
	"testing"
	"time"

	"go.opencensus.io/stats"
	"go.opencensus.io/stats/view"
	"go.opencensus.io/tag"

	metricspb "github.com/census-instrumentation/opencensus-proto/gen-go/metrics/v1"

	"github.com/golang/protobuf/ptypes/timestamp"
)

var (
	keyField, _      = tag.NewKey("field")
	keyName, _       = tag.NewKey("name")
	keyPlayerName, _ = tag.NewKey("player_name")

	mSprinterLatencyMs = stats.Float64("sprint_latency", "The time in which a sprinter completes the course", "ms")
	mFouls             = stats.Int64("fouls", "The number of fouls reported", "1")
)

type test struct {
	in      *view.Data
	want    *metricspb.Metric
	wantErr string
}

func TestViewDataToMetrics_Distribution(t *testing.T) {
	startTime := time.Date(2018, 11, 25, 15, 38, 18, 997, time.UTC)
	endTime := startTime.Add(100 * time.Millisecond)

	tests := []*test{
		{in: nil, wantErr: "expecting non-nil a view.Data"},
		{in: &view.Data{}, wantErr: "expecting non-nil a view.View"},
		{in: &view.Data{View: &view.View{}}, wantErr: "expecting a non-nil stats.Measure"},
		{
			in: &view.Data{
				Start: startTime,
				End:   endTime,
				View: &view.View{
					Name:        "ocagent.io/latency",
					Description: "latency of runners for a 100m dash",
					Aggregation: view.Distribution(0, 10, 20, 30, 40),
					TagKeys:     []tag.Key{keyField, keyName},
					Measure:     mSprinterLatencyMs,
				},
				Rows: []*view.Row{
					{
						Tags: []tag.Tag{
							{Key: keyField, Value: "main-field"},
							{Key: keyName, Value: "sprinter-#10"},
						},
						Data: &view.DistributionData{
							// Points: [11.9]
							Count:           1,
							Min:             11.9,
							Max:             11.9,
							Mean:            11.9,
							CountPerBucket:  []int64{0, 1, 0, 0, 0},
							SumOfSquaredDev: 0,
						},
					},
					{
						Tags: []tag.Tag{
							{Key: keyField, Value: "small-field"},
							{Key: keyName, Value: ""},
						},
						Data: &view.DistributionData{
							// Points: [20.2]
							Count:           1,
							Min:             20.2,
							Max:             20.2,
							Mean:            20.2,
							CountPerBucket:  []int64{0, 0, 1, 0, 0},
							SumOfSquaredDev: 0,
						},
					},
					{
						Tags: []tag.Tag{
							{Key: keyField, Value: "small-field"},
							{Key: keyName, Value: "sprinter-#yp"},
						},
						Data: &view.DistributionData{
							// Points: [28.9]
							Count:           1,
							Min:             28.9,
							Max:             28.9,
							Mean:            28.9,
							CountPerBucket:  []int64{0, 0, 1, 0, 0},
							SumOfSquaredDev: 0,
						},
					},
				},
			},
			want: &metricspb.Metric{
				MetricDescriptor: &metricspb.MetricDescriptor{
					Name:        "ocagent.io/latency",
					Description: "latency of runners for a 100m dash",
					Unit:        "ms", // Derived from the measure
					Type:        metricspb.MetricDescriptor_CUMULATIVE_DISTRIBUTION,
					LabelKeys: []*metricspb.LabelKey{
						{Key: "field"},
						{Key: "name"},
					},
				},
				Timeseries: []*metricspb.TimeSeries{
					{
						StartTimestamp: &timestamp.Timestamp{
							Seconds: 1543160298,
							Nanos:   997,
						},
						LabelValues: []*metricspb.LabelValue{
							{Value: "main-field", HasValue: true},
							{Value: "sprinter-#10", HasValue: true},
						},
						Points: []*metricspb.Point{
							{
								Timestamp: &timestamp.Timestamp{
									Seconds: 1543160298,
									Nanos:   100000997,
								},
								Value: &metricspb.Point_DistributionValue{
									DistributionValue: &metricspb.DistributionValue{
										Count:                 1,
										Sum:                   11.9,
										SumOfSquaredDeviation: 0,
										Buckets: []*metricspb.DistributionValue_Bucket{
											{}, {Count: 1}, {}, {}, {},
										},
										BucketOptions: &metricspb.DistributionValue_BucketOptions{
											Type: &metricspb.DistributionValue_BucketOptions_Explicit_{
												Explicit: &metricspb.DistributionValue_BucketOptions_Explicit{
													Bounds: []float64{0, 10, 20, 30, 40},
												},
											},
										},
									},
								},
							},
						},
					},
					{
						StartTimestamp: &timestamp.Timestamp{
							Seconds: 1543160298,
							Nanos:   997,
						},
						LabelValues: []*metricspb.LabelValue{
							{Value: "small-field", HasValue: true},
							{Value: "", HasValue: true},
						},
						Points: []*metricspb.Point{
							{
								Timestamp: &timestamp.Timestamp{
									Seconds: 1543160298,
									Nanos:   100000997,
								},
								Value: &metricspb.Point_DistributionValue{
									DistributionValue: &metricspb.DistributionValue{
										Count:                 1,
										Sum:                   20.2,
										SumOfSquaredDeviation: 0,
										Buckets: []*metricspb.DistributionValue_Bucket{
											{}, {}, {Count: 1}, {}, {},
										},
										BucketOptions: &metricspb.DistributionValue_BucketOptions{
											Type: &metricspb.DistributionValue_BucketOptions_Explicit_{
												Explicit: &metricspb.DistributionValue_BucketOptions_Explicit{
													Bounds: []float64{0, 10, 20, 30, 40},
												},
											},
										},
									},
								},
							},
						},
					},
					{
						StartTimestamp: &timestamp.Timestamp{
							Seconds: 1543160298,
							Nanos:   997,
						},
						LabelValues: []*metricspb.LabelValue{
							{Value: "small-field", HasValue: true},
							{Value: "sprinter-#yp", HasValue: true},
						},
						Points: []*metricspb.Point{
							{
								Timestamp: &timestamp.Timestamp{
									Seconds: 1543160298,
									Nanos:   100000997,
								},
								Value: &metricspb.Point_DistributionValue{
									DistributionValue: &metricspb.DistributionValue{
										Count:                 1,
										Sum:                   28.9,
										SumOfSquaredDeviation: 0,
										Buckets: []*metricspb.DistributionValue_Bucket{
											{}, {}, {Count: 1}, {}, {},
										},
										BucketOptions: &metricspb.DistributionValue_BucketOptions{
											Type: &metricspb.DistributionValue_BucketOptions_Explicit_{
												Explicit: &metricspb.DistributionValue_BucketOptions_Explicit{
													Bounds: []float64{0, 10, 20, 30, 40},
												},
											},
										},
									},
								},
							},
						},
					},
				},
				Resource: nil,
			},
		},
		{
			in: &view.Data{
				Start: startTime,
				End:   endTime,
				View: &view.View{
					Name:        "ocagent.io/fouls",
					Description: "The number of fouls recorded",
					Aggregation: view.Distribution(0, 5, 10, 20, 50),
					TagKeys:     []tag.Key{keyField, keyName, keyPlayerName},
					Measure:     mFouls,
				},
				Rows: []*view.Row{
					{
						Tags: []tag.Tag{
							{Key: keyField, Value: "main-field"},
							{Key: keyName, Value: "sprinter-#10"},
							{Key: keyPlayerName, Value: "player-1"},
						},
						Data: &view.DistributionData{
							// Points: [26]
							Count:           1,
							Min:             26,
							Max:             26,
							Mean:            26,
							CountPerBucket:  []int64{0, 0, 0, 1, 0},
							SumOfSquaredDev: 0,
						},
					},
					{
						Tags: []tag.Tag{
							{Key: keyField, Value: "small-field"},
							{Key: keyName, Value: ""},
							{Key: keyPlayerName, Value: "player-2"},
						},
						Data: &view.DistributionData{
							// Points: [3]
							Count:           1,
							Min:             3,
							Max:             3,
							Mean:            3,
							CountPerBucket:  []int64{1, 0, 0, 0, 0},
							SumOfSquaredDev: 0,
						},
					},
				},
			},
			want: &metricspb.Metric{
				MetricDescriptor: &metricspb.MetricDescriptor{
					Name:        "ocagent.io/fouls",
					Description: "The number of fouls recorded",
					Unit:        "1", // Derived from the measure
					Type:        metricspb.MetricDescriptor_CUMULATIVE_DISTRIBUTION,
					LabelKeys: []*metricspb.LabelKey{
						{Key: "field"},
						{Key: "name"},
						{Key: "player_name"},
					},
				},
				Timeseries: []*metricspb.TimeSeries{
					{
						StartTimestamp: &timestamp.Timestamp{
							Seconds: 1543160298,
							Nanos:   997,
						},
						LabelValues: []*metricspb.LabelValue{
							{Value: "main-field", HasValue: true},
							{Value: "sprinter-#10", HasValue: true},
							{Value: "player-1", HasValue: true},
						},
						Points: []*metricspb.Point{
							{
								Timestamp: &timestamp.Timestamp{
									Seconds: 1543160298,
									Nanos:   100000997,
								},
								Value: &metricspb.Point_DistributionValue{
									DistributionValue: &metricspb.DistributionValue{
										Count:                 1,
										Sum:                   26,
										SumOfSquaredDeviation: 0,
										Buckets: []*metricspb.DistributionValue_Bucket{
											{}, {}, {}, {Count: 1}, {},
										},
										BucketOptions: &metricspb.DistributionValue_BucketOptions{
											Type: &metricspb.DistributionValue_BucketOptions_Explicit_{
												Explicit: &metricspb.DistributionValue_BucketOptions_Explicit{
													Bounds: []float64{0, 5, 10, 20, 50},
												},
											},
										},
									},
								},
							},
						},
					},
					{
						StartTimestamp: &timestamp.Timestamp{
							Seconds: 1543160298,
							Nanos:   997,
						},
						LabelValues: []*metricspb.LabelValue{
							{Value: "small-field", HasValue: true},
							{Value: "", HasValue: true},
							{Value: "player-2", HasValue: true},
						},
						Points: []*metricspb.Point{
							{
								Timestamp: &timestamp.Timestamp{
									Seconds: 1543160298,
									Nanos:   100000997,
								},
								Value: &metricspb.Point_DistributionValue{
									DistributionValue: &metricspb.DistributionValue{
										Count:                 1,
										Sum:                   3,
										SumOfSquaredDeviation: 0,
										Buckets: []*metricspb.DistributionValue_Bucket{
											{Count: 1}, {}, {}, {}, {},
										},
										BucketOptions: &metricspb.DistributionValue_BucketOptions{
											Type: &metricspb.DistributionValue_BucketOptions_Explicit_{
												Explicit: &metricspb.DistributionValue_BucketOptions_Explicit{
													Bounds: []float64{0, 5, 10, 20, 50},
												},
											},
										},
									},
								},
							},
						},
					},
				},
				Resource: nil,
			},
		},
	}

	testViewDataToMetrics(t, tests)
}

func TestViewDataToMetrics_LastValue(t *testing.T) {
	startTime := time.Date(2018, 11, 25, 15, 38, 18, 997, time.UTC)
	endTime := startTime.Add(100 * time.Millisecond)

	tests := []*test{
		{in: nil, wantErr: "expecting non-nil a view.Data"},
		{in: &view.Data{}, wantErr: "expecting non-nil a view.View"},
		{in: &view.Data{View: &view.View{}}, wantErr: "expecting a non-nil stats.Measure"},
		{
			in: &view.Data{
				Start: startTime,
				End:   endTime,
				View: &view.View{
					Name:        "ocagent.io/chronospeed",
					Description: "the chronometer readings per referee",
					Aggregation: view.LastValue(),
					TagKeys:     []tag.Key{keyField, keyName},
					Measure:     mSprinterLatencyMs,
				},
				Rows: []*view.Row{
					{
						Tags: []tag.Tag{
							{Key: keyField, Value: "main-field"},
							{Key: keyName, Value: "sprinter-#10"},
						},
						Data: &view.LastValueData{Value: 50},
					},
					{
						Tags: []tag.Tag{
							{Key: keyField, Value: "small-field"},
							{Key: keyName, Value: "sprints"},
						},
						Data: &view.LastValueData{Value: 17},
					},
				},
			},
			want: &metricspb.Metric{
				MetricDescriptor: &metricspb.MetricDescriptor{
					Name:        "ocagent.io/chronospeed",
					Description: "the chronometer readings per referee",
					Unit:        "ms",
					Type:        metricspb.MetricDescriptor_GAUGE_DOUBLE,
					LabelKeys: []*metricspb.LabelKey{
						{Key: "field"},
						{Key: "name"},
					},
				},
				Timeseries: []*metricspb.TimeSeries{
					{
						StartTimestamp: &timestamp.Timestamp{
							Seconds: 1543160298,
							Nanos:   997,
						},
						LabelValues: []*metricspb.LabelValue{
							{Value: "main-field", HasValue: true},
							{Value: "sprinter-#10", HasValue: true},
						},
						Points: []*metricspb.Point{
							{
								Timestamp: &timestamp.Timestamp{
									Seconds: 1543160298,
									Nanos:   100000997,
								},
								Value: &metricspb.Point_DoubleValue{DoubleValue: 50},
							},
						},
					},
					{
						StartTimestamp: &timestamp.Timestamp{
							Seconds: 1543160298,
							Nanos:   997,
						},
						LabelValues: []*metricspb.LabelValue{
							{Value: "small-field", HasValue: true},
							{Value: "sprints", HasValue: true},
						},
						Points: []*metricspb.Point{
							{
								Timestamp: &timestamp.Timestamp{
									Seconds: 1543160298,
									Nanos:   100000997,
								},
								Value: &metricspb.Point_DoubleValue{DoubleValue: 17},
							},
						},
					},
				},
				Resource: nil,
			},
		},

		{
			in: &view.Data{
				Start: startTime,
				End:   endTime,
				View: &view.View{
					Name:        "ocagent.io/latency",
					Description: "latency of runners for a 100m dash",
					Aggregation: view.Distribution(0, 10, 20, 30, 40),
					TagKeys:     []tag.Key{keyField, keyName},
					Measure:     mSprinterLatencyMs,
				},
				Rows: []*view.Row{
					{
						Tags: []tag.Tag{
							{Key: keyField, Value: "main-field"},
							{Key: keyName, Value: "sprinter-#10"},
						},
						Data: &view.DistributionData{
							// Points: [11.9]
							Count:           1,
							Min:             11.9,
							Max:             11.9,
							Mean:            11.9,
							CountPerBucket:  []int64{0, 1, 0, 0, 0},
							SumOfSquaredDev: 0,
						},
					},
					{
						Tags: []tag.Tag{
							{Key: keyField, Value: "small-field"},
							{Key: keyName, Value: ""},
						},
						Data: &view.DistributionData{
							// Points: [20.2]
							Count:           1,
							Min:             20.2,
							Max:             20.2,
							Mean:            20.2,
							CountPerBucket:  []int64{0, 0, 1, 0, 0},
							SumOfSquaredDev: 0,
						},
					},
					{
						Tags: []tag.Tag{
							{Key: keyField, Value: "small-field"},
							{Key: keyName, Value: "sprinter-#yp"},
						},
						Data: &view.DistributionData{
							// Points: [28.9]
							Count:           1,
							Min:             28.9,
							Max:             28.9,
							Mean:            28.9,
							CountPerBucket:  []int64{0, 0, 1, 0, 0},
							SumOfSquaredDev: 0,
						},
					},
				},
			},
			want: &metricspb.Metric{
				MetricDescriptor: &metricspb.MetricDescriptor{
					Name:        "ocagent.io/latency",
					Description: "latency of runners for a 100m dash",
					Unit:        "ms", // Derived from the measure
					Type:        metricspb.MetricDescriptor_CUMULATIVE_DISTRIBUTION,
					LabelKeys: []*metricspb.LabelKey{
						{Key: "field"},
						{Key: "name"},
					},
				},
				Timeseries: []*metricspb.TimeSeries{
					{
						StartTimestamp: &timestamp.Timestamp{
							Seconds: 1543160298,
							Nanos:   997,
						},
						LabelValues: []*metricspb.LabelValue{
							{Value: "main-field", HasValue: true},
							{Value: "sprinter-#10", HasValue: true},
						},
						Points: []*metricspb.Point{
							{
								Timestamp: &timestamp.Timestamp{
									Seconds: 1543160298,
									Nanos:   100000997,
								},
								Value: &metricspb.Point_DistributionValue{
									DistributionValue: &metricspb.DistributionValue{
										Count:                 1,
										Sum:                   11.9,
										SumOfSquaredDeviation: 0,
										Buckets: []*metricspb.DistributionValue_Bucket{
											{}, {Count: 1}, {}, {}, {},
										},
										BucketOptions: &metricspb.DistributionValue_BucketOptions{
											Type: &metricspb.DistributionValue_BucketOptions_Explicit_{
												Explicit: &metricspb.DistributionValue_BucketOptions_Explicit{
													Bounds: []float64{0, 10, 20, 30, 40},
												},
											},
										},
									},
								},
							},
						},
					},
					{
						StartTimestamp: &timestamp.Timestamp{
							Seconds: 1543160298,
							Nanos:   997,
						},
						LabelValues: []*metricspb.LabelValue{
							{Value: "small-field", HasValue: true},
							{Value: "", HasValue: true},
						},
						Points: []*metricspb.Point{
							{
								Timestamp: &timestamp.Timestamp{
									Seconds: 1543160298,
									Nanos:   100000997,
								},
								Value: &metricspb.Point_DistributionValue{
									DistributionValue: &metricspb.DistributionValue{
										Count:                 1,
										Sum:                   20.2,
										SumOfSquaredDeviation: 0,
										Buckets: []*metricspb.DistributionValue_Bucket{
											{}, {}, {Count: 1}, {}, {},
										},
										BucketOptions: &metricspb.DistributionValue_BucketOptions{
											Type: &metricspb.DistributionValue_BucketOptions_Explicit_{
												Explicit: &metricspb.DistributionValue_BucketOptions_Explicit{
													Bounds: []float64{0, 10, 20, 30, 40},
												},
											},
										},
									},
								},
							},
						},
					},
					{
						StartTimestamp: &timestamp.Timestamp{
							Seconds: 1543160298,
							Nanos:   997,
						},
						LabelValues: []*metricspb.LabelValue{
							{Value: "small-field", HasValue: true},
							{Value: "sprinter-#yp", HasValue: true},
						},
						Points: []*metricspb.Point{
							{
								Timestamp: &timestamp.Timestamp{
									Seconds: 1543160298,
									Nanos:   100000997,
								},
								Value: &metricspb.Point_DistributionValue{
									DistributionValue: &metricspb.DistributionValue{
										Count:                 1,
										Sum:                   28.9,
										SumOfSquaredDeviation: 0,
										Buckets: []*metricspb.DistributionValue_Bucket{
											{}, {}, {Count: 1}, {}, {},
										},
										BucketOptions: &metricspb.DistributionValue_BucketOptions{
											Type: &metricspb.DistributionValue_BucketOptions_Explicit_{
												Explicit: &metricspb.DistributionValue_BucketOptions_Explicit{
													Bounds: []float64{0, 10, 20, 30, 40},
												},
											},
										},
									},
								},
							},
						},
					},
				},
				Resource: nil,
			},
		},
	}

	testViewDataToMetrics(t, tests)
}

func TestViewDataToMetrics_Count(t *testing.T) {
	startTime := time.Date(2018, 11, 25, 15, 38, 18, 997, time.UTC)
	endTime := startTime.Add(100 * time.Millisecond)

	tests := []*test{
		{in: nil, wantErr: "expecting non-nil a view.Data"},
		{in: &view.Data{}, wantErr: "expecting non-nil a view.View"},
		{in: &view.Data{View: &view.View{}}, wantErr: "expecting a non-nil stats.Measure"},

		// Testing with a stats.Float64 measure.
		{
			in: &view.Data{
				Start: startTime,
				End:   endTime,
				View: &view.View{
					Name:        "ocagent.io/counts",
					Description: "count of runners for a 100m dash",
					Aggregation: view.Count(),
					TagKeys:     []tag.Key{keyField, keyName},
					Measure:     mSprinterLatencyMs,
				},
				Rows: []*view.Row{
					{
						Tags: []tag.Tag{
							{Key: keyField, Value: "main-field"},
							{Key: keyName, Value: "sprinter-#10"},
						},
						Data: &view.CountData{Value: 10},
					},
					{
						Tags: []tag.Tag{
							{Key: keyField, Value: "small-field"},
							{Key: keyName, Value: "sprints"},
						},
						Data: &view.CountData{Value: 25},
					},
				},
			},
			want: &metricspb.Metric{
				MetricDescriptor: &metricspb.MetricDescriptor{
					Name:        "ocagent.io/counts",
					Description: "count of runners for a 100m dash",
					Unit:        "ms",
					Type:        metricspb.MetricDescriptor_CUMULATIVE_INT64,
					LabelKeys: []*metricspb.LabelKey{
						{Key: "field"},
						{Key: "name"},
					},
				},
				Timeseries: []*metricspb.TimeSeries{
					{
						StartTimestamp: &timestamp.Timestamp{
							Seconds: 1543160298,
							Nanos:   997,
						},
						LabelValues: []*metricspb.LabelValue{
							{Value: "main-field", HasValue: true},
							{Value: "sprinter-#10", HasValue: true},
						},
						Points: []*metricspb.Point{
							{
								Timestamp: &timestamp.Timestamp{
									Seconds: 1543160298,
									Nanos:   100000997,
								},
								Value: &metricspb.Point_Int64Value{Int64Value: 10},
							},
						},
					},
					{
						StartTimestamp: &timestamp.Timestamp{
							Seconds: 1543160298,
							Nanos:   997,
						},
						LabelValues: []*metricspb.LabelValue{
							{Value: "small-field", HasValue: true},
							{Value: "sprints", HasValue: true},
						},
						Points: []*metricspb.Point{
							{
								Timestamp: &timestamp.Timestamp{
									Seconds: 1543160298,
									Nanos:   100000997,
								},
								Value: &metricspb.Point_Int64Value{Int64Value: 25},
							},
						},
					},
				},
				Resource: nil,
			},
		},

		// Testing with a stats.Int64 measure.
		{
			in: &view.Data{
				Start: startTime,
				End:   endTime,
				View: &view.View{
					Name:        "ocagent.io/fouls",
					Description: "the number of fouls by players",
					Aggregation: view.Count(),
					TagKeys:     []tag.Key{keyField, keyName, keyPlayerName},
					Measure:     mFouls,
				},
				Rows: []*view.Row{
					{
						Tags: []tag.Tag{
							{Key: keyField, Value: "main-field"},
							{Key: keyName, Value: "sprinter-#10"},
							{Key: keyName, Value: "player_1"},
						},
						Data: &view.CountData{Value: 3},
					},
					{
						Tags: []tag.Tag{
							{Key: keyField, Value: "small-field"},
							{Key: keyName, Value: "sprints"},
							{Key: keyName, Value: "player_2"},
						},
						Data: &view.CountData{Value: 1},
					},
				},
			},
			want: &metricspb.Metric{
				MetricDescriptor: &metricspb.MetricDescriptor{
					Name:        "ocagent.io/fouls",
					Description: "the number of fouls by players",
					Unit:        "1",
					Type:        metricspb.MetricDescriptor_CUMULATIVE_INT64,
					LabelKeys: []*metricspb.LabelKey{
						{Key: "field"},
						{Key: "name"},
						{Key: "player_name"},
					},
				},
				Timeseries: []*metricspb.TimeSeries{
					{
						StartTimestamp: &timestamp.Timestamp{
							Seconds: 1543160298,
							Nanos:   997,
						},
						LabelValues: []*metricspb.LabelValue{
							{Value: "main-field", HasValue: true},
							{Value: "sprinter-#10", HasValue: true},
							{Value: "player_1", HasValue: true},
						},
						Points: []*metricspb.Point{
							{
								Timestamp: &timestamp.Timestamp{
									Seconds: 1543160298,
									Nanos:   100000997,
								},
								Value: &metricspb.Point_Int64Value{Int64Value: 3},
							},
						},
					},
					{
						StartTimestamp: &timestamp.Timestamp{
							Seconds: 1543160298,
							Nanos:   997,
						},
						LabelValues: []*metricspb.LabelValue{
							{Value: "small-field", HasValue: true},
							{Value: "sprints", HasValue: true},
							{Value: "player_2", HasValue: true},
						},
						Points: []*metricspb.Point{
							{
								Timestamp: &timestamp.Timestamp{
									Seconds: 1543160298,
									Nanos:   100000997,
								},
								Value: &metricspb.Point_Int64Value{Int64Value: 1},
							},
						},
					},
				},
				Resource: nil,
			},
		},
	}

	testViewDataToMetrics(t, tests)
}

func TestViewDataToMetrics_Sum(t *testing.T) {
	startTime := time.Date(2018, 11, 25, 15, 38, 18, 997, time.UTC)
	endTime := startTime.Add(100 * time.Millisecond)

	tests := []*test{
		{in: nil, wantErr: "expecting non-nil a view.Data"},
		{in: &view.Data{}, wantErr: "expecting non-nil a view.View"},
		{in: &view.Data{View: &view.View{}}, wantErr: "expecting a non-nil stats.Measure"},

		// Testing with a stats.Float64 measure.
		{
			in: &view.Data{
				Start: startTime,
				End:   endTime,
				View: &view.View{
					Name:        "ocagent.io/latency",
					Description: "speed of the various runners",
					Aggregation: view.Sum(),
					TagKeys:     []tag.Key{keyField, keyName},
					Measure:     mSprinterLatencyMs,
				},
				Rows: []*view.Row{
					{
						Tags: []tag.Tag{
							{Key: keyField, Value: "main-field"},
							{Key: keyName, Value: "sprinter-#10"},
						},
						Data: &view.SumData{Value: 27},
					},
					{
						Tags: []tag.Tag{
							{Key: keyField, Value: "small-field"},
							{Key: keyName, Value: "sprints"},
						},
						Data: &view.SumData{Value: 25},
					},
				},
			},
			want: &metricspb.Metric{
				MetricDescriptor: &metricspb.MetricDescriptor{
					Name:        "ocagent.io/latency",
					Description: "speed of the various runners",
					Unit:        "ms",
					Type:        metricspb.MetricDescriptor_CUMULATIVE_DOUBLE,
					LabelKeys: []*metricspb.LabelKey{
						{Key: "field"},
						{Key: "name"},
					},
				},
				Timeseries: []*metricspb.TimeSeries{
					{
						StartTimestamp: &timestamp.Timestamp{
							Seconds: 1543160298,
							Nanos:   997,
						},
						LabelValues: []*metricspb.LabelValue{
							{Value: "main-field", HasValue: true},
							{Value: "sprinter-#10", HasValue: true},
						},
						Points: []*metricspb.Point{
							{
								Timestamp: &timestamp.Timestamp{
									Seconds: 1543160298,
									Nanos:   100000997,
								},
								Value: &metricspb.Point_DoubleValue{DoubleValue: 27},
							},
						},
					},
					{
						StartTimestamp: &timestamp.Timestamp{
							Seconds: 1543160298,
							Nanos:   997,
						},
						LabelValues: []*metricspb.LabelValue{
							{Value: "small-field", HasValue: true},
							{Value: "sprints", HasValue: true},
						},
						Points: []*metricspb.Point{
							{
								Timestamp: &timestamp.Timestamp{
									Seconds: 1543160298,
									Nanos:   100000997,
								},
								Value: &metricspb.Point_DoubleValue{DoubleValue: 25},
							},
						},
					},
				},
				Resource: nil,
			},
		},

		// Testing with a stats.Int64 measure.
		{
			in: &view.Data{
				Start: startTime,
				End:   endTime,
				View: &view.View{
					Name:        "ocagent.io/fouls",
					Description: "the number of fouls by players",
					Aggregation: view.Sum(),
					TagKeys:     []tag.Key{keyField, keyName, keyPlayerName},
					Measure:     mFouls,
				},
				Rows: []*view.Row{
					{
						Tags: []tag.Tag{
							{Key: keyField, Value: "main-field"},
							{Key: keyName, Value: "sprinter-#10"},
							{Key: keyName, Value: "player_1"},
						},
						Data: &view.SumData{Value: 3},
					},
					{
						Tags: []tag.Tag{
							{Key: keyField, Value: "small-field"},
							{Key: keyName, Value: "sprints"},
							{Key: keyName, Value: "player_2"},
						},
						Data: &view.SumData{Value: 1},
					},
				},
			},
			want: &metricspb.Metric{
				MetricDescriptor: &metricspb.MetricDescriptor{
					Name:        "ocagent.io/fouls",
					Description: "the number of fouls by players",
					Unit:        "1",
					Type:        metricspb.MetricDescriptor_CUMULATIVE_INT64,
					LabelKeys: []*metricspb.LabelKey{
						{Key: "field"},
						{Key: "name"},
						{Key: "player_name"},
					},
				},
				Timeseries: []*metricspb.TimeSeries{
					{
						StartTimestamp: &timestamp.Timestamp{
							Seconds: 1543160298,
							Nanos:   997,
						},
						LabelValues: []*metricspb.LabelValue{
							{Value: "main-field", HasValue: true},
							{Value: "sprinter-#10", HasValue: true},
							{Value: "player_1", HasValue: true},
						},
						Points: []*metricspb.Point{
							{
								Timestamp: &timestamp.Timestamp{
									Seconds: 1543160298,
									Nanos:   100000997,
								},
								Value: &metricspb.Point_Int64Value{Int64Value: 3},
							},
						},
					},
					{
						StartTimestamp: &timestamp.Timestamp{
							Seconds: 1543160298,
							Nanos:   997,
						},
						LabelValues: []*metricspb.LabelValue{
							{Value: "small-field", HasValue: true},
							{Value: "sprints", HasValue: true},
							{Value: "player_2", HasValue: true},
						},
						Points: []*metricspb.Point{
							{
								Timestamp: &timestamp.Timestamp{
									Seconds: 1543160298,
									Nanos:   100000997,
								},
								Value: &metricspb.Point_Int64Value{Int64Value: 1},
							},
						},
					},
				},
				Resource: nil,
			},
		},
	}

	testViewDataToMetrics(t, tests)
}

func TestViewDataToMetrics_MissingVsEmptyLabelValues(t *testing.T) {
	startTime := time.Date(2018, 11, 25, 15, 38, 18, 997, time.UTC)
	endTime := startTime.Add(100 * time.Millisecond)

	tests := []*test{
		// Testing with a stats.Float64 measure.
		{
			in: &view.Data{
				Start: startTime,
				End:   endTime,
				View: &view.View{
					Name:        "ocagent.io/latency",
					Description: "speed of the various runners",
					Aggregation: view.Sum(),
					TagKeys:     []tag.Key{keyField, keyName, keyPlayerName},
					Measure:     mSprinterLatencyMs,
				},
				Rows: []*view.Row{
					{
						Tags: []tag.Tag{
							{}, // Testing a missing tag
							{Key: keyName, Value: "sprinter-#10"},
							{Key: keyPlayerName, Value: ""},
						},
						Data: &view.SumData{Value: 27},
					},
				},
			},
			want: &metricspb.Metric{
				MetricDescriptor: &metricspb.MetricDescriptor{
					Name:        "ocagent.io/latency",
					Description: "speed of the various runners",
					Unit:        "ms",
					Type:        metricspb.MetricDescriptor_CUMULATIVE_DOUBLE,
					LabelKeys: []*metricspb.LabelKey{
						{Key: "field"},
						{Key: "name"},
						{Key: "player_name"},
					},
				},
				Timeseries: []*metricspb.TimeSeries{
					{
						StartTimestamp: &timestamp.Timestamp{
							Seconds: 1543160298,
							Nanos:   997,
						},
						LabelValues: []*metricspb.LabelValue{
							{Value: "", HasValue: false},
							{Value: "sprinter-#10", HasValue: true},
							{Value: "", HasValue: true},
						},
						Points: []*metricspb.Point{
							{
								Timestamp: &timestamp.Timestamp{
									Seconds: 1543160298,
									Nanos:   100000997,
								},
								Value: &metricspb.Point_DoubleValue{DoubleValue: 27},
							},
						},
					},
				},
			},
		},

		// Testing with a stats.Int64 measure.
		{
			in: &view.Data{
				Start: startTime,
				End:   endTime,
				View: &view.View{
					Name:        "ocagent.io/fouls",
					Description: "the number of fouls by players",
					Aggregation: view.Sum(),
					TagKeys:     []tag.Key{keyField, keyName, keyPlayerName},
					Measure:     mFouls,
				},
				Rows: []*view.Row{
					{
						Tags: []tag.Tag{
							{},
							{Key: keyName, Value: "player_1"},
							{Key: keyField, Value: ""},
						},
						Data: &view.SumData{Value: 3},
					},
				},
			},
			want: &metricspb.Metric{
				MetricDescriptor: &metricspb.MetricDescriptor{
					Name:        "ocagent.io/fouls",
					Description: "the number of fouls by players",
					Unit:        "1",
					Type:        metricspb.MetricDescriptor_CUMULATIVE_INT64,
					LabelKeys: []*metricspb.LabelKey{
						{Key: "field"},
						{Key: "name"},
						{Key: "player_name"},
					},
				},
				Timeseries: []*metricspb.TimeSeries{
					{
						StartTimestamp: &timestamp.Timestamp{
							Seconds: 1543160298,
							Nanos:   997,
						},
						LabelValues: []*metricspb.LabelValue{
							{Value: "", HasValue: false},
							{Value: "player_1", HasValue: true},
							{Value: "", HasValue: true},
						},
						Points: []*metricspb.Point{
							{
								Timestamp: &timestamp.Timestamp{
									Seconds: 1543160298,
									Nanos:   100000997,
								},
								Value: &metricspb.Point_Int64Value{Int64Value: 3},
							},
						},
					},
				},
				Resource: nil,
			},
		},
	}

	testViewDataToMetrics(t, tests)
}

func testViewDataToMetrics(t *testing.T, tests []*test) {
	for i, tt := range tests {
		got, err := viewDataToMetric(tt.in)
		if tt.wantErr != "" {
			continue
		}

		// Otherwise we shouldn't get any error.
		if err != nil {
			if got != nil {
				t.Errorf("#%d: unexpected error %v with inconsistency (non-nil result): %v", i, err, got)
			} else {
				t.Errorf("#%d: unexpected error: %v", i, err)
			}
			continue
		}

		if !reflect.DeepEqual(got, tt.want) {
			gj := serializeAsJSON(got)
			wj := serializeAsJSON(tt.want)
			if gj != wj {
				t.Errorf("#%d: Unmatched JSON\nGot:\n\t%s\nWant:\n\t%s", i, gj, wj)
			}
		}
	}
}

func serializeAsJSON(v interface{}) string {
	blob, _ := json.MarshalIndent(v, "", "  ")
	return string(blob)
}
