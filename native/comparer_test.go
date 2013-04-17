// Copyright 2013 Prometheus Team
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

package native

import (
	"code.google.com/p/goprotobuf/proto"
	"github.com/prometheus/prometheus/model"
	dto "github.com/prometheus/prometheus/model/generated"
	"testing"
	"time"
)

func TestLifecycle(t *testing.T) {
	comparator := NewSampleKeyComparator()
	if comparator == nil {
		t.Fatal("comparator == nil")
	}
	defer comparator.Close()
	if comparator.Comparator() == nil {
		t.Fatal("comparator.Comparator == nil")
	}
}

type side struct {
	fingerprint string
	time        time.Time
	sampleCount int
	lastTime    time.Time
}

func (s side) toDTO() (k *dto.SampleKey) {
	k = &dto.SampleKey{
		Fingerprint:   model.NewFingerprintFromRowKey(s.fingerprint).ToDTO(),
		Timestamp:     proto.Int64(s.time.Unix()),
		SampleCount:   proto.Uint32(uint32(s.sampleCount)),
		LastTimestamp: proto.Int64(s.lastTime.Unix()),
	}

	return
}

func TestFlow(t *testing.T) {
	instant := time.Now()

	var scenarios = []struct {
		left  side
		right side
		out   int
	}{
		{
			left: side{
				fingerprint: "1000-a-0-z",
				time:        instant,
				sampleCount: 1,
				lastTime:    instant,
			},
			right: side{
				fingerprint: "1000-a-0-z",
				time:        instant,
				sampleCount: 1,
				lastTime:    instant,
			},
			out: 0,
		},
		{
			left: side{
				fingerprint: "1000-a-0-z",
				time:        instant,
				sampleCount: 1,
				lastTime:    instant,
			},
			right: side{
				fingerprint: "1000-a-0-z",
				time:        instant,
				sampleCount: 1,
				lastTime:    instant.Add(time.Second),
			},
			out: 0,
		},
		{
			left: side{
				fingerprint: "1000-a-0-z",
				time:        instant,
				sampleCount: 1,
				lastTime:    instant,
			},
			right: side{
				fingerprint: "1000-a-0-z",
				time:        instant,
				sampleCount: 2,
				lastTime:    instant,
			},
			out: 0,
		},
		{
			left: side{
				fingerprint: "999-a-0-z",
				time:        instant,
				sampleCount: 1,
				lastTime:    instant,
			},
			right: side{
				fingerprint: "1000-a-0-z",
				time:        instant,
				sampleCount: 1,
				lastTime:    instant,
			},
			out: -1,
		},
		{
			left: side{
				fingerprint: "1000-a-0-z",
				time:        instant,
				sampleCount: 1,
				lastTime:    instant,
			},
			right: side{
				fingerprint: "999-a-0-z",
				time:        instant,
				sampleCount: 1,
				lastTime:    instant,
			},
			out: 1,
		},
		{
			left: side{
				fingerprint: "1000-a-0-z",
				time:        instant,
				sampleCount: 1,
				lastTime:    instant,
			},
			right: side{
				fingerprint: "1000-b-0-z",
				time:        instant,
				sampleCount: 1,
				lastTime:    instant,
			},
			out: -1,
		},
		{
			left: side{
				fingerprint: "1000-b-0-z",
				time:        instant,
				sampleCount: 1,
				lastTime:    instant,
			},
			right: side{
				fingerprint: "1000-a-0-z",
				time:        instant,
				sampleCount: 1,
				lastTime:    instant,
			},
			out: 1,
		},
		{
			left: side{
				fingerprint: "1000-a-0-z",
				time:        instant,
				sampleCount: 1,
				lastTime:    instant,
			},
			right: side{
				fingerprint: "1000-a-1-z",
				time:        instant,
				sampleCount: 1,
				lastTime:    instant,
			},
			out: -1,
		},
		{
			left: side{
				fingerprint: "1000-a-1-z",
				time:        instant,
				sampleCount: 1,
				lastTime:    instant,
			},
			right: side{
				fingerprint: "1000-a-0-z",
				time:        instant,
				sampleCount: 1,
				lastTime:    instant,
			},
			out: 1,
		},
		{
			left: side{
				fingerprint: "1000-a-0-y",
				time:        instant,
				sampleCount: 1,
				lastTime:    instant,
			},
			right: side{
				fingerprint: "1000-a-0-z",
				time:        instant,
				sampleCount: 1,
				lastTime:    instant,
			},
			out: -1,
		},
		{
			left: side{
				fingerprint: "1000-a-0-z",
				time:        instant,
				sampleCount: 1,
				lastTime:    instant,
			},
			right: side{
				fingerprint: "1000-a-0-y",
				time:        instant,
				sampleCount: 1,
				lastTime:    instant,
			},
			out: 1,
		},
		{
			left: side{
				fingerprint: "1000-a-0-z",
				time:        instant,
				sampleCount: 1,
				lastTime:    instant,
			},
			right: side{
				fingerprint: "1000-a-0-z",
				time:        instant.Add(time.Second),
				sampleCount: 1,
				lastTime:    instant,
			},
			out: -1,
		},
		{
			left: side{
				fingerprint: "1000-a-0-z",
				time:        instant.Add(time.Second),
				sampleCount: 1,
				lastTime:    instant,
			},
			right: side{
				fingerprint: "1000-a-0-z",
				time:        instant,
				sampleCount: 1,
				lastTime:    instant,
			},
			out: 1,
		},
	}

	for i, scenario := range scenarios {
		leftBuffer, err := proto.Marshal(scenario.left.toDTO())
		if err != nil {
			t.Fatal(err)
		}
		rightBuffer, err := proto.Marshal(scenario.right.toDTO())
		if err != nil {
			t.Fatal(err)
		}

		actual := compare(string(leftBuffer), len(leftBuffer), string(rightBuffer), len(rightBuffer))

		if actual != scenario.out {
			t.Fatalf("%d. for %s and %s expected %d, got %d", i, scenario.left, scenario.right, scenario.out, actual)
		}
	}
}
