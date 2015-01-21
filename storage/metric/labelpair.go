// Copyright 2013 The Prometheus Authors
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
	clientmodel "github.com/prometheus/client_golang/model"
)

// LabelPair pairs a name with a value.
type LabelPair struct {
	Name  clientmodel.LabelName
	Value clientmodel.LabelValue
}

// Equal returns true iff both the Name and the Value of this LabelPair and o
// are equal.
func (l *LabelPair) Equal(o *LabelPair) bool {
	switch {
	case l.Name != o.Name:
		return false
	case l.Value != o.Value:
		return false
	default:
		return true
	}
}

// LabelPairs is a sortable slice of LabelPair pointers. It implements
// sort.Interface.
type LabelPairs []*LabelPair

func (l LabelPairs) Len() int {
	return len(l)
}

func (l LabelPairs) Less(i, j int) bool {
	switch {
	case l[i].Name > l[j].Name:
		return false
	case l[i].Name < l[j].Name:
		return true
	case l[i].Value > l[j].Value:
		return false
	case l[i].Value < l[j].Value:
		return true
	default:
		return false
	}
}

func (l LabelPairs) Swap(i, j int) {
	l[i], l[j] = l[j], l[i]
}
