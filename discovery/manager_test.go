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
	"reflect"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/prometheus/common/model"
	"github.com/prometheus/prometheus/config"
	yaml "gopkg.in/yaml.v2"
)

func TestTargetSetThrottlesTheSyncCalls(t *testing.T) {
	testCases := []struct {
		title             string
		updates           map[string][]update
		expectedSyncCalls [][]string
	}{
		{
			title: "Single TP no updates",
			updates: map[string][]update{
				"tp1": {},
			},
			expectedSyncCalls: [][]string{
				{},
			},
		},
		{
			title: "Multips TPs no updates",
			updates: map[string][]update{
				"tp1": {},
				"tp2": {},
				"tp3": {},
			},
			expectedSyncCalls: [][]string{
				{},
			},
		},
		{
			title: "Single TP empty initials",
			updates: map[string][]update{
				"tp1": {
					{
						targetGroups: []config.TargetGroup{},
						interval:     5,
					},
				},
			},
			expectedSyncCalls: [][]string{
				{},
			},
		},
		{
			title: "Multiple TPs empty initials",
			updates: map[string][]update{
				"tp1": {
					{
						targetGroups: []config.TargetGroup{},
						interval:     5,
					},
				},
				"tp2": {
					{
						targetGroups: []config.TargetGroup{},
						interval:     500,
					},
				},
				"tp3": {
					{
						targetGroups: []config.TargetGroup{},
						interval:     100,
					},
				},
			},
			expectedSyncCalls: [][]string{
				{},
			},
		},
		{
			title: "Multiple TPs empty initials with a delay",
			updates: map[string][]update{
				"tp1": {
					{
						targetGroups: []config.TargetGroup{},
						interval:     6000,
					},
				},
				"tp2": {
					{
						targetGroups: []config.TargetGroup{},
						interval:     6500,
					},
				},
			},
			expectedSyncCalls: [][]string{
				{},
			},
		},
		{
			title: "Single TP initials only",
			updates: map[string][]update{
				"tp1": {
					{
						targetGroups: []config.TargetGroup{{Source: "initial1"}, {Source: "initial2"}},
						interval:     0,
					},
				},
			},
			expectedSyncCalls: [][]string{
				{"initial1", "initial2"},
			},
		},
		{
			title: "Multiple TPs initials only",
			updates: map[string][]update{
				"tp1": {
					{
						targetGroups: []config.TargetGroup{{Source: "tp1-initial1"}, {Source: "tp1-initial2"}},
						interval:     0,
					},
				},
				"tp2": {
					{
						targetGroups: []config.TargetGroup{{Source: "tp2-initial1"}},
						interval:     0,
					},
				},
			},
			expectedSyncCalls: [][]string{
				{"tp1-initial1", "tp1-initial2", "tp2-initial1"},
			},
		},
		{
			title: "Single TP delayed initials only",
			updates: map[string][]update{
				"tp1": {
					{
						targetGroups: []config.TargetGroup{{Source: "initial1"}, {Source: "initial2"}},
						interval:     6000,
					},
				},
			},
			expectedSyncCalls: [][]string{
				{},
				{"initial1", "initial2"},
			},
		},
		{
			title: "Multiple TPs with some delayed initials",
			updates: map[string][]update{
				"tp1": {
					{
						targetGroups: []config.TargetGroup{{Source: "tp1-initial1"}, {Source: "tp1-initial2"}},
						interval:     100,
					},
				},
				"tp2": {
					{
						targetGroups: []config.TargetGroup{{Source: "tp2-initial1"}, {Source: "tp2-initial2"}, {Source: "tp2-initial3"}},
						interval:     6000,
					},
				},
			},
			expectedSyncCalls: [][]string{
				{"tp1-initial1", "tp1-initial2"},
				{"tp1-initial1", "tp1-initial2", "tp2-initial1", "tp2-initial2", "tp2-initial3"},
			},
		},
		{
			title: "Single TP initials followed by empty updates",
			updates: map[string][]update{
				"tp1": {
					{
						targetGroups: []config.TargetGroup{{Source: "initial1"}, {Source: "initial2"}},
						interval:     0,
					},
					{
						targetGroups: []config.TargetGroup{},
						interval:     10,
					},
				},
			},
			expectedSyncCalls: [][]string{
				{"initial1", "initial2"},
			},
		},
		{
			title: "Single TP initials and new groups",
			updates: map[string][]update{
				"tp1": {
					{
						targetGroups: []config.TargetGroup{{Source: "initial1"}, {Source: "initial2"}},
						interval:     0,
					},
					{
						targetGroups: []config.TargetGroup{{Source: "update1"}, {Source: "update2"}},
						interval:     10,
					},
				},
			},
			expectedSyncCalls: [][]string{
				{"initial1", "initial2"},
				{"initial1", "initial2", "update1", "update2"},
			},
		},
		{
			title: "Multiple TPs initials and new groups",
			updates: map[string][]update{
				"tp1": {
					{
						targetGroups: []config.TargetGroup{{Source: "tp1-initial1"}, {Source: "tp1-initial2"}},
						interval:     10,
					},
					{
						targetGroups: []config.TargetGroup{{Source: "tp1-update1"}, {Source: "tp1-update2"}},
						interval:     1500,
					},
				},
				"tp2": {
					{
						targetGroups: []config.TargetGroup{{Source: "tp2-initial1"}, {Source: "tp2-initial2"}},
						interval:     100,
					},
					{
						targetGroups: []config.TargetGroup{{Source: "tp2-update1"}, {Source: "tp2-update2"}},
						interval:     10,
					},
				},
			},
			expectedSyncCalls: [][]string{
				{"tp1-initial1", "tp1-initial2", "tp2-initial1", "tp2-initial2"},
				{"tp1-initial1", "tp1-initial2", "tp2-initial1", "tp2-initial2", "tp1-update1", "tp1-update2", "tp2-update1", "tp2-update2"},
			},
		},
		{
			title: "One tp initials arrive after other tp updates but still within 5 seconds",
			updates: map[string][]update{
				"tp1": {
					{
						targetGroups: []config.TargetGroup{{Source: "tp1-initial1"}, {Source: "tp1-initial2"}},
						interval:     10,
					},
					{
						targetGroups: []config.TargetGroup{{Source: "tp1-update1"}, {Source: "tp1-update2"}},
						interval:     1500,
					},
				},
				"tp2": {
					{
						targetGroups: []config.TargetGroup{{Source: "tp2-initial1"}, {Source: "tp2-initial2"}},
						interval:     2000,
					},
					{
						targetGroups: []config.TargetGroup{{Source: "tp2-update1"}, {Source: "tp2-update2"}},
						interval:     1000,
					},
				},
			},
			expectedSyncCalls: [][]string{
				{"tp1-initial1", "tp1-initial2", "tp1-update1", "tp1-update2", "tp2-initial1", "tp2-initial2"},
				{"tp1-initial1", "tp1-initial2", "tp1-update1", "tp1-update2", "tp2-initial1", "tp2-initial2", "tp2-update1", "tp2-update2"},
			},
		},
		{
			title: "One tp initials arrive after other tp updates and after 5 seconds",
			updates: map[string][]update{
				"tp1": {
					{
						targetGroups: []config.TargetGroup{{Source: "tp1-initial1"}, {Source: "tp1-initial2"}},
						interval:     10,
					},
					{
						targetGroups: []config.TargetGroup{{Source: "tp1-update1"}, {Source: "tp1-update2"}},
						interval:     1500,
					},
					{
						targetGroups: []config.TargetGroup{{Source: "tp1-update3"}},
						interval:     5000,
					},
				},
				"tp2": {
					{
						targetGroups: []config.TargetGroup{{Source: "tp2-initial1"}, {Source: "tp2-initial2"}},
						interval:     6000,
					},
				},
			},
			expectedSyncCalls: [][]string{
				{"tp1-initial1", "tp1-initial2", "tp1-update1", "tp1-update2"},
				{"tp1-initial1", "tp1-initial2", "tp1-update1", "tp1-update2", "tp2-initial1", "tp2-initial2", "tp1-update3"},
			},
		},
		{
			title: "Single TP initials and new groups after a delay",
			updates: map[string][]update{
				"tp1": {
					{
						targetGroups: []config.TargetGroup{{Source: "initial1"}, {Source: "initial2"}},
						interval:     6000,
					},
					{
						targetGroups: []config.TargetGroup{{Source: "update1"}, {Source: "update2"}},
						interval:     10,
					},
				},
			},
			expectedSyncCalls: [][]string{
				{},
				{"initial1", "initial2", "update1", "update2"},
			},
		},
		{
			title: "Single TP initial and successive updates",
			updates: map[string][]update{
				"tp1": {
					{
						targetGroups: []config.TargetGroup{{Source: "initial1"}},
						interval:     100,
					},
					{
						targetGroups: []config.TargetGroup{{Source: "update1"}},
						interval:     100,
					},
					{
						targetGroups: []config.TargetGroup{{Source: "update2"}},
						interval:     10,
					},
					{
						targetGroups: []config.TargetGroup{{Source: "update3"}},
						interval:     10,
					},
				},
			},
			expectedSyncCalls: [][]string{
				{"initial1"},
				{"initial1", "update1", "update2", "update3"},
			},
		},
		{
			title: "Multiple TPs initials and successive updates",
			updates: map[string][]update{
				"tp1": {
					{
						targetGroups: []config.TargetGroup{{Source: "tp1-initial1"}},
						interval:     1000,
					},
					{
						targetGroups: []config.TargetGroup{{Source: "tp1-update1"}},
						interval:     1000,
					},
					{
						targetGroups: []config.TargetGroup{{Source: "tp1-update2"}},
						interval:     2000,
					},
					{
						targetGroups: []config.TargetGroup{{Source: "tp1-update3"}},
						interval:     2000,
					},
				},
				"tp2": {
					{
						targetGroups: []config.TargetGroup{{Source: "tp2-initial1"}},
						interval:     3000,
					},
					{
						targetGroups: []config.TargetGroup{{Source: "tp2-update1"}},
						interval:     1000,
					},
					{
						targetGroups: []config.TargetGroup{{Source: "tp2-update2"}},
						interval:     3000,
					},
					{
						targetGroups: []config.TargetGroup{{Source: "tp2-update3"}},
						interval:     2000,
					},
				},
			},
			expectedSyncCalls: [][]string{
				{"tp1-initial1", "tp1-update1", "tp2-initial1"},
				{"tp1-initial1", "tp1-update1", "tp2-initial1", "tp1-update2", "tp1-update3", "tp2-update1", "tp2-update2"},
				{"tp1-initial1", "tp1-update1", "tp2-initial1", "tp1-update2", "tp1-update3", "tp2-update1", "tp2-update2", "tp2-update3"},
			},
		},
		{
			title: "Single TP Multiple updates 5 second window",
			updates: map[string][]update{
				"tp1": {
					{
						targetGroups: []config.TargetGroup{{Source: "initial1"}},
						interval:     10,
					},
					{
						targetGroups: []config.TargetGroup{{Source: "update1"}},
						interval:     25,
					},
					{
						targetGroups: []config.TargetGroup{{Source: "update2"}, {Source: "update3"}, {Source: "update4"}},
						interval:     10,
					},
					{
						targetGroups: []config.TargetGroup{{Source: "update5"}},
						interval:     0,
					},
					{
						targetGroups: []config.TargetGroup{{Source: "update6"}, {Source: "update7"}, {Source: "update8"}},
						interval:     70,
					},
				},
			},
			expectedSyncCalls: [][]string{
				{"initial1"},
				{"initial1", "update1", "update2", "update3", "update4", "update5", "update6", "update7", "update8"},
			},
		},
		{
			title: "Single TP Single provider empty update in between",
			updates: map[string][]update{
				"tp1": {
					{
						targetGroups: []config.TargetGroup{{Source: "initial1"}, {Source: "initial2"}},
						interval:     30,
					},
					{
						targetGroups: []config.TargetGroup{{Source: "update1"}, {Source: "update2"}},
						interval:     300,
					},
					{
						targetGroups: []config.TargetGroup{},
						interval:     10,
					},
					{
						targetGroups: []config.TargetGroup{{Source: "update3"}, {Source: "update4"}, {Source: "update5"}, {Source: "update6"}},
						interval:     6000,
					},
				},
			},
			expectedSyncCalls: [][]string{
				{"initial1", "initial2"},
				{"initial1", "initial2", "update1", "update2"},
				{"initial1", "initial2", "update1", "update2", "update3", "update4", "update5", "update6"},
			},
		},
	}

	for i, testCase := range testCases {
		finalize := make(chan bool)

		syncCallCount := 0
		syncedGroups := make([][]string, 0)

		targetSet := NewTargetSet(&mockSyncer{
			sync: func(tgs []*config.TargetGroup) {

				currentCallGroup := make([]string, len(tgs))
				for i, tg := range tgs {
					currentCallGroup[i] = tg.Source
				}
				syncedGroups = append(syncedGroups, currentCallGroup)

				syncCallCount++
				if syncCallCount == len(testCase.expectedSyncCalls) {
					// All the groups are sent, we can start asserting.
					close(finalize)
				}
			},
		})

		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		targetProviders := map[string]TargetProvider{}
		for tpName, tpUpdates := range testCase.updates {
			tp := newMockTargetProvider(tpUpdates)
			targetProviders[tpName] = tp
		}

		go targetSet.Run(ctx)
		targetSet.UpdateProviders(targetProviders)

		select {
		case <-time.After(20 * time.Second):
			t.Errorf("%d. %q: Test timed out after 20 seconds. All targets should be sent within the timeout", i, testCase.title)

		case <-finalize:
			for name, tp := range targetProviders {
				runCallCount := tp.(mockTargetProvider).callCount()
				if runCallCount != 1 {
					t.Errorf("%d. %q: TargetProvider Run should be called once for each target provider. For %q was called %d times", i, testCase.title, name, runCallCount)
				}
			}

			if len(syncedGroups) != len(testCase.expectedSyncCalls) {
				t.Errorf("%d. %q: received sync calls: \n %v \n do not match expected calls: \n %v \n", i, testCase.title, syncedGroups, testCase.expectedSyncCalls)
			}

			for j := range syncedGroups {
				if len(syncedGroups[j]) != len(testCase.expectedSyncCalls[j]) {
					t.Errorf("%d. %q: received sync calls in call [%v]: \n %v \n do not match expected calls: \n %v \n", i, testCase.title, j, syncedGroups[j], testCase.expectedSyncCalls[j])
				}

				expectedGroupsMap := make(map[string]struct{})
				for _, expectedGroup := range testCase.expectedSyncCalls[j] {
					expectedGroupsMap[expectedGroup] = struct{}{}
				}

				for _, syncedGroup := range syncedGroups[j] {
					if _, ok := expectedGroupsMap[syncedGroup]; !ok {
						t.Errorf("%d. %q: '%s' does not exist in expected target groups: %s", i, testCase.title, syncedGroup, testCase.expectedSyncCalls[j])
					} else {
						delete(expectedGroupsMap, syncedGroup) // Remove used targets from the map.
					}
				}
			}
		}
	}
}

func TestTargetSetConsolidatesToTheLatestState(t *testing.T) {
	testCases := []struct {
		title   string
		updates map[string][]update
	}{
		{
			title: "Single TP update same initial group multiple times",
			updates: map[string][]update{
				"tp1": {
					{
						targetGroups: []config.TargetGroup{
							{
								Source:  "initial1",
								Targets: []model.LabelSet{{"__instance__": "10.11.122.11:6003"}},
							},
						},
						interval: 100,
					},
					{
						targetGroups: []config.TargetGroup{
							{
								Source:  "initial1",
								Targets: []model.LabelSet{{"__instance__": "10.11.122.11:6003"}, {"__instance__": "10.11.122.12:6003"}},
							},
						},
						interval: 250,
					},
					{
						targetGroups: []config.TargetGroup{
							{
								Source:  "initial1",
								Targets: []model.LabelSet{{"__instance__": "10.11.122.12:6003"}},
							},
						},
						interval: 250,
					},
				},
			},
		},
		{
			title: "Multiple TPs update",
			updates: map[string][]update{
				"tp1": {
					{
						targetGroups: []config.TargetGroup{
							{
								Source:  "tp1-initial1",
								Targets: []model.LabelSet{{"__instance__": "10.11.122.11:6003"}},
							},
						},
						interval: 3,
					},
					{
						targetGroups: []config.TargetGroup{
							{
								Source:  "tp1-update1",
								Targets: []model.LabelSet{{"__instance__": "10.11.122.12:6003"}},
							},
						},
						interval: 10,
					},
					{
						targetGroups: []config.TargetGroup{
							{
								Source: "tp1-update1",
								Targets: []model.LabelSet{
									{"__instance__": "10.11.122.12:6003"},
									{"__instance__": "10.11.122.13:6003"},
									{"__instance__": "10.11.122.14:6003"},
								},
							},
						},
						interval: 10,
					},
				},
				"tp2": {
					{
						targetGroups: []config.TargetGroup{
							{
								Source:  "tp2-initial1",
								Targets: []model.LabelSet{{"__instance__": "10.11.122.11:6003"}},
							},
						},
						interval: 3,
					},
					{
						targetGroups: []config.TargetGroup{
							{
								Source:  "tp2-initial1",
								Targets: []model.LabelSet{{"__instance__": "10.11.122.12:6003"}},
							},
						},
						interval: 10,
					},
					{
						targetGroups: []config.TargetGroup{
							{
								Source: "tp2-update1",
								Targets: []model.LabelSet{
									{"__instance__": "10.11.122.12:6003"},
									{"__instance__": "10.11.122.13:6003"},
									{"__instance__": "10.11.122.14:6003"},
								},
							},
						},
						interval: 10,
					},
				},
			},
		},
		{
			title: "Multiple TPs update II",
			updates: map[string][]update{
				"tp1": {
					{
						targetGroups: []config.TargetGroup{
							{
								Source:  "tp1-initial1",
								Targets: []model.LabelSet{{"__instance__": "10.11.122.11:6003"}},
							},
							{
								Source:  "tp1-initial2",
								Targets: []model.LabelSet{{"__instance__": "10.11.122.12:6003"}},
							},
						},
						interval: 100,
					},
					{
						targetGroups: []config.TargetGroup{
							{
								Source:  "tp1-update1",
								Targets: []model.LabelSet{{"__instance__": "10.11.122.13:6003"}},
							},
							{
								Source: "tp1-initial2",
								Targets: []model.LabelSet{
									{"__instance__": "10.11.122.12:6003"},
									{"__instance__": "10.11.122.14:6003"},
								},
							},
						},
						interval: 100,
					},
					{
						targetGroups: []config.TargetGroup{
							{
								Source:  "tp1-update2",
								Targets: []model.LabelSet{{"__instance__": "10.11.122.15:6003"}},
							},
							{
								Source:  "tp1-initial1",
								Targets: []model.LabelSet{},
							},
						},
						interval: 100,
					},
					{
						targetGroups: []config.TargetGroup{
							{
								Source: "tp1-update1",
								Targets: []model.LabelSet{
									{"__instance__": "10.11.122.16:6003"},
									{"__instance__": "10.11.122.17:6003"},
									{"__instance__": "10.11.122.18:6003"},
								},
							},
						},
						interval: 100,
					},
				},
				"tp2": {
					{
						targetGroups: []config.TargetGroup{},
						interval:     100,
					},
					{
						targetGroups: []config.TargetGroup{
							{
								Source:  "tp2-update1",
								Targets: []model.LabelSet{{"__instance__": "10.11.122.13:6003"}},
							},
						},
						interval: 100,
					},
					{
						targetGroups: []config.TargetGroup{
							{
								Source:  "tp2-update2",
								Targets: []model.LabelSet{{"__instance__": "10.11.122.15:6003"}},
							},
							{
								Source:  "tp2-update1",
								Targets: []model.LabelSet{},
							},
						},
						interval: 300,
					},
				},
			},
		},
		{
			title: "Three rounds of sync call",
			updates: map[string][]update{
				"tp1": {
					{
						targetGroups: []config.TargetGroup{
							{
								Source: "tp1-initial1",
								Targets: []model.LabelSet{
									{"__instance__": "10.11.122.11:6003"},
									{"__instance__": "10.11.122.12:6003"},
									{"__instance__": "10.11.122.13:6003"},
								},
							},
							{
								Source:  "tp1-initial2",
								Targets: []model.LabelSet{{"__instance__": "10.11.122.14:6003"}},
							},
						},
						interval: 1000,
					},
					{
						targetGroups: []config.TargetGroup{
							{
								Source:  "tp1-initial1",
								Targets: []model.LabelSet{{"__instance__": "10.11.122.12:6003"}},
							},
							{
								Source:  "tp1-update1",
								Targets: []model.LabelSet{{"__instance__": "10.11.122.15:6003"}},
							},
						},
						interval: 3000,
					},
					{
						targetGroups: []config.TargetGroup{
							{
								Source:  "tp1-initial1",
								Targets: []model.LabelSet{},
							},
							{
								Source: "tp1-update1",
								Targets: []model.LabelSet{
									{"__instance__": "10.11.122.15:6003"},
									{"__instance__": "10.11.122.16:6003"},
								},
							},
						},
						interval: 3000,
					},
				},
				"tp2": {
					{
						targetGroups: []config.TargetGroup{
							{
								Source: "tp2-initial1",
								Targets: []model.LabelSet{
									{"__instance__": "10.11.122.11:6003"},
								},
							},
							{
								Source:  "tp2-initial2",
								Targets: []model.LabelSet{{"__instance__": "10.11.122.14:6003"}},
							},
							{
								Source:  "tp2-initial3",
								Targets: []model.LabelSet{{"__instance__": "10.11.122.15:6003"}},
							},
						},
						interval: 6000,
					},
					{
						targetGroups: []config.TargetGroup{
							{
								Source:  "tp2-initial1",
								Targets: []model.LabelSet{{"__instance__": "10.11.122.11:6003"}, {"__instance__": "10.11.122.12:6003"}},
							},
							{
								Source:  "tp2-update1",
								Targets: []model.LabelSet{{"__instance__": "10.11.122.15:6003"}},
							},
						},
						interval: 3000,
					},
				},
			},
		},
	}

	// Function to determine if the sync call received the latest state of
	// all the target groups that came out of the target provider.
	endStateAchieved := func(groupsSentToSyc []*config.TargetGroup, endState map[string]config.TargetGroup) bool {

		if len(groupsSentToSyc) != len(endState) {
			return false
		}

		for _, tg := range groupsSentToSyc {
			if _, ok := endState[tg.Source]; ok == false {
				// The target group does not exist in the end state.
				return false
			}

			if reflect.DeepEqual(endState[tg.Source], *tg) == false {
				// The target group has not reached its final state yet.
				return false
			}

			delete(endState, tg.Source) // Remove used target groups.
		}

		return true
	}

	for i, testCase := range testCases {
		expectedGroups := make(map[string]config.TargetGroup)
		for _, tpUpdates := range testCase.updates {
			for _, update := range tpUpdates {
				for _, targetGroup := range update.targetGroups {
					expectedGroups[targetGroup.Source] = targetGroup
				}
			}
		}

		finalize := make(chan bool)

		targetSet := NewTargetSet(&mockSyncer{
			sync: func(tgs []*config.TargetGroup) {

				endState := make(map[string]config.TargetGroup)
				for k, v := range expectedGroups {
					endState[k] = v
				}

				if endStateAchieved(tgs, endState) == false {
					return
				}

				close(finalize)
			},
		})

		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		targetProviders := map[string]TargetProvider{}
		for tpName, tpUpdates := range testCase.updates {
			tp := newMockTargetProvider(tpUpdates)
			targetProviders[tpName] = tp
		}

		go targetSet.Run(ctx)
		targetSet.UpdateProviders(targetProviders)

		select {
		case <-time.After(20 * time.Second):
			t.Errorf("%d. %q: Test timed out after 20 seconds. All targets should be sent within the timeout", i, testCase.title)

		case <-finalize:
			// System successfully reached to the end state.
		}
	}
}

func TestTargetSetRecreatesTargetGroupsEveryRun(t *testing.T) {
	verifyPresence := func(tgroups map[string]*config.TargetGroup, name string, present bool) {
		if _, ok := tgroups[name]; ok != present {
			msg := ""
			if !present {
				msg = "not "
			}
			t.Fatalf("'%s' should %sbe present in TargetSet.tgroups: %s", name, msg, tgroups)
		}
	}

	cfg := &config.ServiceDiscoveryConfig{}

	sOne := `
static_configs:
- targets: ["foo:9090"]
- targets: ["bar:9090"]
`
	if err := yaml.Unmarshal([]byte(sOne), cfg); err != nil {
		t.Fatalf("Unable to load YAML config sOne: %s", err)
	}
	called := make(chan struct{})

	ts := NewTargetSet(&mockSyncer{
		sync: func([]*config.TargetGroup) { called <- struct{}{} },
	})
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go ts.Run(ctx)

	ts.UpdateProviders(ProvidersFromConfig(*cfg, nil))
	<-called

	verifyPresence(ts.tgroups, "static/0/0", true)
	verifyPresence(ts.tgroups, "static/0/1", true)

	sTwo := `
static_configs:
- targets: ["foo:9090"]
`
	if err := yaml.Unmarshal([]byte(sTwo), cfg); err != nil {
		t.Fatalf("Unable to load YAML config sTwo: %s", err)
	}

	ts.UpdateProviders(ProvidersFromConfig(*cfg, nil))
	<-called

	verifyPresence(ts.tgroups, "static/0/0", true)
	verifyPresence(ts.tgroups, "static/0/1", false)
}

func TestTargetSetRunsSameTargetProviderMultipleTimes(t *testing.T) {
	var wg sync.WaitGroup

	wg.Add(2)

	ts1 := NewTargetSet(&mockSyncer{
		sync: func([]*config.TargetGroup) { wg.Done() },
	})

	ts2 := NewTargetSet(&mockSyncer{
		sync: func([]*config.TargetGroup) { wg.Done() },
	})

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	updates := []update{
		{
			targetGroups: []config.TargetGroup{{Source: "initial1"}, {Source: "initial2"}},
			interval:     10,
		},
	}

	tp := newMockTargetProvider(updates)
	targetProviders := map[string]TargetProvider{}
	targetProviders["testProvider"] = tp

	go ts1.Run(ctx)
	go ts2.Run(ctx)

	ts1.UpdateProviders(targetProviders)
	ts2.UpdateProviders(targetProviders)

	finalize := make(chan struct{})
	go func() {
		defer close(finalize)
		wg.Wait()
	}()

	select {
	case <-time.After(20 * time.Second):
		t.Error("Test timed out after 20 seconds. All targets should be sent within the timeout")

	case <-finalize:
		if tp.callCount() != 2 {
			t.Errorf("Was expecting 2 calls, received %d", tp.callCount())
		}
	}
}

type mockSyncer struct {
	sync func(tgs []*config.TargetGroup)
}

func (s *mockSyncer) Sync(tgs []*config.TargetGroup) {
	if s.sync != nil {
		s.sync(tgs)
	}
}

type update struct {
	targetGroups []config.TargetGroup
	interval     time.Duration
}

type mockTargetProvider struct {
	_callCount *int32
	updates    []update
	up         chan<- []*config.TargetGroup
}

func newMockTargetProvider(updates []update) mockTargetProvider {
	var callCount int32

	tp := mockTargetProvider{
		_callCount: &callCount,
		updates:    updates,
	}

	return tp
}

func (tp mockTargetProvider) Run(ctx context.Context, up chan<- []*config.TargetGroup) {
	atomic.AddInt32(tp._callCount, 1)
	tp.up = up
	tp.sendUpdates()
}

func (tp mockTargetProvider) sendUpdates() {
	for _, update := range tp.updates {

		time.Sleep(update.interval * time.Millisecond)

		tgs := make([]*config.TargetGroup, len(update.targetGroups))
		for i := range update.targetGroups {
			tgs[i] = &update.targetGroups[i]
		}

		tp.up <- tgs
	}
}

func (tp mockTargetProvider) callCount() int {
	return int(atomic.LoadInt32(tp._callCount))
}
