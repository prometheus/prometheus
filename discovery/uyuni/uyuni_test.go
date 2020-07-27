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

package uyuni

import "testing"

func TestExtractPortFromFormulaData(t *testing.T) {
	type args struct {
		args    string
		address string
	}
	tests := []struct {
		name    string
		args    args
		want    string
		wantErr bool
	}{
		{name: `TestArgs`, args: args{args: `--web.listen-address=":9100"`}, want: `9100`},
		{name: `TestAddress`, args: args{address: `:9100`}, want: `9100`},
		{name: `TestArgsAndAddress`, args: args{args: `--web.listen-address=":9100"`, address: `9999`}, want: `9100`},
		{name: `TestMissingPort`, args: args{args: `localhost`}, wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := extractPortFromFormulaData(tt.args.args, tt.args.address)
			if (err != nil) != tt.wantErr {
				t.Errorf("extractPortFromFormulaData() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if got != tt.want {
				t.Errorf("extractPortFromFormulaData() got = %v, want %v", got, tt.want)
			}
		})
	}
}
