/*
Copyright 2017 Google LLC

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/
package bigtable

import (
	"testing"
	"time"

	bttdpb "google.golang.org/genproto/googleapis/bigtable/admin/v2"
)

func TestGcRuleToString(t *testing.T) {

	intersection := IntersectionPolicy(MaxVersionsPolicy(5), MaxVersionsPolicy(10), MaxAgePolicy(16*time.Hour))

	var tests = []struct {
		proto *bttdpb.GcRule
		want  string
	}{
		{MaxAgePolicy(72 * time.Hour).proto(), "age() > 3d"},
		{MaxVersionsPolicy(5).proto(), "versions() > 5"},
		{intersection.proto(), "(versions() > 5 && versions() > 10 && age() > 16h)"},
		{UnionPolicy(intersection, MaxAgePolicy(72*time.Hour)).proto(),
			"((versions() > 5 && versions() > 10 && age() > 16h) || age() > 3d)"},
	}

	for _, test := range tests {
		got := GCRuleToString(test.proto)
		if got != test.want {
			t.Errorf("got gc rule string: %v, wanted: %v", got, test.want)
		}
	}
}
