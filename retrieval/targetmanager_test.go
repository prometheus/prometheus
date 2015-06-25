// Copyright 2013 The Prometheus Authors
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

package retrieval

import (
	"reflect"
	"regexp"
	"testing"
	"time"

	"github.com/prometheus/common/model"

	"github.com/prometheus/prometheus/config"
)

func TestTargetManagerChan(t *testing.T) {
	testJob1 := &config.ScrapeConfig{
		JobName:        "test_job1",
		ScrapeInterval: config.Duration(1 * time.Minute),
		TargetGroups: []*config.TargetGroup{{
			Targets: []model.LabelSet{
				{model.AddressLabel: "example.org:80"},
				{model.AddressLabel: "example.com:80"},
			},
		}},
	}
	prov1 := &fakeTargetProvider{
		sources: []string{"src1", "src2"},
		update:  make(chan *config.TargetGroup),
	}

	targetManager := &TargetManager{
		sampleAppender: nopAppender{},
		providers: map[*config.ScrapeConfig][]TargetProvider{
			testJob1: {prov1},
		},
		targets: make(map[string][]*Target),
	}
	go targetManager.Run()
	defer targetManager.Stop()

	sequence := []struct {
		tgroup   *config.TargetGroup
		expected map[string][]model.LabelSet
	}{
		{
			tgroup: &config.TargetGroup{
				Source: "src1",
				Targets: []model.LabelSet{
					{model.AddressLabel: "test-1:1234"},
					{model.AddressLabel: "test-2:1234", "label": "set"},
					{model.AddressLabel: "test-3:1234"},
				},
			},
			expected: map[string][]model.LabelSet{
				"test_job1:src1": {
					{model.JobLabel: "test_job1", model.InstanceLabel: "test-1:1234"},
					{model.JobLabel: "test_job1", model.InstanceLabel: "test-2:1234", "label": "set"},
					{model.JobLabel: "test_job1", model.InstanceLabel: "test-3:1234"},
				},
			},
		}, {
			tgroup: &config.TargetGroup{
				Source: "src2",
				Targets: []model.LabelSet{
					{model.AddressLabel: "test-1:1235"},
					{model.AddressLabel: "test-2:1235"},
					{model.AddressLabel: "test-3:1235"},
				},
				Labels: model.LabelSet{"group": "label"},
			},
			expected: map[string][]model.LabelSet{
				"test_job1:src1": {
					{model.JobLabel: "test_job1", model.InstanceLabel: "test-1:1234"},
					{model.JobLabel: "test_job1", model.InstanceLabel: "test-2:1234", "label": "set"},
					{model.JobLabel: "test_job1", model.InstanceLabel: "test-3:1234"},
				},
				"test_job1:src2": {
					{model.JobLabel: "test_job1", model.InstanceLabel: "test-1:1235", "group": "label"},
					{model.JobLabel: "test_job1", model.InstanceLabel: "test-2:1235", "group": "label"},
					{model.JobLabel: "test_job1", model.InstanceLabel: "test-3:1235", "group": "label"},
				},
			},
		}, {
			tgroup: &config.TargetGroup{
				Source:  "src2",
				Targets: []model.LabelSet{},
			},
			expected: map[string][]model.LabelSet{
				"test_job1:src1": {
					{model.JobLabel: "test_job1", model.InstanceLabel: "test-1:1234"},
					{model.JobLabel: "test_job1", model.InstanceLabel: "test-2:1234", "label": "set"},
					{model.JobLabel: "test_job1", model.InstanceLabel: "test-3:1234"},
				},
			},
		}, {
			tgroup: &config.TargetGroup{
				Source: "src1",
				Targets: []model.LabelSet{
					{model.AddressLabel: "test-1:1234", "added": "label"},
					{model.AddressLabel: "test-3:1234"},
					{model.AddressLabel: "test-4:1234", "fancy": "label"},
				},
			},
			expected: map[string][]model.LabelSet{
				"test_job1:src1": {
					{model.JobLabel: "test_job1", model.InstanceLabel: "test-1:1234", "added": "label"},
					{model.JobLabel: "test_job1", model.InstanceLabel: "test-3:1234"},
					{model.JobLabel: "test_job1", model.InstanceLabel: "test-4:1234", "fancy": "label"},
				},
			},
		},
	}

	for i, step := range sequence {
		prov1.update <- step.tgroup

		<-time.After(1 * time.Millisecond)

		if len(targetManager.targets) != len(step.expected) {
			t.Fatalf("step %d: sources mismatch %v, %v", targetManager.targets, step.expected)
		}

		for source, actTargets := range targetManager.targets {
			expTargets, ok := step.expected[source]
			if !ok {
				t.Fatalf("step %d: unexpected source %q: %v", i, source, actTargets)
			}
			for _, expt := range expTargets {
				found := false
				for _, actt := range actTargets {
					if reflect.DeepEqual(expt, actt.BaseLabels()) {
						found = true
						break
					}
				}
				if !found {
					t.Errorf("step %d: expected target %v not found in actual targets", i, expt)
				}
			}
		}
	}
}

func TestTargetManagerConfigUpdate(t *testing.T) {
	testJob1 := &config.ScrapeConfig{
		JobName:        "test_job1",
		ScrapeInterval: config.Duration(1 * time.Minute),
		TargetGroups: []*config.TargetGroup{{
			Targets: []model.LabelSet{
				{model.AddressLabel: "example.org:80"},
				{model.AddressLabel: "example.com:80"},
			},
		}},
	}
	testJob2 := &config.ScrapeConfig{
		JobName:        "test_job2",
		ScrapeInterval: config.Duration(1 * time.Minute),
		TargetGroups: []*config.TargetGroup{
			{
				Targets: []model.LabelSet{
					{model.AddressLabel: "example.org:8080"},
					{model.AddressLabel: "example.com:8081"},
				},
				Labels: model.LabelSet{
					"foo":  "bar",
					"boom": "box",
				},
			},
			{
				Targets: []model.LabelSet{
					{model.AddressLabel: "test.com:1234"},
				},
			},
			{
				Targets: []model.LabelSet{
					{model.AddressLabel: "test.com:1235"},
				},
				Labels: model.LabelSet{"instance": "fixed"},
			},
		},
		RelabelConfigs: []*config.RelabelConfig{
			{
				SourceLabels: model.LabelNames{model.AddressLabel},
				Regex:        &config.Regexp{*regexp.MustCompile(`^test\.(.*?):(.*)`)},
				Replacement:  "foo.${1}:${2}",
				TargetLabel:  model.AddressLabel,
				Action:       config.RelabelReplace,
			},
			{
				// Add a new label for example.* targets.
				SourceLabels: model.LabelNames{model.AddressLabel, "boom", "foo"},
				Regex:        &config.Regexp{*regexp.MustCompile("^example.*?-b([a-z-]+)r$")},
				TargetLabel:  "new",
				Replacement:  "$1",
				Separator:    "-",
				Action:       config.RelabelReplace,
			},
			{
				// Drop an existing label.
				SourceLabels: model.LabelNames{"boom"},
				Regex:        &config.Regexp{*regexp.MustCompile(".*")},
				TargetLabel:  "boom",
				Replacement:  "",
				Action:       config.RelabelReplace,
			},
		},
	}

	sequence := []struct {
		scrapeConfigs []*config.ScrapeConfig
		expected      map[string][]model.LabelSet
	}{
		{
			scrapeConfigs: []*config.ScrapeConfig{testJob1},
			expected: map[string][]model.LabelSet{
				"test_job1:static:0": {
					{model.JobLabel: "test_job1", model.InstanceLabel: "example.org:80"},
					{model.JobLabel: "test_job1", model.InstanceLabel: "example.com:80"},
				},
			},
		}, {
			scrapeConfigs: []*config.ScrapeConfig{testJob1},
			expected: map[string][]model.LabelSet{
				"test_job1:static:0": {
					{model.JobLabel: "test_job1", model.InstanceLabel: "example.org:80"},
					{model.JobLabel: "test_job1", model.InstanceLabel: "example.com:80"},
				},
			},
		}, {
			scrapeConfigs: []*config.ScrapeConfig{testJob1, testJob2},
			expected: map[string][]model.LabelSet{
				"test_job1:static:0": {
					{model.JobLabel: "test_job1", model.InstanceLabel: "example.org:80"},
					{model.JobLabel: "test_job1", model.InstanceLabel: "example.com:80"},
				},
				"test_job2:static:0": {
					{model.JobLabel: "test_job2", model.InstanceLabel: "example.org:8080", "foo": "bar", "new": "ox-ba"},
					{model.JobLabel: "test_job2", model.InstanceLabel: "example.com:8081", "foo": "bar", "new": "ox-ba"},
				},
				"test_job2:static:1": {
					{model.JobLabel: "test_job2", model.InstanceLabel: "foo.com:1234"},
				},
				"test_job2:static:2": {
					{model.JobLabel: "test_job2", model.InstanceLabel: "fixed"},
				},
			},
		}, {
			scrapeConfigs: []*config.ScrapeConfig{},
			expected:      map[string][]model.LabelSet{},
		}, {
			scrapeConfigs: []*config.ScrapeConfig{testJob2},
			expected: map[string][]model.LabelSet{
				"test_job2:static:0": {
					{model.JobLabel: "test_job2", model.InstanceLabel: "example.org:8080", "foo": "bar", "new": "ox-ba"},
					{model.JobLabel: "test_job2", model.InstanceLabel: "example.com:8081", "foo": "bar", "new": "ox-ba"},
				},
				"test_job2:static:1": {
					{model.JobLabel: "test_job2", model.InstanceLabel: "foo.com:1234"},
				},
				"test_job2:static:2": {
					{model.JobLabel: "test_job2", model.InstanceLabel: "fixed"},
				},
			},
		},
	}
	conf := &config.Config{}
	*conf = config.DefaultConfig

	targetManager := NewTargetManager(nopAppender{})
	targetManager.ApplyConfig(conf)

	targetManager.Run()
	defer targetManager.Stop()

	for i, step := range sequence {
		conf.ScrapeConfigs = step.scrapeConfigs
		targetManager.ApplyConfig(conf)

		<-time.After(1 * time.Millisecond)

		if len(targetManager.targets) != len(step.expected) {
			t.Fatalf("step %d: sources mismatch %v, %v", targetManager.targets, step.expected)
		}

		for source, actTargets := range targetManager.targets {
			expTargets, ok := step.expected[source]
			if !ok {
				t.Fatalf("step %d: unexpected source %q: %v", i, source, actTargets)
			}
			for _, expt := range expTargets {
				found := false
				for _, actt := range actTargets {
					if reflect.DeepEqual(expt, actt.BaseLabels()) {
						found = true
						break
					}
				}
				if !found {
					t.Errorf("step %d: expected target %v for %q not found in actual targets", i, expt, source)
				}
			}
		}
	}
}
