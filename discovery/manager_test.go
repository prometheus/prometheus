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
	"sort"
	"strconv"
	"testing"
	"time"

	"github.com/go-kit/kit/log"
	"github.com/stretchr/testify/require"

	"github.com/prometheus/common/model"
	"github.com/prometheus/prometheus/discovery/targetgroup"
)

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

			discoveryManager := NewManager(ctx, log.NewNopLogger(), WaitDuration(100*time.Millisecond))

			var totalUpdatesCount int
			provUpdates := make(chan []*targetgroup.Group)
			for _, up := range tc.updates {
				go newMockDiscoveryProvider(up...).Run(ctx, provUpdates)
				if len(up) > 0 {
					totalUpdatesCount = totalUpdatesCount + len(up)
				}
			}

		Loop:
			for x := 0; x < totalUpdatesCount; x++ {
				select {
				case <-ctx.Done():
					t.Errorf("%d: no update arrived within the timeout limit", x)
					break Loop
				case tgs := <-provUpdates:
					discoveryManager.updateGroup(poolKey{setName: strconv.Itoa(i), provider: tc.title}, tgs)
					for _, got := range discoveryManager.allGroups() {
						assertEqualGroups(t, tc.expectedTargets[x], got)
					}
				}
			}
		})
	}
}

func assertEqualGroups(t *testing.T, expected, got []*targetgroup.Group) {
	t.Helper()

	// Need to sort by the groups's source as the received order is not guaranteed.
	sort.Sort(byGroupSource(got))
	sort.Sort(byGroupSource(expected))

	require.Equal(t, expected, got)
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
	provider := &onceProvider{
		tgs: []*targetgroup.Group{
			{
				Targets: []model.LabelSet{
					{"__address__": "foo:9090"},
					{"__address__": "bar:9090"},
				},
			},
		},
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	discoveryManager := NewManager(ctx, log.NewNopLogger(), WaitDuration(200*time.Millisecond))
	go discoveryManager.Run()

	discoveryManager.ApplyConfig([]Provider{NewProvider("prometheus", provider)})

	<-discoveryManager.SyncCh()
	verifyPresence(t, discoveryManager.targets, poolKey{setName: "prometheus", provider: "prometheus"}, "{__address__=\"foo:9090\"}", true)
	verifyPresence(t, discoveryManager.targets, poolKey{setName: "prometheus", provider: "prometheus"}, "{__address__=\"bar:9090\"}", true)

	provider = &onceProvider{
		tgs: []*targetgroup.Group{
			{
				Targets: []model.LabelSet{{"__address__": "foo:9090"}},
			},
		},
	}
	discoveryManager.ApplyConfig([]Provider{NewProvider("prometheus", provider)})

	<-discoveryManager.SyncCh()
	verifyPresence(t, discoveryManager.targets, poolKey{setName: "prometheus", provider: "prometheus"}, "{__address__=\"foo:9090\"}", true)
	verifyPresence(t, discoveryManager.targets, poolKey{setName: "prometheus", provider: "prometheus"}, "{__address__=\"bar:9090\"}", false)
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

			mgr := NewManager(ctx, nil, WaitDuration(updateDelay))
			go mgr.Run()

			for name, p := range tc.providers {
				mgr.startProvider(ctx, NewProvider(name, p))
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
						assertEqualGroups(t, expected.tgs[k], tgs[k])
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
			t := time.NewTicker(u.interval)
			defer t.Stop()
		Loop:
			for {
				select {
				case <-ctx.Done():
					return
				case <-t.C:
					break Loop
				}
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
