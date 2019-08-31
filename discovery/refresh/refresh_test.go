// Copyright 2019 The Prometheus Authors
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
	"fmt"
	"testing"
	"time"

	"github.com/prometheus/common/model"

	"github.com/prometheus/prometheus/discovery/targetgroup"
	"github.com/prometheus/prometheus/util/testutil"
)

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
	refresh := func(ctx context.Context) ([]*targetgroup.Group, error) {
		i++
		switch i {
		case 1:
			return tg1, nil
		case 2:
			return tg2, nil
		}
		return nil, fmt.Errorf("some error")
	}
	interval := time.Millisecond
	d := NewDiscovery(nil, "test", interval, refresh)

	ch := make(chan []*targetgroup.Group)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go d.Run(ctx, ch)

	tg := <-ch
	testutil.Equals(t, tg1, tg)

	tg = <-ch
	testutil.Equals(t, tg2, tg)

	tick := time.NewTicker(2 * interval)
	defer tick.Stop()
	select {
	case <-ch:
		t.Fatal("Unexpected target group")
	case <-tick.C:
	}
}
