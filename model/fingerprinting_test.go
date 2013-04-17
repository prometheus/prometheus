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
	"testing"
)

func TestFingerprintComparison(t *testing.T) {
	fingerprints := []fingerprint{
		{
			hash: 0,
			firstCharacterOfFirstLabelName: "b",
			labelMatterLengthModulus:       1,
			lastCharacterOfLastLabelValue:  "b",
		},
		{
			hash: 1,
			firstCharacterOfFirstLabelName: "a",
			labelMatterLengthModulus:       0,
			lastCharacterOfLastLabelValue:  "a",
		},
		{
			hash: 1,
			firstCharacterOfFirstLabelName: "a",
			labelMatterLengthModulus:       1000,
			lastCharacterOfLastLabelValue:  "b",
		},
		{
			hash: 1,
			firstCharacterOfFirstLabelName: "b",
			labelMatterLengthModulus:       0,
			lastCharacterOfLastLabelValue:  "a",
		},
		{
			hash: 1,
			firstCharacterOfFirstLabelName: "b",
			labelMatterLengthModulus:       1,
			lastCharacterOfLastLabelValue:  "a",
		},
		{
			hash: 1,
			firstCharacterOfFirstLabelName: "b",
			labelMatterLengthModulus:       1,
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
