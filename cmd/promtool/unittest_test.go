// Copyright 2018 The Prometheus Authors
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

package main

import "testing"

func TestRulesUnitTest(t *testing.T) {
	type args struct {
		files []string
	}
	tests := []struct {
		name string
		args args
		want int
	}{
		{
			name: "Passing Unit Tests",
			args: args{
				files: []string{"./testdata/unittest.yml"},
			},
			want: 0,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := RulesUnitTest(tt.args.files...); got != tt.want {
				t.Errorf("RulesUnitTest() = %v, want %v", got, tt.want)
			}
		})
	}
}
