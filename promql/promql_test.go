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

package promql

import (
	"path/filepath"
	"testing"
)

func TestEvaluations(t *testing.T) {
	files, err := filepath.Glob("testdata/*.test")
	if err != nil {
		t.Fatal(err)
	}
	for _, fn := range files {
		test, err := newTestFromFile(t, fn)
		if err != nil {
			t.Errorf("error creating test for %s: %s", fn, err)
		}
		err = test.Run()
		if err != nil {
			t.Errorf("error running test %s: %s", fn, err)
		}
		test.Close()
	}
}
