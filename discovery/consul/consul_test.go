// Copyright 2015 The Prometheus Authors
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

package consul

import (
	"testing"

	"github.com/prometheus/prometheus/config"
	"github.com/prometheus/prometheus/util/testutil"
)

func TestConfiguredService(t *testing.T) {
	conf := &config.ConsulSDConfig{
		Services: []string{"configuredServiceName"}}
	consulDiscovery, err := NewDiscovery(conf, nil)

	testutil.Ok(t, err)

	testutil.Assert(
		t,
		consulDiscovery.shouldWatch("configuredServiceName"),
		"Expected service configuredServiceName to be watched",
	)

	testutil.Assert(
		t,
		!consulDiscovery.shouldWatch("nonConfiguredServiceName"),
		"Expected service nonConfiguredServiceName to not be watched",
	)
}

func TestNonConfiguredService(t *testing.T) {
	conf := &config.ConsulSDConfig{}
	consulDiscovery, err := NewDiscovery(conf, nil)

	testutil.Ok(t, err)

	testutil.Assert(
		t,
		consulDiscovery.shouldWatch("nonConfiguredServiceName"),
		"Expected service nonConfiguredServiceName to not be watched",
	)
}
