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
	"github.com/prometheus/prometheus/utility"

	dto "github.com/prometheus/prometheus/model/generated"
)

type dtoSampleKeyList struct {
	l utility.FreeList
}

func newDtoSampleKeyList(cap int) *dtoSampleKeyList {
	return &dtoSampleKeyList{
		l: utility.NewFreeList(cap),
	}
}

func (l *dtoSampleKeyList) Get() (*dto.SampleKey, bool) {
	if v, ok := l.l.Get(); ok {
		return v.(*dto.SampleKey), ok
	}

	return new(dto.SampleKey), false
}

func (l *dtoSampleKeyList) Give(v *dto.SampleKey) bool {
	v.Reset()

	return l.l.Give(v)
}

func (l *dtoSampleKeyList) Close() {
	l.l.Close()
}

type sampleKeyList struct {
	l utility.FreeList
}

var defaultSampleKey = new(SampleKey)

func newSampleKeyList(cap int) *sampleKeyList {
	return &sampleKeyList{
		l: utility.NewFreeList(cap),
	}
}

func (l *sampleKeyList) Get() (*SampleKey, bool) {
	if v, ok := l.l.Get(); ok {
		return v.(*SampleKey), ok
	}

	return new(SampleKey), false
}

func (l *sampleKeyList) Give(v *SampleKey) bool {
	*v = *defaultSampleKey

	return l.l.Give(v)
}

func (l *sampleKeyList) Close() {
	l.l.Close()
}
