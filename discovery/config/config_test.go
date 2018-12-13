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

package config

import (
	"fmt"
	"reflect"
	"testing"

	yaml "gopkg.in/yaml.v2"

	"github.com/prometheus/prometheus/discovery/file"
)

func TestApplyConfigDoesNotModifyStaticProviderTargets(t *testing.T) {
	cfgText := `
static_configs:
 - targets: ["foo:9090"]
 - targets: ["bar:9090"]
 - targets: ["baz:9090"]
`
	originalConfig := ServiceDiscoveryConfig{}
	if err := yaml.UnmarshalStrict([]byte(cfgText), &originalConfig); err != nil {
		t.Fatalf("Unable to load YAML config cfgYaml: %s", err)
	}

	processedConfig := ServiceDiscoveryConfig{}
	if err := yaml.UnmarshalStrict([]byte(cfgText), &processedConfig); err != nil {
		t.Fatalf("Unable to load YAML config cfgYaml: %s", err)
	}

	pset := NewProviderSet("foo", nil)
	pset.Add("cfg", processedConfig)

	if len(pset.providers) != 1 {
		t.Fatalf("Invalid number of providers: expected %d, got %d", 1, len(pset.providers))
	}
	if !reflect.DeepEqual(originalConfig, processedConfig) {
		t.Fatalf("discovery manager modified static config \n  expected: %v\n  got: %v\n",
			originalConfig, processedConfig)
	}
}

// TestTargetSetRecreatesEmptyStaticConfigs ensures that reloading a config file after
// removing all targets from the static_configs sends an update with empty targetGroups.
// This is required to signal the receiver that this target set has no current targets.
func TestTargetSetRecreatesEmptyStaticConfigs(t *testing.T) {
	pset := NewProviderSet("foo", nil)
	pset.Add("config/0", ServiceDiscoveryConfig{})

	if len(pset.providers) != 1 {
		t.Fatalf("Invalid number of providers: expected %d, got %d", 1, len(pset.providers))
	}
	staticProvider, ok := pset.providers[0].Discoverer.(*StaticDiscoverer)
	if !ok {
		t.Fatalf("Expected *StaticDiscoverer")
	}
	if len(staticProvider.TargetGroups) != 1 {
		t.Fatalf("Expected 1 group, got %d", len(staticProvider.TargetGroups))
	}
	if len(staticProvider.TargetGroups[0].Targets) != 0 {
		t.Fatalf("Expected 0 targets, got %d", len(staticProvider.TargetGroups[0].Targets))
	}
}

func TestIdenticalConfigurationsAreCoalesced(t *testing.T) {
	cfg := []ServiceDiscoveryConfig{}
	cfgString := `
- file_sd_configs:
  - files: ["/etc/foo.json"]
  - files: ["/etc/bar.json"]
- file_sd_configs:
  - files: ["/etc/foo.json"]
`
	if err := yaml.UnmarshalStrict([]byte(cfgString), &cfg); err != nil {
		t.Fatalf("Unable to load YAML config sOne: %s", err)
	}

	pset := NewProviderSet("foo", nil)
	for i, v := range cfg {
		pset.Add(fmt.Sprintf("config/%d", i), v)
	}

	expected := []struct {
		files []string
		subs  []string
	}{
		{
			files: []string{"/etc/foo.json"},
			subs:  []string{"config/0", "config/1"},
		},
		{
			files: []string{"/etc/bar.json"},
			subs:  []string{"config/0"},
		},
	}
	if len(pset.providers) != len(expected) {
		t.Fatalf("Invalid number of providers: expected %d, got %d", len(expected), len(pset.providers))
	}
	for i, exp := range expected {
		prov := pset.providers[i]

		if !reflect.DeepEqual(exp.subs, prov.Subscribers()) {
			t.Fatalf("Expected %v subscribers, got %v", exp.subs, prov.Subscribers())
		}

		fileCfg, ok := prov.config.(*file.SDConfig)
		if !ok {
			t.Fatalf("Expected *file.SDConfig configuration")
		}
		if !reflect.DeepEqual(exp.files, fileCfg.Files) {
			t.Fatalf("Expected %v files, got %v", exp.files, fileCfg.Files)
		}
	}
}
