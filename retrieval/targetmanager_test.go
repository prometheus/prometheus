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
	"testing"

	"golang.org/x/net/context"
	"gopkg.in/yaml.v2"

	"github.com/prometheus/prometheus/config"
	"github.com/prometheus/prometheus/storage/local"
)

func TestTargetSetRecreatesTargetGroupsEveryRun(t *testing.T) {

	verifyPresence := func(tgroups map[string][]*Target, name string, present bool) {
		if _, ok := tgroups[name]; ok != present {
			msg := ""
			if !present {
				msg = "not "
			}
			t.Fatalf("'%s' should %sbe present in TargetSet.tgroups: %s", name, msg, tgroups)
		}

	}

	scrapeConfig := &config.ScrapeConfig{}

	sOne := `
job_name: "foo"
dns_sd_configs:
- names:
  - "srv.name.one.example.org"
`
	if err := yaml.Unmarshal([]byte(sOne), scrapeConfig); err != nil {
		t.Fatalf("Unable to load YAML config sOne: %s", err)
	}

	// Not properly setting it up, but that seems okay
	mss := &local.MemorySeriesStorage{}

	ts := newTargetSet(scrapeConfig, mss)

	ts.runProviders(context.Background(), providersFromConfig(scrapeConfig))

	verifyPresence(ts.tgroups, "dns/0/srv.name.one.example.org", true)

	sTwo := `
job_name: "foo"
dns_sd_configs:
- names:
  - "srv.name.two.example.org"
`
	if err := yaml.Unmarshal([]byte(sTwo), scrapeConfig); err != nil {
		t.Fatalf("Unable to load YAML config sTwo: %s", err)
	}

	ts.runProviders(context.Background(), providersFromConfig(scrapeConfig))

	verifyPresence(ts.tgroups, "dns/0/srv.name.one.example.org", false)
	verifyPresence(ts.tgroups, "dns/0/srv.name.two.example.org", true)
}
