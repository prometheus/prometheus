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

package promql

import (
	"context"
	"testing"
	"time"

	"github.com/prometheus/prometheus/pkg/labels"
	"github.com/prometheus/prometheus/pkg/timestamp"
	"github.com/prometheus/prometheus/util/testutil"
)

func TestDeriv(t *testing.T) {
	// https://github.com/prometheus/prometheus/issues/2674#issuecomment-315439393
	// This requires more precision than the usual test system offers,
	// so we test it by hand.
	storage := testutil.NewStorage(t)
	defer storage.Close()
	opts := EngineOpts{
		Logger:        nil,
		Reg:           nil,
		MaxConcurrent: 10,
		MaxSamples:    10000,
		Timeout:       10 * time.Second,
	}
	engine := NewEngine(opts)

	a, err := storage.Appender()
	testutil.Ok(t, err)

	metric := labels.FromStrings("__name__", "foo")
	a.Add(metric, 1493712816939, 1.0)
	a.Add(metric, 1493712846939, 1.0)

	err = a.Commit()
	testutil.Ok(t, err)

	query, err := engine.NewInstantQuery(storage, "deriv(foo[30m])", timestamp.Time(1493712846939))
	testutil.Ok(t, err)

	result := query.Exec(context.Background())
	testutil.Ok(t, result.Err)

	vec, _ := result.Vector()
	testutil.Assert(t, len(vec) == 1, "Expected 1 result, got %d", len(vec))
	testutil.Assert(t, vec[0].V == 0.0, "Expected 0.0 as value, got %f", vec[0].V)
}
