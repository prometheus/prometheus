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

package logging

import (
	"testing"
	"time"

	"github.com/prometheus/prometheus/util/testutil"
)

type counter int

func (c *counter) Log(keyvals ...interface{}) error {
	(*c)++
	return nil
}

func TestDedupe(t *testing.T) {
	var c counter
	d := Dedupe(&c, 100*time.Millisecond)
	defer d.Stop()

	// Log 10 times quickly, ensure they are deduped.
	for i := 0; i < 10; i++ {
		err := d.Log("msg", "hello")
		testutil.Ok(t, err)
	}
	testutil.Equals(t, 1, int(c))

	// Wait, then log again, make sure it is logged.
	time.Sleep(200 * time.Millisecond)
	err := d.Log("msg", "hello")
	testutil.Ok(t, err)
	testutil.Equals(t, 2, int(c))
}
