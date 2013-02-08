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

package metric

import (
	"bytes"
	"fmt"
	"github.com/prometheus/prometheus/model"
)

// scanJob models a range of queries.
type scanJob struct {
	fingerprint model.Fingerprint
	operations  ops
}

func (s scanJob) String() string {
	buffer := &bytes.Buffer{}
	fmt.Fprintf(buffer, "Scan Job { fingerprint=%s ", s.fingerprint)
	fmt.Fprintf(buffer, " with %d operations [", len(s.operations))
	for _, operation := range s.operations {
		fmt.Fprintf(buffer, "%s", operation)
	}
	fmt.Fprintf(buffer, "] }")

	return buffer.String()
}

type scanJobs []scanJob

func (s scanJobs) Len() int {
	return len(s)
}

func (s scanJobs) Less(i, j int) (less bool) {
	less = s[i].fingerprint.Less(s[j].fingerprint)

	return
}

func (s scanJobs) Swap(i, j int) {
	s[i], s[j] = s[j], s[i]
}
