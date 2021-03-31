// Copyright 2017 The Prometheus Authors
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

package goversion_test

import (
	"testing"

	_ "github.com/prometheus/prometheus/tsdb/goversion"
)

// This test is intentionally blank and exists only so `go test` believes
// there is something to test.
//
// The blank import above is actually what invokes the test of this package. If
// the import succeeds (the code compiles), the test passed.
func Test(t *testing.T) {}
