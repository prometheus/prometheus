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

package cloudmap

import "testing"

func TestAccountParse(t *testing.T) {
	cases := []struct {
		in, want string
	}{
		{"arn:aws:iam::222265345133:role/Monitoring-EU-WEST-1", "222265345133"},
	}
	for _, c := range cases {
		got := ParseAccountNumberFromArn(c.in)
		if got != c.want {
			t.Errorf("ParseAccountNumberFromArn(%q) == %q, want %q", c.in, got, c.want)
		}
	}
}
