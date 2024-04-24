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
	"sync"
	"testing"
	"time"

	"github.com/go-kit/log"
	"github.com/prometheus/client_golang/prometheus"
	client_testutil "github.com/prometheus/client_golang/prometheus/testutil"
	"github.com/prometheus/common/model"
	"github.com/stretchr/testify/require"

	"github.com/prometheus/prometheus/discovery/targetgroup"
	"github.com/prometheus/prometheus/util/testutil"
)

func TestMain(m *testing.M) {
	testutil.TolerantVerifyLeak(m)
}

func NewTestMetrics(t *testing.T, reg prometheus.Registerer) (RefreshMetricsManager, map[string]DiscovererMetrics) {
	refreshMetrics := NewRefreshMetrics(reg)
	sdMetrics, err := RegisterSDMetrics(reg, refreshMetrics)
	require.NoError(t, err)
	return refreshMetrics, sdMetrics
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
							},
						},
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

			reg := prometheus.NewRegistry()
			_, sdMetrics := NewTestMetrics(t, reg)

			discoveryManager := NewManager(ctx, log.NewNopLogger(), reg, sdMetrics)
			require.NotNil(t, discoveryManager)
			discoveryManager.updatert = 100 * time.Millisecond

			var totalUpdatesCount int
			for _, up := range tc.updates {
				if len(up) > 0 {
					totalUpdatesCount += len(up)
				}
			}
			provUpdates := make(chan []*targetgroup.Group, totalUpdatesCount)

			for _, up := range tc.updates {
				go newMockDiscoveryProvider(up...).Run(ctx, provUpdates)
			}

			for x := 0; x < totalUpdatesCount; x++ {
				select {
				case <-ctx.Done():
					require.FailNow(t, "%d: no update arrived within the timeout limit", x)
				case tgs := <-provUpdates:
					discoveryManager.updateGroup(poolKey{setName: strconv.Itoa(i), provider: tc.title}, tgs)
					for _, got := range discoveryManager.allGroups() {
						assertEqualGroups(t, got, tc.expectedTargets[x])
					}
				}
			}
		})
	}
}

func assertEqualGroups(t *testing.T, got, expected []*targetgroup.Group) {
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

func verifySyncedPresence(t *testing.T, tGroups map[string][]*targetgroup.Group, key, label string, present bool) {
	t.Helper()
	if _, ok := tGroups[key]; !ok {
		t.Fatalf("'%s' should be present in Group map keys: %v", key, tGroups)
	}
	match := false
	var mergedTargets string
	for _, targetGroups := range tGroups[key] {
		for _, l := range targetGroups.Targets {
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
		t.Fatalf("%q should %s be present in Group labels: %q", label, msg, mergedTargets)
	}
}

func verifyPresence(t *testing.T, tSets map[poolKey]map[string]*targetgroup.Group, poolKey poolKey, label string, present bool) {
	t.Helper()
	_, ok := tSets[poolKey]
	require.True(t, ok, "'%s' should be present in Pool keys: %v", poolKey, tSets)

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
		require.FailNow(t, "%q should %s be present in Targets labels: %q", label, msg, mergedTargets)
	}
}

func pk(provider, setName string, n int) poolKey {
	return poolKey{
		setName:  setName,
		provider: fmt.Sprintf("%s/%d", provider, n),
	}
}

func TestTargetSetTargetGroupsPresentOnConfigReload(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	reg := prometheus.NewRegistry()
	_, sdMetrics := NewTestMetrics(t, reg)

	discoveryManager := NewManager(ctx, log.NewNopLogger(), reg, sdMetrics)
	require.NotNil(t, discoveryManager)
	discoveryManager.updatert = 100 * time.Millisecond
	go discoveryManager.Run()

	c := map[string]Configs{
		"prometheus": {
			staticConfig("foo:9090"),
		},
	}
	discoveryManager.ApplyConfig(c)

	syncedTargets := <-discoveryManager.SyncCh()
	require.Len(t, syncedTargets, 1)
	verifySyncedPresence(t, syncedTargets, "prometheus", "{__address__=\"foo:9090\"}", true)
	require.Len(t, syncedTargets["prometheus"], 1)
	p := pk("static", "prometheus", 0)
	verifyPresence(t, discoveryManager.targets, p, "{__address__=\"foo:9090\"}", true)
	require.Len(t, discoveryManager.targets, 1)

	discoveryManager.ApplyConfig(c)

	syncedTargets = <-discoveryManager.SyncCh()
	verifyPresence(t, discoveryManager.targets, p, "{__address__=\"foo:9090\"}", true)
	require.Len(t, discoveryManager.targets, 1)
	require.Len(t, syncedTargets, 1)
	verifySyncedPresence(t, syncedTargets, "prometheus", "{__address__=\"foo:9090\"}", true)
	require.Len(t, syncedTargets["prometheus"], 1)
}

func TestTargetSetTargetGroupsPresentOnConfigRename(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	reg := prometheus.NewRegistry()
	_, sdMetrics := NewTestMetrics(t, reg)

	discoveryManager := NewManager(ctx, log.NewNopLogger(), reg, sdMetrics)
	require.NotNil(t, discoveryManager)
	discoveryManager.updatert = 100 * time.Millisecond
	go discoveryManager.Run()

	c := map[string]Configs{
		"prometheus": {
			staticConfig("foo:9090"),
		},
	}
	discoveryManager.ApplyConfig(c)

	syncedTargets := <-discoveryManager.SyncCh()
	require.Len(t, syncedTargets, 1)
	verifySyncedPresence(t, syncedTargets, "prometheus", "{__address__=\"foo:9090\"}", true)
	require.Len(t, syncedTargets["prometheus"], 1)
	p := pk("static", "prometheus", 0)
	verifyPresence(t, discoveryManager.targets, p, "{__address__=\"foo:9090\"}", true)
	require.Len(t, discoveryManager.targets, 1)

	c["prometheus2"] = c["prometheus"]
	delete(c, "prometheus")
	discoveryManager.ApplyConfig(c)

	syncedTargets = <-discoveryManager.SyncCh()
	p = pk("static", "prometheus2", 0)
	verifyPresence(t, discoveryManager.targets, p, "{__address__=\"foo:9090\"}", true)
	require.Len(t, discoveryManager.targets, 1)
	require.Len(t, syncedTargets, 1)
	verifySyncedPresence(t, syncedTargets, "prometheus2", "{__address__=\"foo:9090\"}", true)
	require.Len(t, syncedTargets["prometheus2"], 1)
}

func TestTargetSetTargetGroupsPresentOnConfigDuplicateAndDeleteOriginal(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	reg := prometheus.NewRegistry()
	_, sdMetrics := NewTestMetrics(t, reg)

	discoveryManager := NewManager(ctx, log.NewNopLogger(), reg, sdMetrics)
	require.NotNil(t, discoveryManager)
	discoveryManager.updatert = 100 * time.Millisecond
	go discoveryManager.Run()

	c := map[string]Configs{
		"prometheus": {
			staticConfig("foo:9090"),
		},
	}
	discoveryManager.ApplyConfig(c)
	<-discoveryManager.SyncCh()

	c["prometheus2"] = c["prometheus"]
	discoveryManager.ApplyConfig(c)
	syncedTargets := <-discoveryManager.SyncCh()
	require.Len(t, syncedTargets, 2)
	verifySyncedPresence(t, syncedTargets, "prometheus", "{__address__=\"foo:9090\"}", true)
	require.Len(t, syncedTargets["prometheus"], 1)
	verifySyncedPresence(t, syncedTargets, "prometheus2", "{__address__=\"foo:9090\"}", true)
	require.Len(t, syncedTargets["prometheus2"], 1)
	p := pk("static", "prometheus", 0)
	verifyPresence(t, discoveryManager.targets, p, "{__address__=\"foo:9090\"}", true)
	require.Len(t, discoveryManager.targets, 2)

	delete(c, "prometheus")
	discoveryManager.ApplyConfig(c)
	syncedTargets = <-discoveryManager.SyncCh()
	p = pk("static", "prometheus2", 0)
	verifyPresence(t, discoveryManager.targets, p, "{__address__=\"foo:9090\"}", true)
	require.Len(t, discoveryManager.targets, 1)
	require.Len(t, syncedTargets, 1)
	verifySyncedPresence(t, syncedTargets, "prometheus2", "{__address__=\"foo:9090\"}", true)
	require.Len(t, syncedTargets["prometheus2"], 1)
}

func TestTargetSetTargetGroupsPresentOnConfigChange(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	reg := prometheus.NewRegistry()
	_, sdMetrics := NewTestMetrics(t, reg)

	discoveryManager := NewManager(ctx, log.NewNopLogger(), reg, sdMetrics)
	require.NotNil(t, discoveryManager)
	discoveryManager.updatert = 100 * time.Millisecond
	go discoveryManager.Run()

	c := map[string]Configs{
		"prometheus": {
			staticConfig("foo:9090"),
		},
	}
	discoveryManager.ApplyConfig(c)

	syncedTargets := <-discoveryManager.SyncCh()
	require.Len(t, syncedTargets, 1)
	verifySyncedPresence(t, syncedTargets, "prometheus", "{__address__=\"foo:9090\"}", true)
	require.Len(t, syncedTargets["prometheus"], 1)

	var mu sync.Mutex
	c["prometheus2"] = Configs{
		lockStaticConfig{
			mu:     &mu,
			config: staticConfig("bar:9090"),
		},
	}
	mu.Lock()
	discoveryManager.ApplyConfig(c)

	// Original targets should be present as soon as possible.
	syncedTargets = <-discoveryManager.SyncCh()
	mu.Unlock()
	require.Len(t, syncedTargets, 1)
	verifySyncedPresence(t, syncedTargets, "prometheus", "{__address__=\"foo:9090\"}", true)
	require.Len(t, syncedTargets["prometheus"], 1)

	// prometheus2 configs should be ready on second sync.
	syncedTargets = <-discoveryManager.SyncCh()
	require.Len(t, syncedTargets, 2)
	verifySyncedPresence(t, syncedTargets, "prometheus", "{__address__=\"foo:9090\"}", true)
	require.Len(t, syncedTargets["prometheus"], 1)
	verifySyncedPresence(t, syncedTargets, "prometheus2", "{__address__=\"bar:9090\"}", true)
	require.Len(t, syncedTargets["prometheus2"], 1)

	p := pk("static", "prometheus", 0)
	verifyPresence(t, discoveryManager.targets, p, "{__address__=\"foo:9090\"}", true)
	p = pk("lockstatic", "prometheus2", 1)
	verifyPresence(t, discoveryManager.targets, p, "{__address__=\"bar:9090\"}", true)
	require.Len(t, discoveryManager.targets, 2)

	// Delete part of config and ensure only original targets exist.
	delete(c, "prometheus2")
	discoveryManager.ApplyConfig(c)
	syncedTargets = <-discoveryManager.SyncCh()
	require.Len(t, discoveryManager.targets, 1)
	verifyPresence(t, discoveryManager.targets, pk("static", "prometheus", 0), "{__address__=\"foo:9090\"}", true)
	require.Len(t, syncedTargets, 1)
	verifySyncedPresence(t, syncedTargets, "prometheus", "{__address__=\"foo:9090\"}", true)
	require.Len(t, syncedTargets["prometheus"], 1)
}

func TestTargetSetRecreatesTargetGroupsOnConfigChange(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	reg := prometheus.NewRegistry()
	_, sdMetrics := NewTestMetrics(t, reg)

	discoveryManager := NewManager(ctx, log.NewNopLogger(), reg, sdMetrics)
	require.NotNil(t, discoveryManager)
	discoveryManager.updatert = 100 * time.Millisecond
	go discoveryManager.Run()

	c := map[string]Configs{
		"prometheus": {
			staticConfig("foo:9090", "bar:9090"),
		},
	}
	discoveryManager.ApplyConfig(c)

	syncedTargets := <-discoveryManager.SyncCh()
	p := pk("static", "prometheus", 0)
	verifyPresence(t, discoveryManager.targets, p, "{__address__=\"foo:9090\"}", true)
	verifyPresence(t, discoveryManager.targets, p, "{__address__=\"bar:9090\"}", true)
	require.Len(t, discoveryManager.targets, 1)
	require.Len(t, syncedTargets, 1)
	verifySyncedPresence(t, syncedTargets, "prometheus", "{__address__=\"foo:9090\"}", true)
	verifySyncedPresence(t, syncedTargets, "prometheus", "{__address__=\"bar:9090\"}", true)
	require.Len(t, syncedTargets["prometheus"], 2)

	c["prometheus"] = Configs{
		staticConfig("foo:9090"),
	}
	discoveryManager.ApplyConfig(c)
	syncedTargets = <-discoveryManager.SyncCh()
	require.Len(t, discoveryManager.targets, 1)
	p = pk("static", "prometheus", 1)
	verifyPresence(t, discoveryManager.targets, p, "{__address__=\"foo:9090\"}", true)
	verifyPresence(t, discoveryManager.targets, p, "{__address__=\"bar:9090\"}", false)
	require.Len(t, discoveryManager.targets, 1)
	require.Len(t, syncedTargets, 1)
	verifySyncedPresence(t, syncedTargets, "prometheus", "{__address__=\"foo:9090\"}", true)
	require.Len(t, syncedTargets["prometheus"], 1)
}

func TestDiscovererConfigs(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	reg := prometheus.NewRegistry()
	_, sdMetrics := NewTestMetrics(t, reg)

	discoveryManager := NewManager(ctx, log.NewNopLogger(), reg, sdMetrics)
	require.NotNil(t, discoveryManager)
	discoveryManager.updatert = 100 * time.Millisecond
	go discoveryManager.Run()

	c := map[string]Configs{
		"prometheus": {
			staticConfig("foo:9090", "bar:9090"),
			staticConfig("baz:9090"),
		},
	}
	discoveryManager.ApplyConfig(c)

	syncedTargets := <-discoveryManager.SyncCh()
	p := pk("static", "prometheus", 0)
	verifyPresence(t, discoveryManager.targets, p, "{__address__=\"foo:9090\"}", true)
	verifyPresence(t, discoveryManager.targets, p, "{__address__=\"bar:9090\"}", true)
	p = pk("static", "prometheus", 1)
	verifyPresence(t, discoveryManager.targets, p, "{__address__=\"baz:9090\"}", true)
	require.Len(t, discoveryManager.targets, 2)
	require.Len(t, syncedTargets, 1)
	verifySyncedPresence(t, syncedTargets, "prometheus", "{__address__=\"foo:9090\"}", true)
	verifySyncedPresence(t, syncedTargets, "prometheus", "{__address__=\"bar:9090\"}", true)
	verifySyncedPresence(t, syncedTargets, "prometheus", "{__address__=\"baz:9090\"}", true)
	require.Len(t, syncedTargets["prometheus"], 3)
}

// TestTargetSetRecreatesEmptyStaticConfigs ensures that reloading a config file after
// removing all targets from the static_configs sends an update with empty targetGroups.
// This is required to signal the receiver that this target set has no current targets.
func TestTargetSetRecreatesEmptyStaticConfigs(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	reg := prometheus.NewRegistry()
	_, sdMetrics := NewTestMetrics(t, reg)

	discoveryManager := NewManager(ctx, log.NewNopLogger(), reg, sdMetrics)
	require.NotNil(t, discoveryManager)
	discoveryManager.updatert = 100 * time.Millisecond
	go discoveryManager.Run()

	c := map[string]Configs{
		"prometheus": {
			staticConfig("foo:9090"),
		},
	}
	discoveryManager.ApplyConfig(c)

	syncedTargets := <-discoveryManager.SyncCh()
	p := pk("static", "prometheus", 0)
	verifyPresence(t, discoveryManager.targets, p, "{__address__=\"foo:9090\"}", true)
	require.Len(t, syncedTargets, 1)
	verifySyncedPresence(t, syncedTargets, "prometheus", "{__address__=\"foo:9090\"}", true)
	require.Len(t, syncedTargets["prometheus"], 1)

	c["prometheus"] = Configs{
		StaticConfig{{}},
	}
	discoveryManager.ApplyConfig(c)

	syncedTargets = <-discoveryManager.SyncCh()
	p = pk("static", "prometheus", 1)
	targetGroups, ok := discoveryManager.targets[p]
	require.True(t, ok, "'%v' should be present in target groups", p)
	group, ok := targetGroups[""]
	require.True(t, ok, "missing '' key in target groups %v", targetGroups)

	require.Empty(t, group.Targets, "Invalid number of targets.")
	require.Len(t, syncedTargets, 1)
	require.Len(t, syncedTargets["prometheus"], 1)
	require.Nil(t, syncedTargets["prometheus"][0].Labels)
}

func TestIdenticalConfigurationsAreCoalesced(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	reg := prometheus.NewRegistry()
	_, sdMetrics := NewTestMetrics(t, reg)

	discoveryManager := NewManager(ctx, nil, reg, sdMetrics)
	require.NotNil(t, discoveryManager)
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

	syncedTargets := <-discoveryManager.SyncCh()
	verifyPresence(t, discoveryManager.targets, pk("static", "prometheus", 0), "{__address__=\"foo:9090\"}", true)
	verifyPresence(t, discoveryManager.targets, pk("static", "prometheus2", 0), "{__address__=\"foo:9090\"}", true)
	require.Len(t, discoveryManager.providers, 1, "Invalid number of providers.")
	require.Len(t, syncedTargets, 2)
	verifySyncedPresence(t, syncedTargets, "prometheus", "{__address__=\"foo:9090\"}", true)
	require.Len(t, syncedTargets["prometheus"], 1)
	verifySyncedPresence(t, syncedTargets, "prometheus2", "{__address__=\"foo:9090\"}", true)
	require.Len(t, syncedTargets["prometheus2"], 1)
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

	reg := prometheus.NewRegistry()
	_, sdMetrics := NewTestMetrics(t, reg)

	discoveryManager := NewManager(ctx, log.NewNopLogger(), reg, sdMetrics)
	require.NotNil(t, discoveryManager)
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

// NewDiscovererMetrics implements discovery.Config.
func (errorConfig) NewDiscovererMetrics(prometheus.Registerer, RefreshMetricsInstantiator) DiscovererMetrics {
	return &NoopDiscovererMetrics{}
}

type lockStaticConfig struct {
	mu     *sync.Mutex
	config StaticConfig
}

// NewDiscovererMetrics implements discovery.Config.
func (lockStaticConfig) NewDiscovererMetrics(prometheus.Registerer, RefreshMetricsInstantiator) DiscovererMetrics {
	return &NoopDiscovererMetrics{}
}

func (s lockStaticConfig) Name() string { return "lockstatic" }
func (s lockStaticConfig) NewDiscoverer(DiscovererOptions) (Discoverer, error) {
	return (lockStaticDiscoverer)(s), nil
}

type lockStaticDiscoverer lockStaticConfig

func (s lockStaticDiscoverer) Run(ctx context.Context, up chan<- []*targetgroup.Group) {
	// TODO: existing implementation closes up chan, but documentation explicitly forbids it...?
	defer close(up)
	s.mu.Lock()
	defer s.mu.Unlock()
	select {
	case <-ctx.Done():
	case up <- s.config:
	}
}

func TestGaugeFailedConfigs(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	reg := prometheus.NewRegistry()
	_, sdMetrics := NewTestMetrics(t, reg)

	discoveryManager := NewManager(ctx, log.NewNopLogger(), reg, sdMetrics)
	require.NotNil(t, discoveryManager)
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

	failedCount := client_testutil.ToFloat64(discoveryManager.metrics.FailedConfigs)
	require.Equal(t, 3.0, failedCount, "Expected to have 3 failed configs.")

	c["prometheus"] = Configs{
		staticConfig("foo:9090"),
	}
	discoveryManager.ApplyConfig(c)
	<-discoveryManager.SyncCh()

	failedCount = client_testutil.ToFloat64(discoveryManager.metrics.FailedConfigs)
	require.Equal(t, 0.0, failedCount, "Expected to get no failed config.")
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

			reg := prometheus.NewRegistry()
			_, sdMetrics := NewTestMetrics(t, reg)

			mgr := NewManager(ctx, nil, reg, sdMetrics)
			require.NotNil(t, mgr)
			mgr.updatert = updateDelay
			go mgr.Run()

			for name, p := range tc.providers {
				mgr.StartCustomProvider(ctx, name, p)
			}

			for i, expected := range tc.expected {
				time.Sleep(expected.delay)
				select {
				case <-ctx.Done():
					require.FailNow(t, "step %d: no update received in the expected timeframe", i)
				case tgs, ok := <-mgr.SyncCh():
					require.True(t, ok, "step %d: discovery manager channel is closed", i)
					require.Equal(t, len(expected.tgs), len(tgs), "step %d: targets mismatch", i)

					for k := range expected.tgs {
						_, ok := tgs[k]
						require.True(t, ok, "step %d: target group not found: %s", i, k)
						assertEqualGroups(t, tgs[k], expected.tgs[k])
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
		select {
		case <-ctx.Done():
			return
		case upCh <- tgs:
		}
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

// TestTargetSetTargetGroupsUpdateDuringApplyConfig is used to detect races when
// ApplyConfig happens at the same time as targets update.
func TestTargetSetTargetGroupsUpdateDuringApplyConfig(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	reg := prometheus.NewRegistry()
	_, sdMetrics := NewTestMetrics(t, reg)

	discoveryManager := NewManager(ctx, log.NewNopLogger(), reg, sdMetrics)
	require.NotNil(t, discoveryManager)
	discoveryManager.updatert = 100 * time.Millisecond
	go discoveryManager.Run()

	td := newTestDiscoverer()

	c := map[string]Configs{
		"prometheus": {
			td,
		},
	}
	discoveryManager.ApplyConfig(c)

	var wg sync.WaitGroup
	wg.Add(2000)

	start := make(chan struct{})
	for i := 0; i < 1000; i++ {
		go func() {
			<-start
			td.update([]*targetgroup.Group{
				{
					Targets: []model.LabelSet{
						{model.AddressLabel: model.LabelValue("127.0.0.1:9090")},
					},
				},
			})
			wg.Done()
		}()
	}

	for i := 0; i < 1000; i++ {
		go func(i int) {
			<-start
			c := map[string]Configs{
				fmt.Sprintf("prometheus-%d", i): {
					td,
				},
			}
			discoveryManager.ApplyConfig(c)
			wg.Done()
		}(i)
	}

	close(start)
	wg.Wait()
}

// testDiscoverer is a config and a discoverer that can adjust targets with a
// simple function.
type testDiscoverer struct {
	up    chan<- []*targetgroup.Group
	ready chan struct{}
}

func newTestDiscoverer() *testDiscoverer {
	return &testDiscoverer{
		ready: make(chan struct{}),
	}
}

// NewDiscovererMetrics implements discovery.Config.
func (*testDiscoverer) NewDiscovererMetrics(prometheus.Registerer, RefreshMetricsInstantiator) DiscovererMetrics {
	return &NoopDiscovererMetrics{}
}

// Name implements Config.
func (t *testDiscoverer) Name() string {
	return "test"
}

// NewDiscoverer implements Config.
func (t *testDiscoverer) NewDiscoverer(DiscovererOptions) (Discoverer, error) {
	return t, nil
}

// Run implements Discoverer.
func (t *testDiscoverer) Run(ctx context.Context, up chan<- []*targetgroup.Group) {
	t.up = up
	close(t.ready)
	<-ctx.Done()
}

func (t *testDiscoverer) update(tgs []*targetgroup.Group) {
	<-t.ready
	t.up <- tgs
}

func TestUnregisterMetrics(t *testing.T) {
	reg := prometheus.NewRegistry()
	// Check that all metrics can be unregistered, allowing a second manager to be created.
	for i := 0; i < 2; i++ {
		ctx, cancel := context.WithCancel(context.Background())

		refreshMetrics, sdMetrics := NewTestMetrics(t, reg)

		discoveryManager := NewManager(ctx, log.NewNopLogger(), reg, sdMetrics)
		// discoveryManager will be nil if there was an error configuring metrics.
		require.NotNil(t, discoveryManager)
		// Unregister all metrics.
		discoveryManager.UnregisterMetrics()
		for _, sdMetric := range sdMetrics {
			sdMetric.Unregister()
		}
		refreshMetrics.Unregister()
		cancel()
	}
}
