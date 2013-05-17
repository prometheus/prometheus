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

package model

import (
	"runtime"
	"testing"
)

func TestFingerprintComparison(t *testing.T) {
	fingerprints := []*Fingerprint{
		{
			hash: 0,
			firstCharacterOfFirstLabelName: "b",
			labelMatterLength:              1,
			lastCharacterOfLastLabelValue:  "b",
		},
		{
			hash: 1,
			firstCharacterOfFirstLabelName: "a",
			labelMatterLength:              0,
			lastCharacterOfLastLabelValue:  "a",
		},
		{
			hash: 1,
			firstCharacterOfFirstLabelName: "a",
			labelMatterLength:              1000,
			lastCharacterOfLastLabelValue:  "b",
		},
		{
			hash: 1,
			firstCharacterOfFirstLabelName: "b",
			labelMatterLength:              0,
			lastCharacterOfLastLabelValue:  "a",
		},
		{
			hash: 1,
			firstCharacterOfFirstLabelName: "b",
			labelMatterLength:              1,
			lastCharacterOfLastLabelValue:  "a",
		},
		{
			hash: 1,
			firstCharacterOfFirstLabelName: "b",
			labelMatterLength:              1,
			lastCharacterOfLastLabelValue:  "b",
		},
	}
	for i := range fingerprints {
		if i == 0 {
			continue
		}

		if !fingerprints[i-1].Less(fingerprints[i]) {
			t.Errorf("%d expected %s < %s", i, fingerprints[i-1], fingerprints[i])
		}
	}
}

func BenchmarkFingerprinting(b *testing.B) {
	b.StopTimer()
	fps := []*Fingerprint{
		{
			hash: 0,
			firstCharacterOfFirstLabelName: "a",
			labelMatterLength:              2,
			lastCharacterOfLastLabelValue:  "z",
		},
		{
			hash: 0,
			firstCharacterOfFirstLabelName: "a",
			labelMatterLength:              2,
			lastCharacterOfLastLabelValue:  "z",
		},
	}
	for i := 0; i < 10; i++ {
		fps[0].Less(fps[1])
	}
	b.Logf("N: %v", b.N)
	b.StartTimer()

	var pre runtime.MemStats
	runtime.ReadMemStats(&pre)

	for i := 0; i < b.N; i++ {
		fps[0].Less(fps[1])
	}

	var post runtime.MemStats
	runtime.ReadMemStats(&post)

	b.Logf("allocs: %d items: ", post.TotalAlloc-pre.TotalAlloc)
}
