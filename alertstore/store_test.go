// Copyright 2024 The Prometheus Authors
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

package alertstore

import (
	"context"
	"math"
	"os"
	"sort"
	"testing"
	"time"

	"github.com/go-kit/log"
	"github.com/stretchr/testify/require"

	"github.com/prometheus/prometheus/model/labels"
	"github.com/prometheus/prometheus/promql"
	"github.com/prometheus/prometheus/promql/parser"
	"github.com/prometheus/prometheus/promql/promqltest"
	"github.com/prometheus/prometheus/rules"
)

func TestKeepFiringForStateRestore(t *testing.T) {
	storage := promqltest.LoadedStorage(t, `
		load 5m
		http_requests{job="app-server", instance="0", group="canary"}	120 110 95 90 0 0 25 0 0 40 0 0
	`)
	t.Cleanup(func() { storage.Close() })

	expr, err := parser.ParseExpr(`http_requests{group="canary", job="app-server"} > 100`)
	require.NoError(t, err)

	store, err := NewBadgerDB("testdata/testalerts", log.NewNopLogger(), time.Minute*60)
	require.NoError(t, err, "Unable to start alertstore")
	defer func() {
		store.Close()
		os.RemoveAll("testdata")
	}()

	testEngine := promql.NewEngine(promql.EngineOpts{
		Logger:                   nil,
		Reg:                      nil,
		MaxSamples:               10000,
		Timeout:                  100 * time.Second,
		NoStepSubqueryIntervalFn: func(int64) int64 { return 60 * 1000 },
		EnableAtModifier:         true,
		EnableNegativeOffset:     true,
		EnablePerStepStats:       true,
	})

	opts := &rules.ManagerOptions{
		QueryFunc:       rules.EngineQueryFunc(testEngine, storage),
		Appendable:      storage,
		Queryable:       storage,
		Context:         context.Background(),
		Logger:          log.NewNopLogger(),
		NotifyFunc:      func(ctx context.Context, expr string, alerts ...*rules.Alert) {},
		OutageTolerance: 30 * time.Minute,
		ForGracePeriod:  10 * time.Minute,
		AlertStore:      store,
	}

	keepFiringForDuration := 30 * time.Minute
	// Initial run before prometheus goes down.
	rule := rules.NewAlertingRule(
		"HTTPRequestRateLow",
		expr,
		0,
		keepFiringForDuration,
		labels.FromStrings("severity", "critical"),
		labels.EmptyLabels(), labels.EmptyLabels(), "", true, nil,
	)

	group := rules.NewGroup(rules.GroupOptions{
		Name:          "default",
		Interval:      time.Second,
		Rules:         []rules.Rule{rule},
		ShouldRestore: true,
		Opts:          opts,
		AlertStore:    store,
	})
	groups := make(map[string]*rules.Group)
	groups["default;"] = group

	initialRuns := []time.Duration{0, 5 * time.Minute, 10 * time.Minute, 15 * time.Minute}

	baseTime := time.Unix(0, 0)
	for _, duration := range initialRuns {
		evalTime := baseTime.Add(duration)
		group.Eval(context.TODO(), evalTime)
	}

	exp := rule.ActiveAlerts()
	require.Len(t, exp, 1)
	for _, aa := range exp {
		require.NotEmpty(t, aa.KeepFiringSince)
	}
	sort.Slice(exp, func(i, j int) bool {
		return labels.Compare(exp[i].Labels, exp[j].Labels) < 0
	})

	// Prometheus goes down here. We create new rules and groups.
	type testInput struct {
		restoreDuration time.Duration
		expectedAlerts  []*rules.Alert
	}

	tests := []testInput{
		{
			// Normal restore.
			restoreDuration: 20 * time.Minute,
			expectedAlerts:  exp,
		},
		{
			// down for more than keep_firing_For (alerts should be resolved post restart).
			restoreDuration: 100 * time.Minute,
			expectedAlerts:  []*rules.Alert{},
		},
	}

	testFunc := func(tst testInput) {
		newRule := rules.NewAlertingRule(
			"HTTPRequestRateLow",
			expr,
			0,
			keepFiringForDuration,
			labels.FromStrings("severity", "critical"),
			labels.EmptyLabels(), labels.EmptyLabels(), "", false, nil,
		)
		newGroup := rules.NewGroup(rules.GroupOptions{
			Name:          "default",
			Interval:      time.Second,
			Rules:         []rules.Rule{newRule},
			ShouldRestore: true,
			Opts:          opts,
			AlertStore:    store,
		})

		newGroups := make(map[string]*rules.Group)
		newGroups["default;"] = newGroup

		restoreTime := baseTime.Add(tst.restoreDuration)

		// Restore happens here.
		newGroup.RestoreKeepFiringForState(restoreTime)
		// First eval after restoration.
		newGroup.Eval(context.TODO(), restoreTime)

		got := newRule.ActiveAlerts()

		sort.Slice(got, func(i, j int) bool {
			return labels.Compare(got[i].Labels, got[j].Labels) < 0
		})
		require.Equal(t, len(tst.expectedAlerts), len(got))

		if len(tst.expectedAlerts) > 0 {
			sortAlerts(tst.expectedAlerts)
			sortAlerts(got)

			for i, e := range exp {
				diff := float64(e.ActiveAt.Unix() - got[i].ActiveAt.Unix())
				require.Equal(t, 0.0, math.Abs(diff), "'keep_firing_for' state restored activeAt time is wrong")
				diff = float64(e.KeepFiringSince.Unix() - got[i].KeepFiringSince.Unix())
				require.Equal(t, 0.0, math.Abs(diff), "'keep_firing_for' state restored time is wrong")
			}
		}
	}

	for _, tst := range tests {
		testFunc(tst)
	}
}

// sortAlerts sorts `[]*Alert` w.r.t. the Labels.
func sortAlerts(items []*rules.Alert) {
	sort.Slice(items, func(i, j int) bool {
		return labels.Compare(items[i].Labels, items[j].Labels) <= 0
	})
}
