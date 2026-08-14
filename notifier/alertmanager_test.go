// Copyright The Prometheus Authors
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

package notifier

import (
	"log/slog"
	"testing"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/common/model"
	"github.com/stretchr/testify/require"

	"github.com/prometheus/prometheus/config"
	"github.com/prometheus/prometheus/discovery/targetgroup"
)

func TestPostPath(t *testing.T) {
	cases := []struct {
		in, out string
	}{
		{
			in:  "",
			out: "/api/v2/alerts",
		},
		{
			in:  "/",
			out: "/api/v2/alerts",
		},
		{
			in:  "/prefix",
			out: "/prefix/api/v2/alerts",
		},
		{
			in:  "/prefix//",
			out: "/prefix/api/v2/alerts",
		},
		{
			in:  "prefix//",
			out: "/prefix/api/v2/alerts",
		},
	}
	for _, c := range cases {
		require.Equal(t, c.out, postPath(c.in, config.AlertmanagerAPIVersionV2))
	}
}

func TestLabelSetNotReused(t *testing.T) {
	tg := makeInputTargetGroup()
	_, _, err := AlertmanagerFromGroup(tg, &config.AlertmanagerConfig{})

	require.NoError(t, err)

	// Target modified during alertmanager extraction
	require.Equal(t, tg, makeInputTargetGroup())
}

// TestAlertmanagerSetSync verifies that sync properly manages sendloop lifecycle:
// - Starts sendloops for new alertmanagers.
// - Stops sendloops for removed alertmanagers.
// - Does NOT stop sendloops that are still in use.
// - Does NOT stop sendloops that were just created.
func TestAlertmanagerSetSync(t *testing.T) {
	reg := prometheus.NewRegistry()
	alertmanagersDiscoveredFunc := func() float64 { return 0 }
	metrics := newAlertMetrics(reg, alertmanagersDiscoveredFunc)
	logger := slog.New(slog.DiscardHandler)
	opts := &Options{QueueCapacity: 10, MaxBatchSize: DefaultMaxBatchSize}

	cfg := config.DefaultAlertmanagerConfig

	// Create alertmanagerSet
	ams, err := newAlertmanagerSet(&cfg, opts, logger, metrics)
	require.NoError(t, err)

	defer func() {
		ams.sync([]*targetgroup.Group{})
		require.Empty(t, ams.sendLoops, "All sendloops should be cleaned up")
	}()

	// First sync: Add AM1 and AM2
	tgs1 := []*targetgroup.Group{
		{
			Targets: []model.LabelSet{
				{model.AddressLabel: "am1.example.com:9093"},
				{model.AddressLabel: "am2.example.com:9093"},
			},
		},
	}

	ams.sync(tgs1)

	require.Len(t, ams.sendLoops, 2, "AM1 and AM2 sendloops should be created")
	require.Contains(t, ams.sendLoops, "http://am1.example.com:9093/api/v2/alerts", "AM1 sendloop should be created")
	require.Contains(t, ams.sendLoops, "http://am2.example.com:9093/api/v2/alerts", "AM2 sendloop should be created")

	am1Loop := ams.sendLoops["http://am1.example.com:9093/api/v2/alerts"]
	am2Loop := ams.sendLoops["http://am2.example.com:9093/api/v2/alerts"]
	require.NotNil(t, am1Loop)
	require.NotNil(t, am2Loop)

	// Second sync: Keep AM2, remove AM1, add AM3
	tgs2 := []*targetgroup.Group{
		{
			Targets: []model.LabelSet{
				{model.AddressLabel: "am2.example.com:9093"},
				{model.AddressLabel: "am3.example.com:9093"},
			},
		},
	}

	ams.sync(tgs2)

	require.Len(t, ams.sendLoops, 2)
	require.NotContains(t, ams.sendLoops, "http://am1.example.com:9093/api/v2/alerts", "AM1 sendloop should be removed")
	require.Contains(t, ams.sendLoops, "http://am2.example.com:9093/api/v2/alerts", "AM2 sendloop should be kept")
	require.Contains(t, ams.sendLoops, "http://am3.example.com:9093/api/v2/alerts", "AM3 sendloop should be created")

	am2LoopAfter := ams.sendLoops["http://am2.example.com:9093/api/v2/alerts"]
	require.Same(t, am2Loop, am2LoopAfter, "AM2 sendloop should not be recreated")

	am3Loop := ams.sendLoops["http://am3.example.com:9093/api/v2/alerts"]
	require.NotNil(t, am3Loop, "AM3 sendloop should be created")

	// Third sync: Keep only AM3, remove AM2
	tgs3 := []*targetgroup.Group{
		{
			Targets: []model.LabelSet{
				{model.AddressLabel: "am3.example.com:9093"},
			},
		},
	}

	ams.sync(tgs3)

	require.Len(t, ams.sendLoops, 1)
	require.NotContains(t, ams.sendLoops, "http://am2.example.com:9093/api/v2/alerts", "AM2 sendloop should be removed")
	require.Contains(t, ams.sendLoops, "http://am3.example.com:9093/api/v2/alerts", "AM3 sendloop should be kept")

	am3LoopAfter := ams.sendLoops["http://am3.example.com:9093/api/v2/alerts"]
	require.Same(t, am3Loop, am3LoopAfter, "AM3 sendloop should not be recreated")
}
