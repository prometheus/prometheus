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
	"net/url"
	"reflect"
	"regexp"
	"testing"
	"time"

	clientmodel "github.com/prometheus/client_golang/model"

	"github.com/prometheus/prometheus/config"
)

func TestTargetManagerChan(t *testing.T) {
	testJob1 := &config.ScrapeConfig{
		JobName:        "test_job1",
		ScrapeInterval: config.Duration(1 * time.Minute),
		TargetGroups: []*config.TargetGroup{{
			Targets: []clientmodel.LabelSet{
				{clientmodel.AddressLabel: "example.org:80"},
				{clientmodel.AddressLabel: "example.com:80"},
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
		expected map[string][]clientmodel.LabelSet
	}{
		{
			tgroup: &config.TargetGroup{
				Source: "src1",
				Targets: []clientmodel.LabelSet{
					{clientmodel.AddressLabel: "test-1:1234"},
					{clientmodel.AddressLabel: "test-2:1234", "label": "set"},
					{clientmodel.AddressLabel: "test-3:1234"},
				},
			},
			expected: map[string][]clientmodel.LabelSet{
				"test_job1:src1": {
					{clientmodel.JobLabel: "test_job1", clientmodel.InstanceLabel: "test-1:1234"},
					{clientmodel.JobLabel: "test_job1", clientmodel.InstanceLabel: "test-2:1234", "label": "set"},
					{clientmodel.JobLabel: "test_job1", clientmodel.InstanceLabel: "test-3:1234"},
				},
			},
		}, {
			tgroup: &config.TargetGroup{
				Source: "src2",
				Targets: []clientmodel.LabelSet{
					{clientmodel.AddressLabel: "test-1:1235"},
					{clientmodel.AddressLabel: "test-2:1235"},
					{clientmodel.AddressLabel: "test-3:1235"},
				},
				Labels: clientmodel.LabelSet{"group": "label"},
			},
			expected: map[string][]clientmodel.LabelSet{
				"test_job1:src1": {
					{clientmodel.JobLabel: "test_job1", clientmodel.InstanceLabel: "test-1:1234"},
					{clientmodel.JobLabel: "test_job1", clientmodel.InstanceLabel: "test-2:1234", "label": "set"},
					{clientmodel.JobLabel: "test_job1", clientmodel.InstanceLabel: "test-3:1234"},
				},
				"test_job1:src2": {
					{clientmodel.JobLabel: "test_job1", clientmodel.InstanceLabel: "test-1:1235", "group": "label"},
					{clientmodel.JobLabel: "test_job1", clientmodel.InstanceLabel: "test-2:1235", "group": "label"},
					{clientmodel.JobLabel: "test_job1", clientmodel.InstanceLabel: "test-3:1235", "group": "label"},
				},
			},
		}, {
			tgroup: &config.TargetGroup{
				Source:  "src2",
				Targets: []clientmodel.LabelSet{},
			},
			expected: map[string][]clientmodel.LabelSet{
				"test_job1:src1": {
					{clientmodel.JobLabel: "test_job1", clientmodel.InstanceLabel: "test-1:1234"},
					{clientmodel.JobLabel: "test_job1", clientmodel.InstanceLabel: "test-2:1234", "label": "set"},
					{clientmodel.JobLabel: "test_job1", clientmodel.InstanceLabel: "test-3:1234"},
				},
			},
		}, {
			tgroup: &config.TargetGroup{
				Source: "src1",
				Targets: []clientmodel.LabelSet{
					{clientmodel.AddressLabel: "test-1:1234", "added": "label"},
					{clientmodel.AddressLabel: "test-3:1234"},
					{clientmodel.AddressLabel: "test-4:1234", "fancy": "label"},
				},
			},
			expected: map[string][]clientmodel.LabelSet{
				"test_job1:src1": {
					{clientmodel.JobLabel: "test_job1", clientmodel.InstanceLabel: "test-1:1234", "added": "label"},
					{clientmodel.JobLabel: "test_job1", clientmodel.InstanceLabel: "test-3:1234"},
					{clientmodel.JobLabel: "test_job1", clientmodel.InstanceLabel: "test-4:1234", "fancy": "label"},
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
		Params: url.Values{
			"testParam": []string{"paramValue", "secondValue"},
		},
		TargetGroups: []*config.TargetGroup{{
			Targets: []clientmodel.LabelSet{
				{clientmodel.AddressLabel: "example.org:80"},
				{clientmodel.AddressLabel: "example.com:80"},
			},
		}},
		RelabelConfigs: []*config.RelabelConfig{
			{
				// Copy out the URL parameter.
				SourceLabels: clientmodel.LabelNames{"__param_testParam"},
				Regex:        &config.Regexp{*regexp.MustCompile("^(.*)$")},
				TargetLabel:  "testParam",
				Replacement:  "$1",
				Action:       config.RelabelReplace,
			},
		},
	}
	testJob2 := &config.ScrapeConfig{
		JobName:        "test_job2",
		ScrapeInterval: config.Duration(1 * time.Minute),
		TargetGroups: []*config.TargetGroup{
			{
				Targets: []clientmodel.LabelSet{
					{clientmodel.AddressLabel: "example.org:8080"},
					{clientmodel.AddressLabel: "example.com:8081"},
				},
				Labels: clientmodel.LabelSet{
					"foo":  "bar",
					"boom": "box",
				},
			},
			{
				Targets: []clientmodel.LabelSet{
					{clientmodel.AddressLabel: "test.com:1234"},
				},
			},
			{
				Targets: []clientmodel.LabelSet{
					{clientmodel.AddressLabel: "test.com:1235"},
				},
				Labels: clientmodel.LabelSet{"instance": "fixed"},
			},
		},
		RelabelConfigs: []*config.RelabelConfig{
			{
				SourceLabels: clientmodel.LabelNames{clientmodel.AddressLabel},
				Regex:        &config.Regexp{*regexp.MustCompile(`^test\.(.*?):(.*)`)},
				Replacement:  "foo.${1}:${2}",
				TargetLabel:  clientmodel.AddressLabel,
				Action:       config.RelabelReplace,
			},
			{
				// Add a new label for example.* targets.
				SourceLabels: clientmodel.LabelNames{clientmodel.AddressLabel, "boom", "foo"},
				Regex:        &config.Regexp{*regexp.MustCompile("^example.*?-b([a-z-]+)r$")},
				TargetLabel:  "new",
				Replacement:  "$1",
				Separator:    "-",
				Action:       config.RelabelReplace,
			},
			{
				// Drop an existing label.
				SourceLabels: clientmodel.LabelNames{"boom"},
				Regex:        &config.Regexp{*regexp.MustCompile(".*")},
				TargetLabel:  "boom",
				Replacement:  "",
				Action:       config.RelabelReplace,
			},
		},
	}

	sequence := []struct {
		scrapeConfigs []*config.ScrapeConfig
		expected      map[string][]clientmodel.LabelSet
	}{
		{
			scrapeConfigs: []*config.ScrapeConfig{testJob1},
			expected: map[string][]clientmodel.LabelSet{
				"test_job1:static:0": {
					{clientmodel.JobLabel: "test_job1", clientmodel.InstanceLabel: "example.org:80", "testParam": "paramValue"},
					{clientmodel.JobLabel: "test_job1", clientmodel.InstanceLabel: "example.com:80", "testParam": "paramValue"},
				},
			},
		}, {
			scrapeConfigs: []*config.ScrapeConfig{testJob1},
			expected: map[string][]clientmodel.LabelSet{
				"test_job1:static:0": {
					{clientmodel.JobLabel: "test_job1", clientmodel.InstanceLabel: "example.org:80", "testParam": "paramValue"},
					{clientmodel.JobLabel: "test_job1", clientmodel.InstanceLabel: "example.com:80", "testParam": "paramValue"},
				},
			},
		}, {
			scrapeConfigs: []*config.ScrapeConfig{testJob1, testJob2},
			expected: map[string][]clientmodel.LabelSet{
				"test_job1:static:0": {
					{clientmodel.JobLabel: "test_job1", clientmodel.InstanceLabel: "example.org:80", "testParam": "paramValue"},
					{clientmodel.JobLabel: "test_job1", clientmodel.InstanceLabel: "example.com:80", "testParam": "paramValue"},
				},
				"test_job2:static:0": {
					{clientmodel.JobLabel: "test_job2", clientmodel.InstanceLabel: "example.org:8080", "foo": "bar", "new": "ox-ba"},
					{clientmodel.JobLabel: "test_job2", clientmodel.InstanceLabel: "example.com:8081", "foo": "bar", "new": "ox-ba"},
				},
				"test_job2:static:1": {
					{clientmodel.JobLabel: "test_job2", clientmodel.InstanceLabel: "foo.com:1234"},
				},
				"test_job2:static:2": {
					{clientmodel.JobLabel: "test_job2", clientmodel.InstanceLabel: "fixed"},
				},
			},
		}, {
			scrapeConfigs: []*config.ScrapeConfig{},
			expected:      map[string][]clientmodel.LabelSet{},
		}, {
			scrapeConfigs: []*config.ScrapeConfig{testJob2},
			expected: map[string][]clientmodel.LabelSet{
				"test_job2:static:0": {
					{clientmodel.JobLabel: "test_job2", clientmodel.InstanceLabel: "example.org:8080", "foo": "bar", "new": "ox-ba"},
					{clientmodel.JobLabel: "test_job2", clientmodel.InstanceLabel: "example.com:8081", "foo": "bar", "new": "ox-ba"},
				},
				"test_job2:static:1": {
					{clientmodel.JobLabel: "test_job2", clientmodel.InstanceLabel: "foo.com:1234"},
				},
				"test_job2:static:2": {
					{clientmodel.JobLabel: "test_job2", clientmodel.InstanceLabel: "fixed"},
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
