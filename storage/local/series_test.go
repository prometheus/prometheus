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

package local

import (
	"testing"
	"time"

	"github.com/prometheus/common/model"
)

func TestDropChunks(t *testing.T) {
	s, err := newMemorySeries(nil, nil, time.Time{})
	if err != nil {
		t.Fatal(err)
	}

	s.add(model.SamplePair{
		Timestamp: 100,
		Value:     42,
	})
	s.add(model.SamplePair{
		Timestamp: 110,
		Value:     4711,
	})

	err = s.dropChunks(110)
	if err != nil {
		t.Fatal(err)
	}
	if len(s.chunkDescs) == 0 {
		t.Fatal("chunk dropped too early")
	}

	err = s.dropChunks(115)
	if err != nil {
		t.Fatal(err)
	}
	if len(s.chunkDescs) == 0 {
		t.Fatal("open head chunk dropped")
	}

	s.headChunkClosed = true
	s.persistWatermark = 1
	err = s.dropChunks(115)
	if err != nil {
		t.Fatal(err)
	}
	if len(s.chunkDescs) != 0 {
		t.Error("did not drop closed head chunk")
	}
}
