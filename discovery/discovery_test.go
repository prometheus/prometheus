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

func TestSingleTargetSetWithSingleProviderOnlySendsNewTargetGroups(t *testing.T) {

	testCases := `
# No updates.
- []

# Empty initials.
- - target_groups: []
    interval: 5

# Empty initials with a delay.
- - target_groups: []
    interval: 6000

# Initials only.
- - target_groups:
    - source: initial1
      targets:
      - instance: 10.11.122.11:6001
    - source: initial2
      targets:
      - instance: 10.11.122.12:6001
    interval: 0

# Initials only but after a delay.
- - target_groups:
    - source: initial1
      targets:
      - instance: 10.11.122.11:6001
    - source: initial2
      targets:
      - instance: 10.11.122.12:6001
    interval: 6000

# Initials and new groups.
- - target_groups:
    - source: initial1
      targets:
      - instance: 10.11.122.11:6001
    - source: initial2
      targets:
      - instance: 10.11.122.12:6001
    interval: 0

  - target_groups:
    - source: update1
      targets:
      - instance: 10.11.122.13:6001
    - source: update2
      targets:
      - instance: 10.11.122.14:6001
    interval: 5

# Initials and new groups after a delay.
- - target_groups:
    - source: initial1
      targets:
      - instance: 10.11.122.11:6001
    - source: initial2
      targets:
      - instance: 10.11.122.12:6001
    interval: 6000

  - target_groups:
    - source: update1
      targets:
      - instance: 10.11.122.13:6001
    - source: update2
      targets:
      - instance: 10.11.122.14:6001
    interval: 500

# Next test case.
- - target_groups:
    - source: initial1
      targets:
      - instance: 10.11.122.11:6001
    interval: 100

  - target_groups:
    - source: update1
      targets:
      - instance: 10.11.122.12:6001
    interval: 100

  - target_groups:
    - source: update2
      targets:
      - instance: 10.11.122.13:6001
    interval: 100

# Next test case.
- - target_groups:
    - source: initial1
      targets:
      - instance: 10.11.122.11:6001
    - source: initial2
      targets:
      - instance: 10.11.122.12:6001
    interval: 100

  - target_groups:
    - source: update1
      targets:
      - instance: 10.11.122.13:6001
    - source: update2
      targets:
      - instance: 10.11.122.14:6001
    - source: update3
      targets:
      - instance: 10.11.122.15:6001
    - source: update4
      targets:
      - instance: 10.11.122.16:6001
    - source: update5
      targets:
      - instance: 10.11.122.17:6001
    - source: update6
      targets:
      - instance: 10.11.122.18:6001
    interval: 100

# Next test case.
- - target_groups:
    - source: initial1
      targets:
      - instance: 10.11.122.11:6001
    interval: 10

  - target_groups:
    - source: update1
      targets:
      - instance: 10.11.122.13:6001
    interval: 45

  - target_groups:
    - source: update2
      targets:
      - instance: 10.11.122.14:6001
    - source: update3
      targets:
      - instance: 10.11.122.15:6001
    - source: update4
      targets:
      - instance: 10.11.122.16:6001
    interval: 0

  - target_groups:
    - source: update5
      targets:
      - instance: 10.11.122.17:6001
    interval: 10

  - target_groups:
    - source: update6
      targets:
      - instance: 10.11.122.18:6001
    - source: update7
      targets:
      - instance: 10.11.122.19:6001
    - source: update8
      targets:
      - instance: 10.11.122.20:6001
    interval: 70

# Next test case.
- - target_groups:
    - source: initial1
      targets:
      - instance: 10.11.122.11:6001
    - source: initial2
      targets:
      - instance: 10.11.122.12:6001
    interval: 5

  - target_groups: []
    interval: 100

  - target_groups:
    - source: update1
      targets:
      - instance: 10.11.122.13:6001
    - source: update2
      targets:
      - instance: 10.11.122.14:6001
    interval: 100

  - target_groups:
    - source: update3
      targets:
      - instance: 10.11.122.15:6001
    - source: update4
      targets:
      - instance: 10.11.122.16:6001
    - source: update5
      targets:
      - instance: 10.11.122.17:6001
    interval: 70
`

	parsedTestCases := [][]update{}

	err := yaml.Unmarshal([]byte(testCases), &parsedTestCases)
	if err != nil {
		t.Fatalf("error while parsing test cases: %v", err)
	}

	for i, updates := range parsedTestCases {

		expectedGroups := make(map[string]struct{})
		for _, update := range updates {
			for _, target := range update.TargetGroups {
				expectedGroups[target.Source] = struct{}{}
			}
		}

		finalize := make(chan bool)

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
					finalize <- true
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

		select {
		case <-time.After(20000 * time.Millisecond):
			t.Errorf("In test case %v: Test timed out after 20000 millisecond. All targets should be sent within the timeout", i)

		case <-finalize:

			if *tp.callCount != 1 {
				t.Errorf("In test case %v: TargetProvider Run should be called once only, was called %v times", i, *tp.callCount)
			}

			if len(updates) > 0 && updates[0].Interval > 5000 {
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

func TestSingleTargetSetWithSingleProviderSendsUpdatedTargetGroups(t *testing.T) {

	testCases := `
# Update same initial group multiple times.
- - target_groups:
    - source: initial1
      targets:
      - instance: "10.11.122.11:6001"
    interval: 3

  - target_groups:
    - source: initial1
      targets:
      - instance: "10.11.122.11:6002"
    interval: 10

  - target_groups:
    - source: initial1
      targets:
      - instance: "10.11.122.11:6003"
    interval: 4

# Update second round target.
- - target_groups:
    - source: initial1
      targets:
      - instance: "10.11.122.11:6001"
    interval: 3

  - target_groups:
    - source: update1
      targets:
      - instance: "10.11.122.11:6002"
    interval: 10

  - target_groups:
    - source: update1
      targets:
      - instance: "10.11.122.11:6003"
      - instance: "10.11.122.11:6004"
      - instance: "10.11.122.11:6005"
    interval: 10

# Next test case.
- - target_groups:
    - source: initial1
      targets:
      - instance: "10.11.122.11:6001"
    - source: initial2
      targets:
      - instance: "10.11.122.12:6001"
    interval: 3

  - target_groups:
    - source: update1
      targets:
      - instance: "10.11.122.13:6001"
    - source: initial2
      targets:
      - instance: "10.11.123.12:6001"
      - instance: "10.11.123.13:6001"
    interval: 10

  - target_groups:
    - source: update2
      targets:
      - instance: "10.11.122.14:6001"
    - source: initial1
      targets: []
    interval: 10

  - target_groups:
    - source: update1
      targets:
      - instance: "10.11.122.13:6001"
      - instance: "10.11.122.15:6001"
      - instance: "10.11.122.16:6001"
    interval: 10

# Next test case.
- - target_groups:
    - source: initial1
      targets:
      - instance: "10.11.122.11:6001"
      - instance: "10.11.122.12:6001"
      - instance: "10.11.122.13:6001"
    - source: initial2
      targets:
      - instance: "10.11.122.14:6001"
    interval: 3

  - target_groups:
    - source: initial1
      targets:
      - instance: "10.11.122.13:6001"
    interval: 10

# Next test case.
- - target_groups:
    - source: initial1
      targets:
      - instance: "10.11.122.11:6001"
      - instance: "10.11.122.12:6001"
      - instance: "10.11.122.13:6001"
    - source: initial2
      targets:
      - instance: "10.11.122.14:6001"
    interval: 1000

  - target_groups:
    - source: initial1
      targets:
      - instance: "10.11.122.12:6001"
    - source: update1
      targets:
      - instance: "10.11.122.15:6001"
    interval: 3000

  - target_groups:
    - source: initial1
      targets: []
    - source: update1
      targets:
      - instance: "10.11.122.15:6001"
      - instance: "10.11.122.16:6001"
    interval: 3000
`

	parsedTestCases := [][]update{}
	err := yaml.Unmarshal([]byte(testCases), &parsedTestCases)
	if err != nil {
		t.Fatalf("error while parsing test cases: %v", err)
	}

	// Function to determine if this sync call received the latest state of
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

	for i, updates := range parsedTestCases {

		expectedGroups := make(map[string]config.TargetGroup)
		for _, update := range updates {
			for _, targetGroup := range update.TargetGroups {
				expectedGroups[targetGroup.Source] = targetGroup
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

				finalize <- true
			},
		})

		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		tp := newMockTargetProvider(updates)
		targetProviders := map[string]TargetProvider{}
		targetProviders["testProvider"] = tp

		go targetSet.Run(ctx)
		targetSet.UpdateProviders(targetProviders)

		select {
		case <-time.After(20000 * time.Millisecond):
			t.Errorf("In test case %v: Test timed out after 20000 millisecond. All targets should be sent within the timeout", i)

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

	updatesData := `
- target_groups:
  - source: initial1
    targets:
    - instance: 10.11.122.11:6001
    - instance: 10.11.122.11:6002
  interval: 5
`
	updates := []update{}
	err := yaml.Unmarshal([]byte(updatesData), &updates)
	if err != nil {
		t.Fatalf("error while parsing updates: %v", err)
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
	case <-time.After(20000 * time.Millisecond):
		t.Error("Test timed out after 20000 millisecond. All targets should be sent within the timeout")

	case <-finalize:
		if *tp.callCount != 2 {
			t.Errorf("Was expecting 2 calls received %v", tp.callCount)
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
	TargetGroups []config.TargetGroup `yaml:"target_groups"`
	Interval     int
}

// UnmarshalYAML implements the yaml.Unmarshaler interface.
func (u *update) UnmarshalYAML(unmarshal func(interface{}) error) error {

	s := struct {
		TargetGroups []struct {
			Source  string
			Targets []map[string]string
		} `yaml:"target_groups"`
		Interval int
	}{}

	if err := unmarshal(&s); err != nil {
		return err
	}

	u.Interval = s.Interval

	u.TargetGroups = make([]config.TargetGroup, len(s.TargetGroups))
	for groupIndex, targetGroup := range s.TargetGroups {

		u.TargetGroups[groupIndex] = config.TargetGroup{}
		u.TargetGroups[groupIndex].Source = targetGroup.Source

		u.TargetGroups[groupIndex].Targets = make([]model.LabelSet, len(targetGroup.Targets))
		for targetIndex, target := range targetGroup.Targets {

			u.TargetGroups[groupIndex].Targets[targetIndex] = make(model.LabelSet)
			for labelName, labelValue := range target {
				u.TargetGroups[groupIndex].Targets[targetIndex][model.LabelName(labelName)] = model.LabelValue(labelValue)
			}
		}
	}

	return nil
}

type mockTargetProvider struct {
	callCount *uint32
	updates   []update
	up        chan<- []*config.TargetGroup
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

		time.Sleep(time.Duration(update.Interval) * time.Millisecond)

		tgs := make([]*config.TargetGroup, len(update.TargetGroups))
		for groupIndex, _ := range update.TargetGroups {

			tgs[groupIndex] = &update.TargetGroups[groupIndex]
		}

		tp.up <- tgs
	}
}
