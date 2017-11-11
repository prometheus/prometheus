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
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/prometheus/prometheus/config"
	yaml "gopkg.in/yaml.v2"
)

func TestSingleTargetSetWithSingleProviderOnlySendsNewTargetGroups(t *testing.T) {

	testCases := [][]update{
		[]update{}, // No updates.

		[]update{{[]string{}, 0}}, // Empty initials.

		[]update{{[]string{}, 6000}}, // Empty initials with a delay.

		[]update{{[]string{"initial1", "initial2"}, 0}}, // Initials only.

		[]update{{[]string{"initial1", "initial2"}, 6000}}, // Initials only but after a delay.

		[]update{ // Initials and new groups.
			{[]string{"initial1", "initial2"}, 0},
			{[]string{"update1", "update2"}, 0},
		},

		[]update{ // Initials and new groups after a delay.
			{[]string{"initial1", "initial2"}, 6000},
			{[]string{"update1", "update2"}, 500},
		},

		[]update{
			{[]string{"initial1", "initial2"}, 100},
			{[]string{"update1", "update2", "update3", "update4", "update5", "update6", "update7", "update8", "update9", "update10", "update11"}, 100},
		},

		[]update{
			{[]string{"initial1"}, 10},
			{[]string{"update1"}, 45},
			{[]string{"update2", "update3", "update4"}, 0},
			{[]string{"update5"}, 10},
			{[]string{"update6", "update7", "update8", "update9"}, 70},
		},

		[]update{
			{[]string{"initial1", "initial2"}, 5},
			{[]string{}, 100},
			{[]string{"update1", "update2"}, 100},
			{[]string{"update3", "update4", "update5"}, 70},
		},
	}

	for i, updates := range testCases {

		expectedGroups := make(map[string]struct{})
		for _, update := range updates {
			for _, target := range update.targets {
				expectedGroups[target] = struct{}{}
			}
		}

		var wg sync.WaitGroup
		wg.Add(1)

		isFirstSyncCall := true
		var initialGroups []*config.TargetGroup
		var syncedGroups []*config.TargetGroup

		targetSet := NewTargetSet(&mockSyncer{
			sync: func(tgs []*config.TargetGroup) {
				syncedGroups = tgs

				if isFirstSyncCall {
					isFirstSyncCall = false
					initialGroups = tgs
				}

				if len(tgs) == len(expectedGroups) {
					// All the groups are sent, we can start asserting.
					wg.Done()
				}
			},
		})

		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		tp := newMockTargetProvider(updates)
		targetProviders := map[string]TargetProvider{}
		targetProviders["testProvider"] = tp

		go targetSet.Run(ctx)
		targetSet.UpdateProviders(targetProviders)

		finalize := make(chan struct{})
		go func() {
			defer close(finalize)
			wg.Wait()
		}()

		select {
		case <-time.After(20000 * time.Millisecond):
			t.Errorf("In test case %v: Test timed out after 20000 millisecond. All targets should be sent within the timeout", i)

		case <-finalize:

			if *tp.callCount != 1 {
				t.Errorf("In test case %v: TargetProvider Run should be called once only, was called %v times", i, *tp.callCount)
			}

			if len(updates) > 0 && updates[0].interval > 5000 {
				// If the initial set of targets never arrive or arrive after 5 seconds.
				// The first sync call should receive empty set of targets.
				if len(initialGroups) != 0 {
					t.Errorf("In test case %v: Expecting 0 initial target groups, received %v", i, len(initialGroups))
				}
			}

			if len(syncedGroups) != len(expectedGroups) {
				t.Errorf("In test case %v: Expecting %v target groups in total, received %v", i, len(expectedGroups), len(syncedGroups))
			}

			for _, tg := range syncedGroups {
				if _, ok := expectedGroups[tg.Source]; ok == false {
					t.Errorf("In test case %v: '%s' does not exist in expected target groups: %s", i, tg.Source, expectedGroups)
				} else {
					delete(expectedGroups, tg.Source) // Remove used targets from the map.
				}
			}
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

	tp := newMockTargetProvider([]update{{[]string{"initial1", "initial2"}, 10}})
	targetProviders := map[string]TargetProvider{}
	targetProviders["testProvider"] = tp

	go ts1.Run(ctx)
	go ts2.Run(ctx)

	ts1.UpdateProviders(targetProviders)
	ts2.UpdateProviders(targetProviders)
	wg.Wait()

	if *tp.callCount != 2 {
		t.Errorf("Was expecting 2 calls received %v", tp.callCount)
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

type mockTargetProvider struct {
	callCount *uint32
	updates   []update
	up        chan<- []*config.TargetGroup
}

type update struct {
	targets  []string
	interval time.Duration
}

func newMockTargetProvider(updates []update) mockTargetProvider {
	var callCount uint32

	tp := mockTargetProvider{
		callCount: &callCount,
		updates:   updates,
	}

	return tp
}

func (tp mockTargetProvider) Run(ctx context.Context, up chan<- []*config.TargetGroup) {
	atomic.AddUint32(tp.callCount, 1)
	tp.up = up
	tp.sendUpdates()
}

func (tp mockTargetProvider) sendUpdates() {
	for _, update := range tp.updates {

		time.Sleep(update.interval * time.Millisecond)

		tgs := make([]*config.TargetGroup, len(update.targets))
		for i, tg := range update.targets {
			tgs[i] = &config.TargetGroup{Source: tg}
		}

		tp.up <- tgs
	}
}
