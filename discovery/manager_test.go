// Copyright 2016 The Prometheus Authors
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

package discovery

import (
	"context"
	"fmt"
	"sort"
	"strconv"
	"testing"
	"time"

	"github.com/go-kit/kit/log"
	client_testutil "github.com/prometheus/client_golang/prometheus/testutil"
	"github.com/prometheus/common/model"
	"github.com/stretchr/testify/require"

	"github.com/prometheus/prometheus/discovery/targetgroup"
	"github.com/prometheus/prometheus/util/testutil"
)

func TestMain(m *testing.M) {
	testutil.TolerantVerifyLeak(m)
}

// TestTargetUpdatesOrder checks that the target updates are received in the expected order.
func TestTargetUpdatesOrder(t *testing.T) {

	// The order by which the updates are send is determined by the interval passed to the mock discovery adapter
	// Final targets array is ordered alphabetically by the name of the discoverer.
	// For example discoverer "A" with targets "t2,t3" and discoverer "B" with targets "t1,t2" will result in "t2,t3,t1,t2" after the merge.
	testCases := []struct {
		title           string
		updates         map[string][]update
		expectedTargets [][]*targetgroup.Group
	}{
		{
			title: "Single TP no updates",
			updates: map[string][]update{
				"tp1": {},
			},
			expectedTargets: nil,
		},
		{
			title: "Multiple TPs no updates",
			updates: map[string][]update{
				"tp1": {},
				"tp2": {},
				"tp3": {},
			},
			expectedTargets: nil,
		},
		{
			title: "Single TP empty initials",
			updates: map[string][]update{
				"tp1": {
					{
						targetGroups: []targetgroup.Group{},
						interval:     5 * time.Millisecond,
					},
				},
			},
			expectedTargets: [][]*targetgroup.Group{
				{},
			},
		},
		{
			title: "Multiple TPs empty initials",
			updates: map[string][]update{
				"tp1": {
					{
						targetGroups: []targetgroup.Group{},
						interval:     5 * time.Millisecond,
					},
				},
				"tp2": {
					{
						targetGroups: []targetgroup.Group{},
						interval:     200 * time.Millisecond,
					},
				},
				"tp3": {
					{
						targetGroups: []targetgroup.Group{},
						interval:     100 * time.Millisecond,
					},
				},
			},
			expectedTargets: [][]*targetgroup.Group{
				{},
				{},
				{},
			},
		},
		{
			title: "Single TP initials only",
			updates: map[string][]update{
				"tp1": {
					{
						targetGroups: []targetgroup.Group{
							{
								Source:  "tp1_group1",
								Targets: []model.LabelSet{{"__instance__": "1"}},
							},
							{
								Source:  "tp1_group2",
								Targets: []model.LabelSet{{"__instance__": "2"}},
							}},
					},
				},
			},
			expectedTargets: [][]*targetgroup.Group{
				{
					{
						Source:  "tp1_group1",
						Targets: []model.LabelSet{{"__instance__": "1"}},
					},
					{
						Source:  "tp1_group2",
						Targets: []model.LabelSet{{"__instance__": "2"}},
					},
				},
			},
		},
		{
			title: "Multiple TPs initials only",
			updates: map[string][]update{
				"tp1": {
					{
						targetGroups: []targetgroup.Group{
							{
								Source:  "tp1_group1",
								Targets: []model.LabelSet{{"__instance__": "1"}},
							},
							{
								Source:  "tp1_group2",
								Targets: []model.LabelSet{{"__instance__": "2"}},
							},
						},
					},
				},
				"tp2": {
					{
						targetGroups: []targetgroup.Group{
							{
								Source:  "tp2_group1",
								Targets: []model.LabelSet{{"__instance__": "3"}},
							},
						},
						interval: 10 * time.Millisecond,
					},
				},
			},
			expectedTargets: [][]*targetgroup.Group{
				{
					{
						Source:  "tp1_group1",
						Targets: []model.LabelSet{{"__instance__": "1"}},
					},
					{
						Source:  "tp1_group2",
						Targets: []model.LabelSet{{"__instance__": "2"}},
					},
				}, {
					{
						Source:  "tp1_group1",
						Targets: []model.LabelSet{{"__instance__": "1"}},
					},
					{
						Source:  "tp1_group2",
						Targets: []model.LabelSet{{"__instance__": "2"}},
					},
					{
						Source:  "tp2_group1",
						Targets: []model.LabelSet{{"__instance__": "3"}},
					},
				},
			},
		},
		{
			title: "Single TP initials followed by empty updates",
			updates: map[string][]update{
				"tp1": {
					{
						targetGroups: []targetgroup.Group{
							{
								Source:  "tp1_group1",
								Targets: []model.LabelSet{{"__instance__": "1"}},
							},
							{
								Source:  "tp1_group2",
								Targets: []model.LabelSet{{"__instance__": "2"}},
							},
						},
						interval: 0,
					},
					{
						targetGroups: []targetgroup.Group{
							{
								Source:  "tp1_group1",
								Targets: []model.LabelSet{},
							},
							{
								Source:  "tp1_group2",
								Targets: []model.LabelSet{},
							},
						},
						interval: 10 * time.Millisecond,
					},
				},
			},
			expectedTargets: [][]*targetgroup.Group{
				{
					{
						Source:  "tp1_group1",
						Targets: []model.LabelSet{{"__instance__": "1"}},
					},
					{
						Source:  "tp1_group2",
						Targets: []model.LabelSet{{"__instance__": "2"}},
					},
				},
				{
					{
						Source:  "tp1_group1",
						Targets: []model.LabelSet{},
					},
					{
						Source:  "tp1_group2",
						Targets: []model.LabelSet{},
					},
				},
			},
		},
		{
			title: "Single TP initials and new groups",
			updates: map[string][]update{
				"tp1": {
					{
						targetGroups: []targetgroup.Group{
							{
								Source:  "tp1_group1",
								Targets: []model.LabelSet{{"__instance__": "1"}},
							},
							{
								Source:  "tp1_group2",
								Targets: []model.LabelSet{{"__instance__": "2"}},
							},
						},
						interval: 0,
					},
					{
						targetGroups: []targetgroup.Group{
							{
								Source:  "tp1_group1",
								Targets: []model.LabelSet{{"__instance__": "3"}},
							},
							{
								Source:  "tp1_group2",
								Targets: []model.LabelSet{{"__instance__": "4"}},
							},
							{
								Source:  "tp1_group3",
								Targets: []model.LabelSet{{"__instance__": "1"}},
							},
						},
						interval: 10 * time.Millisecond,
					},
				},
			},
			expectedTargets: [][]*targetgroup.Group{
				{
					{
						Source:  "tp1_group1",
						Targets: []model.LabelSet{{"__instance__": "1"}},
					},
					{
						Source:  "tp1_group2",
						Targets: []model.LabelSet{{"__instance__": "2"}},
					},
				},
				{
					{
						Source:  "tp1_group1",
						Targets: []model.LabelSet{{"__instance__": "3"}},
					},
					{
						Source:  "tp1_group2",
						Targets: []model.LabelSet{{"__instance__": "4"}},
					},
					{
						Source:  "tp1_group3",
						Targets: []model.LabelSet{{"__instance__": "1"}},
					},
				},
			},
		},
		{
			title: "Multiple TPs initials and new groups",
			updates: map[string][]update{
				"tp1": {
					{
						targetGroups: []targetgroup.Group{
							{
								Source:  "tp1_group1",
								Targets: []model.LabelSet{{"__instance__": "1"}},
							},
							{
								Source:  "tp1_group2",
								Targets: []model.LabelSet{{"__instance__": "2"}},
							},
						},
						interval: 10 * time.Millisecond,
					},
					{
						targetGroups: []targetgroup.Group{
							{
								Source:  "tp1_group3",
								Targets: []model.LabelSet{{"__instance__": "3"}},
							},
							{
								Source:  "tp1_group4",
								Targets: []model.LabelSet{{"__instance__": "4"}},
							},
						},
						interval: 500 * time.Millisecond,
					},
				},
				"tp2": {
					{
						targetGroups: []targetgroup.Group{
							{
								Source:  "tp2_group1",
								Targets: []model.LabelSet{{"__instance__": "5"}},
							},
							{
								Source:  "tp2_group2",
								Targets: []model.LabelSet{{"__instance__": "6"}},
							},
						},
						interval: 100 * time.Millisecond,
					},
					{
						targetGroups: []targetgroup.Group{
							{
								Source:  "tp2_group3",
								Targets: []model.LabelSet{{"__instance__": "7"}},
							},
							{
								Source:  "tp2_group4",
								Targets: []model.LabelSet{{"__instance__": "8"}},
							},
						},
						interval: 10 * time.Millisecond,
					},
				},
			},
			expectedTargets: [][]*targetgroup.Group{
				{
					{
						Source:  "tp1_group1",
						Targets: []model.LabelSet{{"__instance__": "1"}},
					},
					{
						Source:  "tp1_group2",
						Targets: []model.LabelSet{{"__instance__": "2"}},
					},
				},
				{
					{
						Source:  "tp1_group1",
						Targets: []model.LabelSet{{"__instance__": "1"}},
					},
					{
						Source:  "tp1_group2",
						Targets: []model.LabelSet{{"__instance__": "2"}},
					},
					{
						Source:  "tp2_group1",
						Targets: []model.LabelSet{{"__instance__": "5"}},
					},
					{
						Source:  "tp2_group2",
						Targets: []model.LabelSet{{"__instance__": "6"}},
					},
				},
				{
					{
						Source:  "tp1_group1",
						Targets: []model.LabelSet{{"__instance__": "1"}},
					},
					{
						Source:  "tp1_group2",
						Targets: []model.LabelSet{{"__instance__": "2"}},
					},
					{
						Source:  "tp2_group1",
						Targets: []model.LabelSet{{"__instance__": "5"}},
					},
					{
						Source:  "tp2_group2",
						Targets: []model.LabelSet{{"__instance__": "6"}},
					},
					{
						Source:  "tp2_group3",
						Targets: []model.LabelSet{{"__instance__": "7"}},
					},
					{
						Source:  "tp2_group4",
						Targets: []model.LabelSet{{"__instance__": "8"}},
					},
				},
				{
					{
						Source:  "tp1_group1",
						Targets: []model.LabelSet{{"__instance__": "1"}},
					},
					{
						Source:  "tp1_group2",
						Targets: []model.LabelSet{{"__instance__": "2"}},
					},
					{
						Source:  "tp1_group3",
						Targets: []model.LabelSet{{"__instance__": "3"}},
					},
					{
						Source:  "tp1_group4",
						Targets: []model.LabelSet{{"__instance__": "4"}},
					},
					{
						Source:  "tp2_group1",
						Targets: []model.LabelSet{{"__instance__": "5"}},
					},
					{
						Source:  "tp2_group2",
						Targets: []model.LabelSet{{"__instance__": "6"}},
					},
					{
						Source:  "tp2_group3",
						Targets: []model.LabelSet{{"__instance__": "7"}},
					},
					{
						Source:  "tp2_group4",
						Targets: []model.LabelSet{{"__instance__": "8"}},
					},
				},
			},
		},
		{
			title: "One TP initials arrive after other TP updates.",
			updates: map[string][]update{
				"tp1": {
					{
						targetGroups: []targetgroup.Group{
							{
								Source:  "tp1_group1",
								Targets: []model.LabelSet{{"__instance__": "1"}},
							},
							{
								Source:  "tp1_group2",
								Targets: []model.LabelSet{{"__instance__": "2"}},
							},
						},
						interval: 10 * time.Millisecond,
					},
					{
						targetGroups: []targetgroup.Group{
							{
								Source:  "tp1_group1",
								Targets: []model.LabelSet{{"__instance__": "3"}},
							},
							{
								Source:  "tp1_group2",
								Targets: []model.LabelSet{{"__instance__": "4"}},
							},
						},
						interval: 150 * time.Millisecond,
					},
				},
				"tp2": {
					{
						targetGroups: []targetgroup.Group{
							{
								Source:  "tp2_group1",
								Targets: []model.LabelSet{{"__instance__": "5"}},
							},
							{
								Source:  "tp2_group2",
								Targets: []model.LabelSet{{"__instance__": "6"}},
							},
						},
						interval: 200 * time.Millisecond,
					},
					{
						targetGroups: []targetgroup.Group{
							{
								Source:  "tp2_group1",
								Targets: []model.LabelSet{{"__instance__": "7"}},
							},
							{
								Source:  "tp2_group2",
								Targets: []model.LabelSet{{"__instance__": "8"}},
							},
						},
						interval: 100 * time.Millisecond,
					},
				},
			},
			expectedTargets: [][]*targetgroup.Group{
				{
					{
						Source:  "tp1_group1",
						Targets: []model.LabelSet{{"__instance__": "1"}},
					},
					{
						Source:  "tp1_group2",
						Targets: []model.LabelSet{{"__instance__": "2"}},
					},
				},
				{
					{
						Source:  "tp1_group1",
						Targets: []model.LabelSet{{"__instance__": "3"}},
					},
					{
						Source:  "tp1_group2",
						Targets: []model.LabelSet{{"__instance__": "4"}},
					},
				},
				{
					{
						Source:  "tp1_group1",
						Targets: []model.LabelSet{{"__instance__": "3"}},
					},
					{
						Source:  "tp1_group2",
						Targets: []model.LabelSet{{"__instance__": "4"}},
					},
					{
						Source:  "tp2_group1",
						Targets: []model.LabelSet{{"__instance__": "5"}},
					},
					{
						Source:  "tp2_group2",
						Targets: []model.LabelSet{{"__instance__": "6"}},
					},
				},
				{
					{
						Source:  "tp1_group1",
						Targets: []model.LabelSet{{"__instance__": "3"}},
					},
					{
						Source:  "tp1_group2",
						Targets: []model.LabelSet{{"__instance__": "4"}},
					},
					{
						Source:  "tp2_group1",
						Targets: []model.LabelSet{{"__instance__": "7"}},
					},
					{
						Source:  "tp2_group2",
						Targets: []model.LabelSet{{"__instance__": "8"}},
					},
				},
			},
		},

		{
			title: "Single TP empty update in between",
			updates: map[string][]update{
				"tp1": {
					{
						targetGroups: []targetgroup.Group{
							{
								Source:  "tp1_group1",
								Targets: []model.LabelSet{{"__instance__": "1"}},
							},
							{
								Source:  "tp1_group2",
								Targets: []model.LabelSet{{"__instance__": "2"}},
							},
						},
						interval: 30 * time.Millisecond,
					},
					{
						targetGroups: []targetgroup.Group{
							{
								Source:  "tp1_group1",
								Targets: []model.LabelSet{},
							},
							{
								Source:  "tp1_group2",
								Targets: []model.LabelSet{},
							},
						},
						interval: 10 * time.Millisecond,
					},
					{
						targetGroups: []targetgroup.Group{
							{
								Source:  "tp1_group1",
								Targets: []model.LabelSet{{"__instance__": "3"}},
							},
							{
								Source:  "tp1_group2",
								Targets: []model.LabelSet{{"__instance__": "4"}},
							},
						},
						interval: 300 * time.Millisecond,
					},
				},
			},
			expectedTargets: [][]*targetgroup.Group{
				{
					{
						Source:  "tp1_group1",
						Targets: []model.LabelSet{{"__instance__": "1"}},
					},
					{
						Source:  "tp1_group2",
						Targets: []model.LabelSet{{"__instance__": "2"}},
					},
				},
				{
					{
						Source:  "tp1_group1",
						Targets: []model.LabelSet{},
					},
					{
						Source:  "tp1_group2",
						Targets: []model.LabelSet{},
					},
				},
				{
					{
						Source:  "tp1_group1",
						Targets: []model.LabelSet{{"__instance__": "3"}},
					},
					{
						Source:  "tp1_group2",
						Targets: []model.LabelSet{{"__instance__": "4"}},
					},
				},
			},
		},
	}

	for i, tc := range testCases {
		tc := tc
		t.Run(tc.title, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()

			discoveryManager := NewManager(ctx, log.NewNopLogger())
			discoveryManager.updatert = 100 * time.Millisecond

			var totalUpdatesCount int
			provUpdates := make(chan []*targetgroup.Group)
			for _, up := range tc.updates {
				go newMockDiscoveryProvider(up...).Run(ctx, provUpdates)
				if len(up) > 0 {
					totalUpdatesCount += len(up)
				}
			}

			for x := 0; x < totalUpdatesCount; x++ {
				select {
				case <-ctx.Done():
					t.Fatalf("%d: no update arrived within the timeout limit", x)
				case tgs := <-provUpdates:
					discoveryManager.updateGroup(poolKey{setName: strconv.Itoa(i), provider: tc.title}, tgs)
					for _, got := range discoveryManager.allGroups() {
						assertEqualGroups(t, got, tc.expectedTargets[x], func(got, expected string) string {
							return fmt.Sprintf("%d: \ntargets mismatch \ngot: %v \nexpected: %v",
								x,
								got,
								expected)
						})
					}
				}
			}
		})
	}
}

func assertEqualGroups(t *testing.T, got, expected []*targetgroup.Group, msg func(got, expected string) string) {
	t.Helper()

	// Need to sort by the groups's source as the received order is not guaranteed.
	sort.Sort(byGroupSource(got))
	sort.Sort(byGroupSource(expected))

	require.Equal(t, expected, got)
}

func staticConfig(addrs ...string) StaticConfig {
	var cfg StaticConfig
	for i, addr := range addrs {
		cfg = append(cfg, &targetgroup.Group{
			Source: fmt.Sprint(i),
			Targets: []model.LabelSet{
				{model.AddressLabel: model.LabelValue(addr)},
			},
		})
	}
	return cfg
}

func verifyPresence(t *testing.T, tSets map[poolKey]map[string]*targetgroup.Group, poolKey poolKey, label string, present bool) {
	t.Helper()
	if _, ok := tSets[poolKey]; !ok {
		t.Fatalf("'%s' should be present in Pool keys: %v", poolKey, tSets)
		return
	}

	match := false
	var mergedTargets string
	for _, targetGroup := range tSets[poolKey] {

		for _, l := range targetGroup.Targets {
			mergedTargets = mergedTargets + " " + l.String()
			if l.String() == label {
				match = true
			}
		}

	}
	if match != present {
		msg := ""
		if !present {
			msg = "not"
		}
		t.Fatalf("%q should %s be present in Targets labels: %q", label, msg, mergedTargets)
	}
}

func TestTargetSetRecreatesTargetGroupsEveryRun(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	discoveryManager := NewManager(ctx, log.NewNopLogger())
	discoveryManager.updatert = 100 * time.Millisecond
	go discoveryManager.Run()

	c := map[string]Configs{
		"prometheus": {
			staticConfig("foo:9090", "bar:9090"),
		},
	}
	discoveryManager.ApplyConfig(c)

	<-discoveryManager.SyncCh()
	verifyPresence(t, discoveryManager.targets, poolKey{setName: "prometheus", provider: "static/0"}, "{__address__=\"foo:9090\"}", true)
	verifyPresence(t, discoveryManager.targets, poolKey{setName: "prometheus", provider: "static/0"}, "{__address__=\"bar:9090\"}", true)

	c["prometheus"] = Configs{
		staticConfig("foo:9090"),
	}
	discoveryManager.ApplyConfig(c)

	<-discoveryManager.SyncCh()
	verifyPresence(t, discoveryManager.targets, poolKey{setName: "prometheus", provider: "static/0"}, "{__address__=\"foo:9090\"}", true)
	verifyPresence(t, discoveryManager.targets, poolKey{setName: "prometheus", provider: "static/0"}, "{__address__=\"bar:9090\"}", false)
}

func TestDiscovererConfigs(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	discoveryManager := NewManager(ctx, log.NewNopLogger())
	discoveryManager.updatert = 100 * time.Millisecond
	go discoveryManager.Run()

	c := map[string]Configs{
		"prometheus": {
			staticConfig("foo:9090", "bar:9090"),
			staticConfig("baz:9090"),
		},
	}
	discoveryManager.ApplyConfig(c)

	<-discoveryManager.SyncCh()
	verifyPresence(t, discoveryManager.targets, poolKey{setName: "prometheus", provider: "static/0"}, "{__address__=\"foo:9090\"}", true)
	verifyPresence(t, discoveryManager.targets, poolKey{setName: "prometheus", provider: "static/0"}, "{__address__=\"bar:9090\"}", true)
	verifyPresence(t, discoveryManager.targets, poolKey{setName: "prometheus", provider: "static/1"}, "{__address__=\"baz:9090\"}", true)
}

// TestTargetSetRecreatesEmptyStaticConfigs ensures that reloading a config file after
// removing all targets from the static_configs sends an update with empty targetGroups.
// This is required to signal the receiver that this target set has no current targets.
func TestTargetSetRecreatesEmptyStaticConfigs(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	discoveryManager := NewManager(ctx, log.NewNopLogger())
	discoveryManager.updatert = 100 * time.Millisecond
	go discoveryManager.Run()

	c := map[string]Configs{
		"prometheus": {
			staticConfig("foo:9090"),
		},
	}
	discoveryManager.ApplyConfig(c)

	<-discoveryManager.SyncCh()
	verifyPresence(t, discoveryManager.targets, poolKey{setName: "prometheus", provider: "static/0"}, "{__address__=\"foo:9090\"}", true)

	c["prometheus"] = Configs{
		StaticConfig{{}},
	}
	discoveryManager.ApplyConfig(c)

	<-discoveryManager.SyncCh()

	pkey := poolKey{setName: "prometheus", provider: "static/0"}
	targetGroups, ok := discoveryManager.targets[pkey]
	if !ok {
		t.Fatalf("'%v' should be present in target groups", pkey)
	}
	group, ok := targetGroups[""]
	if !ok {
		t.Fatalf("missing '' key in target groups %v", targetGroups)
	}

	if len(group.Targets) != 0 {
		t.Fatalf("Invalid number of targets: expected 0, got %d", len(group.Targets))
	}
}

func TestIdenticalConfigurationsAreCoalesced(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	discoveryManager := NewManager(ctx, nil)
	discoveryManager.updatert = 100 * time.Millisecond
	go discoveryManager.Run()

	c := map[string]Configs{
		"prometheus": {
			staticConfig("foo:9090"),
		},
		"prometheus2": {
			staticConfig("foo:9090"),
		},
	}
	discoveryManager.ApplyConfig(c)

	<-discoveryManager.SyncCh()
	verifyPresence(t, discoveryManager.targets, poolKey{setName: "prometheus", provider: "static/0"}, "{__address__=\"foo:9090\"}", true)
	verifyPresence(t, discoveryManager.targets, poolKey{setName: "prometheus2", provider: "static/0"}, "{__address__=\"foo:9090\"}", true)
	if len(discoveryManager.providers) != 1 {
		t.Fatalf("Invalid number of providers: expected 1, got %d", len(discoveryManager.providers))
	}
}

func TestApplyConfigDoesNotModifyStaticTargets(t *testing.T) {
	originalConfig := Configs{
		staticConfig("foo:9090", "bar:9090", "baz:9090"),
	}
	processedConfig := Configs{
		staticConfig("foo:9090", "bar:9090", "baz:9090"),
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	discoveryManager := NewManager(ctx, log.NewNopLogger())
	discoveryManager.updatert = 100 * time.Millisecond
	go discoveryManager.Run()

	cfgs := map[string]Configs{
		"prometheus": processedConfig,
	}
	discoveryManager.ApplyConfig(cfgs)
	<-discoveryManager.SyncCh()

	for _, cfg := range cfgs {
		require.Equal(t, originalConfig, cfg)
	}
}

type errorConfig struct{ err error }

func (e errorConfig) Name() string                                        { return "error" }
func (e errorConfig) NewDiscoverer(DiscovererOptions) (Discoverer, error) { return nil, e.err }

func TestGaugeFailedConfigs(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	discoveryManager := NewManager(ctx, log.NewNopLogger())
	discoveryManager.updatert = 100 * time.Millisecond
	go discoveryManager.Run()

	c := map[string]Configs{
		"prometheus": {
			errorConfig{fmt.Errorf("tests error 0")},
			errorConfig{fmt.Errorf("tests error 1")},
			errorConfig{fmt.Errorf("tests error 2")},
		},
	}
	discoveryManager.ApplyConfig(c)
	<-discoveryManager.SyncCh()

	failedCount := client_testutil.ToFloat64(failedConfigs)
	if failedCount != 3 {
		t.Fatalf("Expected to have 3 failed configs, got: %v", failedCount)
	}

	c["prometheus"] = Configs{
		staticConfig("foo:9090"),
	}
	discoveryManager.ApplyConfig(c)
	<-discoveryManager.SyncCh()

	failedCount = client_testutil.ToFloat64(failedConfigs)
	if failedCount != 0 {
		t.Fatalf("Expected to get no failed config, got: %v", failedCount)
	}

}

func TestCoordinationWithReceiver(t *testing.T) {
	updateDelay := 100 * time.Millisecond

	type expect struct {
		delay time.Duration
		tgs   map[string][]*targetgroup.Group
	}

	testCases := []struct {
		title     string
		providers map[string]Discoverer
		expected  []expect
	}{
		{
			title: "Receiver should get all updates even when one provider closes its channel",
			providers: map[string]Discoverer{
				"once1": &onceProvider{
					tgs: []*targetgroup.Group{
						{
							Source:  "tg1",
							Targets: []model.LabelSet{{"__instance__": "1"}},
						},
					},
				},
				"mock1": newMockDiscoveryProvider(
					update{
						interval: 2 * updateDelay,
						targetGroups: []targetgroup.Group{
							{
								Source:  "tg2",
								Targets: []model.LabelSet{{"__instance__": "2"}},
							},
						},
					},
				),
			},
			expected: []expect{
				{
					tgs: map[string][]*targetgroup.Group{
						"once1": {
							{
								Source:  "tg1",
								Targets: []model.LabelSet{{"__instance__": "1"}},
							},
						},
					},
				},
				{
					tgs: map[string][]*targetgroup.Group{
						"once1": {
							{
								Source:  "tg1",
								Targets: []model.LabelSet{{"__instance__": "1"}},
							},
						},
						"mock1": {
							{
								Source:  "tg2",
								Targets: []model.LabelSet{{"__instance__": "2"}},
							},
						},
					},
				},
			},
		},
		{
			title: "Receiver should get all updates even when the channel is blocked",
			providers: map[string]Discoverer{
				"mock1": newMockDiscoveryProvider(
					update{
						targetGroups: []targetgroup.Group{
							{
								Source:  "tg1",
								Targets: []model.LabelSet{{"__instance__": "1"}},
							},
						},
					},
					update{
						interval: 4 * updateDelay,
						targetGroups: []targetgroup.Group{
							{
								Source:  "tg2",
								Targets: []model.LabelSet{{"__instance__": "2"}},
							},
						},
					},
				),
			},
			expected: []expect{
				{
					delay: 2 * updateDelay,
					tgs: map[string][]*targetgroup.Group{
						"mock1": {
							{
								Source:  "tg1",
								Targets: []model.LabelSet{{"__instance__": "1"}},
							},
						},
					},
				},
				{
					delay: 4 * updateDelay,
					tgs: map[string][]*targetgroup.Group{
						"mock1": {
							{
								Source:  "tg1",
								Targets: []model.LabelSet{{"__instance__": "1"}},
							},
							{
								Source:  "tg2",
								Targets: []model.LabelSet{{"__instance__": "2"}},
							},
						},
					},
				},
			},
		},
	}

	for _, tc := range testCases {
		tc := tc
		t.Run(tc.title, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()

			mgr := NewManager(ctx, nil)
			mgr.updatert = updateDelay
			go mgr.Run()

			for name, p := range tc.providers {
				mgr.StartCustomProvider(ctx, name, p)
			}

			for i, expected := range tc.expected {
				time.Sleep(expected.delay)
				select {
				case <-ctx.Done():
					t.Fatalf("step %d: no update received in the expected timeframe", i)
				case tgs, ok := <-mgr.SyncCh():
					if !ok {
						t.Fatalf("step %d: discovery manager channel is closed", i)
					}
					if len(tgs) != len(expected.tgs) {
						t.Fatalf("step %d: target groups mismatch, got: %d, expected: %d\ngot: %#v\nexpected: %#v",
							i, len(tgs), len(expected.tgs), tgs, expected.tgs)
					}
					for k := range expected.tgs {
						if _, ok := tgs[k]; !ok {
							t.Fatalf("step %d: target group not found: %s\ngot: %#v", i, k, tgs)
						}
						assertEqualGroups(t, tgs[k], expected.tgs[k], func(got, expected string) string {
							return fmt.Sprintf("step %d: targets mismatch \ngot: %q \nexpected: %q", i, got, expected)
						})
					}
				}
			}
		})
	}
}

type update struct {
	targetGroups []targetgroup.Group
	interval     time.Duration
}

type mockdiscoveryProvider struct {
	updates []update
}

func newMockDiscoveryProvider(updates ...update) mockdiscoveryProvider {
	tp := mockdiscoveryProvider{
		updates: updates,
	}
	return tp
}

func (tp mockdiscoveryProvider) Run(ctx context.Context, upCh chan<- []*targetgroup.Group) {
	for _, u := range tp.updates {
		if u.interval > 0 {
			select {
			case <-ctx.Done():
				return
			case <-time.After(u.interval):
			}
		}
		tgs := make([]*targetgroup.Group, len(u.targetGroups))
		for i := range u.targetGroups {
			tgs[i] = &u.targetGroups[i]
		}
		upCh <- tgs
	}
	<-ctx.Done()
}

// byGroupSource implements sort.Interface so we can sort by the Source field.
type byGroupSource []*targetgroup.Group

func (a byGroupSource) Len() int           { return len(a) }
func (a byGroupSource) Swap(i, j int)      { a[i], a[j] = a[j], a[i] }
func (a byGroupSource) Less(i, j int) bool { return a[i].Source < a[j].Source }

// onceProvider sends updates once (if any) and closes the update channel.
type onceProvider struct {
	tgs []*targetgroup.Group
}

func (o onceProvider) Run(_ context.Context, ch chan<- []*targetgroup.Group) {
	if len(o.tgs) > 0 {
		ch <- o.tgs
	}
	close(ch)
}
