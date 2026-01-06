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

package refresh

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/common/model"
	"github.com/stretchr/testify/require"
	"go.uber.org/goleak"

	"github.com/prometheus/prometheus/discovery"
	"github.com/prometheus/prometheus/discovery/targetgroup"
)

func TestMain(m *testing.M) {
	goleak.VerifyTestMain(m)
}

func TestRefresh(t *testing.T) {
	tg1 := []*targetgroup.Group{
		{
			Source: "tg",
			Targets: []model.LabelSet{
				{
					model.LabelName("t1"): model.LabelValue("v1"),
				},
				{
					model.LabelName("t2"): model.LabelValue("v2"),
				},
			},
			Labels: model.LabelSet{
				model.LabelName("l1"): model.LabelValue("lv1"),
			},
		},
	}
	tg2 := []*targetgroup.Group{
		{
			Source: "tg",
		},
	}

	var i int
	refresh := func(context.Context) ([]*targetgroup.Group, error) {
		i++
		switch i {
		case 1:
			return tg1, nil
		case 2:
			return tg2, nil
		}
		return nil, errors.New("some error")
	}
	interval := time.Millisecond

	metrics := discovery.NewRefreshMetrics(prometheus.NewRegistry())
	require.NoError(t, metrics.Register())
	defer metrics.Unregister()

	d := NewDiscovery(
		Options{
			Logger:              nil,
			Mech:                "test",
			SetName:             "test-refresh",
			Interval:            interval,
			RefreshF:            refresh,
			MetricsInstantiator: metrics,
		},
	)

	ch := make(chan []*targetgroup.Group)
	ctx := t.Context()
	go d.Run(ctx, ch)

	tg := <-ch
	require.Equal(t, tg1, tg)

	tg = <-ch
	require.Equal(t, tg2, tg)

	tick := time.NewTicker(2 * interval)
	defer tick.Stop()
	select {
	case <-ch:
		require.FailNow(t, "Unexpected target group")
	case <-tick.C:
	}
}
