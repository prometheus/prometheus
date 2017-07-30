// Copyright 2015 The Prometheus Authors
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
// http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, softwar
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package promql

// RunAsBenchmark runs the test in benchmark mode.
// This will not count any loads or non eval functions.
func (t *Test) RunAsBenchmark(b *Benchmark) error {
	for _, cmd := range t.cmds {

		switch cmd.(type) {
		// Only time the "eval" command.
		case *evalCmd:
			err := t.exec(cmd)
			if err != nil {
				return err
			}
		default:
			if b.iterCount == 0 {
				b.b.StopTimer()
				err := t.exec(cmd)
				if err != nil {
					return err
				}
				b.b.StartTimer()
			}
		}
	}
	return nil
}
