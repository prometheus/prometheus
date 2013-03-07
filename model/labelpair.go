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

package model

type LabelPair struct {
	Name  LabelName
	Value LabelValue
}

type LabelPairs []LabelPair

func (l LabelPairs) Len() int {
	return len(l)
}

func (l LabelPairs) Less(i, j int) (less bool) {
	if l[i].Name < l[j].Name {
		return true
	}

	if l[i].Value < l[j].Value {
		return true
	}

	return false
}

func (l LabelPairs) Swap(i, j int) {
	l[i], l[j] = l[j], l[i]
}
