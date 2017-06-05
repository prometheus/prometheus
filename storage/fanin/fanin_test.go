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

package fanin

import (
	"reflect"
	"sort"
	"testing"
	"time"

	"github.com/prometheus/common/model"
	"golang.org/x/net/context"

	"github.com/prometheus/prometheus/storage/local"
	"github.com/prometheus/prometheus/storage/metric"
	"github.com/prometheus/prometheus/storage/remote"
)

type testQuerier struct {
	series model.Matrix
}

func (q testQuerier) QueryRange(ctx context.Context, from, through model.Time, matchers ...*metric.LabelMatcher) ([]local.SeriesIterator, error) {
	var outMatrix model.Matrix
	for _, s := range q.series {
		for _, m := range matchers {
			if !m.Match(s.Metric[m.Name]) {
				continue
			}
		}

		fromIdx := sort.Search(len(s.Values), func(i int) bool {
			return !s.Values[i].Timestamp.Before(from)
		})
		throughIdx := sort.Search(len(s.Values), func(i int) bool {
			return s.Values[i].Timestamp.After(through)
		})

		outMatrix = append(outMatrix, &model.SampleStream{
			Metric: s.Metric,
			Values: s.Values[fromIdx:throughIdx],
		})
	}

	return remote.MatrixToIterators(outMatrix, nil)
}

func (q testQuerier) QueryInstant(ctx context.Context, ts model.Time, stalenessDelta time.Duration, matchers ...*metric.LabelMatcher) ([]local.SeriesIterator, error) {
	return q.QueryRange(ctx, ts.Add(-stalenessDelta), ts, matchers...)
}

func (q testQuerier) MetricsForLabelMatchers(ctx context.Context, from, through model.Time, matcherSets ...metric.LabelMatchers) ([]metric.Metric, error) {
	fpToMetric := map[model.Fingerprint]model.Metric{}
	for _, s := range q.series {
		matched := false
		for _, matchers := range matcherSets {
			for _, m := range matchers {
				if !m.Match(s.Metric[m.Name]) {
					continue
				}
			}

			matched = true
		}

		fromIdx := sort.Search(len(s.Values), func(i int) bool {
			return !s.Values[i].Timestamp.Before(from)
		})
		throughIdx := sort.Search(len(s.Values), func(i int) bool {
			return s.Values[i].Timestamp.After(through)
		})

		if fromIdx == throughIdx {
			continue
		}

		if matched {
			fpToMetric[s.Metric.Fingerprint()] = s.Metric
		}
	}

	metrics := make([]metric.Metric, 0, len(fpToMetric))
	for _, m := range fpToMetric {
		metrics = append(metrics, metric.Metric{Metric: m})
	}
	return metrics, nil
}

func (q testQuerier) LastSampleForLabelMatchers(ctx context.Context, cutoff model.Time, matcherSets ...metric.LabelMatchers) (model.Vector, error) {
	panic("not implemented")
}

func (q testQuerier) LabelValuesForLabelName(ctx context.Context, ln model.LabelName) (model.LabelValues, error) {
	panic("not implemented")
}

func (q testQuerier) Close() error {
	return nil
}

func TestQueryRange(t *testing.T) {
	type query struct {
		from      model.Time
		through   model.Time
		out       model.Matrix
		localOnly bool
	}

	tests := []struct {
		name    string
		local   model.Matrix
		remote  []model.Matrix
		queries []query
	}{
		{
			name: "duplicate samples are eliminated",
			local: model.Matrix{
				&model.SampleStream{
					Metric: model.Metric{
						model.MetricNameLabel: "testmetric",
					},
					Values: []model.SamplePair{
						{
							Timestamp: 1,
							Value:     1,
						},
						{
							Timestamp: 2,
							Value:     2,
						},
					},
				},
			},
			remote: []model.Matrix{
				{
					&model.SampleStream{
						Metric: model.Metric{
							model.MetricNameLabel: "testmetric",
						},
						Values: []model.SamplePair{
							{
								Timestamp: 0,
								Value:     0,
							},
							{
								Timestamp: 1,
								Value:     1,
							},
							{
								Timestamp: 2,
								Value:     2,
							},
						},
					},
				},
				{
					&model.SampleStream{
						Metric: model.Metric{
							model.MetricNameLabel: "testmetric",
						},
						Values: []model.SamplePair{
							{
								Timestamp: 0,
								Value:     0,
							},
							{
								Timestamp: 1,
								Value:     1,
							},
							{
								Timestamp: 2,
								Value:     2,
							},
						},
					},
				},
			},
			queries: []query{
				{
					from:    0,
					through: 1,
					out: model.Matrix{
						&model.SampleStream{
							Metric: model.Metric{
								model.MetricNameLabel: "testmetric",
							},
							Values: []model.SamplePair{
								{
									Timestamp: 0,
									Value:     0,
								},
								{
									Timestamp: 1,
									Value:     1,
								},
							},
						},
					},
				},
			},
		},

		{
			name: "remote data is thrown away after first local sample",
			local: model.Matrix{
				&model.SampleStream{
					Metric: model.Metric{
						model.MetricNameLabel: "testmetric",
					},
					Values: []model.SamplePair{
						{
							Timestamp: 2,
							Value:     2,
						},
						{
							Timestamp: 4,
							Value:     4,
						},
					},
				},
			},
			remote: []model.Matrix{
				{
					&model.SampleStream{
						Metric: model.Metric{
							model.MetricNameLabel: "testmetric",
						},
						Values: []model.SamplePair{
							{
								Timestamp: 0,
								Value:     0,
							},
							{
								Timestamp: 2,
								Value:     20,
							},
							{
								Timestamp: 4,
								Value:     40,
							},
						},
					},
				},
				{
					&model.SampleStream{
						Metric: model.Metric{
							model.MetricNameLabel: "testmetric",
						},
						Values: []model.SamplePair{
							{
								Timestamp: 1,
								Value:     10,
							},
							{
								Timestamp: 3,
								Value:     30,
							},
						},
					},
				},
			},
			queries: []query{
				{
					from:    0,
					through: 4,
					out: model.Matrix{
						&model.SampleStream{
							Metric: model.Metric{
								model.MetricNameLabel: "testmetric",
							},
							Values: []model.SamplePair{
								{
									Timestamp: 0,
									Value:     0,
								},
								{
									Timestamp: 1,
									Value:     10,
								},
								{
									Timestamp: 2,
									Value:     2,
								},
								{
									Timestamp: 4,
									Value:     4,
								},
							},
						},
					},
				},
				{
					from:    2,
					through: 2,
					out: model.Matrix{
						&model.SampleStream{
							Metric: model.Metric{
								model.MetricNameLabel: "testmetric",
							},
							Values: []model.SamplePair{
								{
									Timestamp: 2,
									Value:     2,
								},
							},
						},
					},
				},
			},
		},

		{
			name: "no local data",
			remote: []model.Matrix{
				{
					&model.SampleStream{
						Metric: model.Metric{
							model.MetricNameLabel: "testmetric",
						},
						Values: []model.SamplePair{
							{
								Timestamp: 0,
								Value:     0,
							},
							{
								Timestamp: 2,
								Value:     20,
							},

							{
								Timestamp: 4,
								Value:     40,
							},
						},
					},
				},
				{
					&model.SampleStream{
						Metric: model.Metric{
							model.MetricNameLabel: "testmetric",
						},
						Values: []model.SamplePair{
							{
								Timestamp: 1,
								Value:     10,
							},
							{
								Timestamp: 3,
								Value:     30,
							},
						},
					},
				},
			},
			queries: []query{
				{
					from:    0,
					through: 4,
					out: model.Matrix{
						&model.SampleStream{
							Metric: model.Metric{
								model.MetricNameLabel: "testmetric",
							},
							Values: []model.SamplePair{
								{
									Timestamp: 0,
									Value:     0,
								},
								{
									Timestamp: 1,
									Value:     10,
								},
								{
									Timestamp: 2,
									Value:     20,
								},
								{
									Timestamp: 3,
									Value:     30,
								},
								{
									Timestamp: 4,
									Value:     40,
								},
							},
						},
					},
				},
				{
					from:    2,
					through: 2,
					out: model.Matrix{
						&model.SampleStream{
							Metric: model.Metric{
								model.MetricNameLabel: "testmetric",
							},
							Values: []model.SamplePair{
								{
									Timestamp: 2,
									Value:     20,
								},
							},
						},
					},
				},
			},
		},

		{
			name: "only local data",
			local: model.Matrix{
				&model.SampleStream{
					Metric: model.Metric{
						model.MetricNameLabel: "testmetric",
					},
					Values: []model.SamplePair{
						{
							Timestamp: 0,
							Value:     0,
						},
						{
							Timestamp: 1,
							Value:     1,
						},

						{
							Timestamp: 2,
							Value:     2,
						},
					},
				},
			},
			queries: []query{
				{
					from:    0,
					through: 4,
					out: model.Matrix{
						&model.SampleStream{
							Metric: model.Metric{
								model.MetricNameLabel: "testmetric",
							},
							Values: []model.SamplePair{
								{
									Timestamp: 0,
									Value:     0,
								},
								{
									Timestamp: 1,
									Value:     1,
								},
								{
									Timestamp: 2,
									Value:     2,
								},
							},
						},
					},
				},
				{
					from:    3,
					through: 3,
					out: model.Matrix{
						&model.SampleStream{
							Metric: model.Metric{
								model.MetricNameLabel: "testmetric",
							},
							Values: []model.SamplePair{
								{
									Timestamp: 2,
									Value:     2,
								},
							},
						},
					},
				},
			},
		},

		{
			name: "context value to indicate only local querying is set",
			local: model.Matrix{
				&model.SampleStream{
					Metric: model.Metric{
						model.MetricNameLabel: "testmetric",
					},
					Values: []model.SamplePair{
						{
							Timestamp: 2,
							Value:     2,
						},
						{
							Timestamp: 3,
							Value:     3,
						},
					},
				},
			},
			remote: []model.Matrix{
				{
					&model.SampleStream{
						Metric: model.Metric{
							model.MetricNameLabel: "testmetric",
						},
						Values: []model.SamplePair{
							{
								Timestamp: 0,
								Value:     0,
							},
							{
								Timestamp: 1,
								Value:     1,
							},
							{
								Timestamp: 2,
								Value:     2,
							},
						},
					},
				},
			},
			queries: []query{
				{
					from:      0,
					through:   3,
					localOnly: true,
					out: model.Matrix{
						&model.SampleStream{
							Metric: model.Metric{
								model.MetricNameLabel: "testmetric",
							},
							Values: []model.SamplePair{
								{
									Timestamp: 2,
									Value:     2,
								},
								{
									Timestamp: 3,
									Value:     3,
								},
							},
						},
					},
				},
				{
					from:      1,
					through:   1,
					localOnly: true,
					out: model.Matrix{
						&model.SampleStream{
							Metric: model.Metric{
								model.MetricNameLabel: "testmetric",
							},
							Values: []model.SamplePair{model.ZeroSamplePair},
						},
					},
				},
				{
					from:      2,
					through:   2,
					localOnly: true,
					out: model.Matrix{
						&model.SampleStream{
							Metric: model.Metric{
								model.MetricNameLabel: "testmetric",
							},
							Values: []model.SamplePair{
								{
									Timestamp: 2,
									Value:     2,
								},
							},
						},
					},
				},
			},
		},
	}

	matcher, err := metric.NewLabelMatcher(metric.Equal, model.MetricNameLabel, "testmetric")
	if err != nil {
		t.Fatal(err)
	}

	for _, test := range tests {
		q := querier{
			local: &testQuerier{test.local},
		}
		for _, m := range test.remote {
			q.remotes = append(q.remotes, &testQuerier{m})
		}

		for i, query := range test.queries {
			ctx := context.Background()
			if query.localOnly {
				ctx = WithLocalOnly(ctx)
			}
			var its []local.SeriesIterator
			var err error
			if query.from == query.through {
				its, err = q.QueryInstant(ctx, query.from, 5*time.Minute, matcher)
			} else {
				its, err = q.QueryRange(ctx, query.from, query.through, matcher)
			}
			if err != nil {
				t.Fatal(err)
			}
			if err = q.Close(); err != nil {
				t.Fatal(err)
			}

			out := make(model.Matrix, 0, len(query.out))
			for _, it := range its {
				var values []model.SamplePair
				if query.from == query.through {
					values = []model.SamplePair{it.ValueAtOrBeforeTime(query.from)}
				} else {
					values = it.RangeValues(metric.Interval{
						OldestInclusive: query.from,
						NewestInclusive: query.through,
					})
				}
				it.Close()

				out = append(out, &model.SampleStream{
					Metric: it.Metric().Metric,
					Values: values,
				})
			}

			sort.Sort(out)
			sort.Sort(query.out)

			if !reflect.DeepEqual(out, query.out) {
				t.Fatalf("Test case %q, query %d: Unexpected query result;\n\ngot:\n\n%s\n\nwant:\n\n%s", test.name, i, out, query.out)
			}
		}
	}
}

func TestMetricsForLabelMatchersIgnoresRemoteData(t *testing.T) {
	q := querier{
		local: &testQuerier{
			series: model.Matrix{
				&model.SampleStream{
					Metric: model.Metric{
						model.MetricNameLabel: "testmetric",
						"testlabel":           "testvalue1",
					},
					Values: []model.SamplePair{{
						Timestamp: 1, Value: 1,
					}},
				},
				&model.SampleStream{
					Metric: model.Metric{
						model.MetricNameLabel: "testmetric",
						"testlabel":           "testvalue2",
					},
					Values: []model.SamplePair{{
						Timestamp: 1, Value: 1,
					}},
				},
			},
		},
		remotes: []local.Querier{
			&testQuerier{
				series: model.Matrix{
					&model.SampleStream{
						Metric: model.Metric{
							model.MetricNameLabel: "testmetric",
							"testlabel":           "testvalue2",
						},
						Values: []model.SamplePair{{
							Timestamp: 1, Value: 1,
						}},
					},
					&model.SampleStream{
						Metric: model.Metric{
							model.MetricNameLabel: "testmetric",
							"testlabel":           "testvalue3",
						},
						Values: []model.SamplePair{{
							Timestamp: 1, Value: 1,
						}},
					},
				},
			},
		},
	}

	matcher, err := metric.NewLabelMatcher(metric.Equal, model.MetricNameLabel, "testmetric")
	if err != nil {
		t.Fatal(err)
	}
	got, err := q.MetricsForLabelMatchers(context.Background(), 0, 1, metric.LabelMatchers{matcher})
	if err != nil {
		t.Fatal(err)
	}
	sort.Slice(got, func(i, j int) bool {
		return got[i].Metric.Before(got[j].Metric)
	})

	want := []metric.Metric{
		{
			Metric: model.Metric{
				model.MetricNameLabel: "testmetric",
				"testlabel":           "testvalue1",
			},
		},
		{
			Metric: model.Metric{
				model.MetricNameLabel: "testmetric",
				"testlabel":           "testvalue2",
			},
		},
	}

	if !reflect.DeepEqual(want, got) {
		t.Fatalf("Unexpected metric returned;\n\nwant:\n\n%#v\n\ngot:\n\n%#v", want, got)
	}
}
