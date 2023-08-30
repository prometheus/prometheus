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
	"github.com/prometheus/prometheus/util/notes"
)

type Notes struct {
	warnings notes.Warnings
}

// func (n *Notes) AddWarning(txt string) {
// 	n.warnings = append(n.warnings, fmt.Errorf(txt))
// }

func (n *Notes) AddWarningErr(err error) {
	n.warnings = append(n.warnings, err)
}

// func CreateNotesWithWarning(txt string) Notes {
// 	notes := Notes{}
// 	notes.AddWarning(txt)
// 	return notes
// }

func CreateNotesWithWarningErr(err error) Notes {
	notes := Notes{}
	notes.AddWarningErr(err)
	return notes
}

func (n *Notes) Merge(nn Notes) {
	n.warnings = append(n.warnings, nn.warnings...)
}
