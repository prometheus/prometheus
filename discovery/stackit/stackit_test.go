// Copyright 2020 The Prometheus Authors
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

package stackit

import (
	"testing"
)

func TestNewRefresher(t *testing.T) {
	tests := []struct {
		role     Role
		wantErr  bool
		testName string
	}{
		{RoleServer, false, "Valid server role"},
		{RolePostgresFlex, false, "Valid postgres_flex role"},
		{"invalid_role", true, "Invalid role"},
	}
	for _, tt := range tests {
		conf := &SDConfig{Role: tt.role}
		_, err := newRefresher(conf, nil)
		gotErr := err != nil
		if gotErr != tt.wantErr {
			t.Errorf("%s: expected error = %v, but got error = %v (%v)", tt.testName, tt.wantErr, gotErr, err)
		}
	}
}
