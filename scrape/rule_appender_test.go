// Copyright 2015 The Prometheus Authors
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

package scrape

import (
	"context"
	"testing"
	"time"

	"github.com/prometheus/prometheus/config"
	"github.com/prometheus/prometheus/model/labels"
	"github.com/stretchr/testify/require"
)

func TestAggregatorAppender(t *testing.T) {
	now := time.Now()
	samples := []sample{
		{
			metric: []labels.Label{{Name: "__name__", Value: "http_requests_total"}, {Name: "code", Value: "200"}, {Name: "handler", Value: "/"}},
			t:      now.UnixMilli(),
			v:      10,
		},
		{
			metric: []labels.Label{{Name: "__name__", Value: "http_requests_total"}, {Name: "code", Value: "200"}, {Name: "handler", Value: "/metrics"}},
			t:      now.UnixMilli(),
			v:      6,
		},
	}
	rules := []*config.AggregationRuleConfig{
		{
			Expr:   "sum by (code) (http_requests_total)",
			Record: "code:http_requests_total:sum",
		},
	}
	instanceLabels := []labels.Label{{Name: "instance", Value: "127.0.0.1"}}
	aggregator := newRuleAppender(instanceLabels, collectResultAppendable{}, rules)
	appender := aggregator.Appender(context.Background())
	for _, s := range samples {
		if _, err := appender.Append(0, s.metric, s.t, s.v); err != nil {
			t.Fatal(err)
		}
	}

	err := appender.Commit()
	require.NoError(t, err)

	result := appender.(*batch).appender.(*collectResultAppender).result
	if len(result) != 3 {
		t.Fatalf("Invalid sample count, got %d, want %d", len(result), 3)
	}

	expectedSamples := append(samples, sample{
		metric: append(labels.Labels{{Name: "code", Value: "200"}, {Name: "__name__", Value: "code:http_requests_total:sum"}}, instanceLabels...),
		t:      now.UnixMilli(),
		v:      16,
	})
	require.Equal(t, expectedSamples, result)
}
