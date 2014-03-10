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
	"time"

	"github.com/prometheus/prometheus/utility"

	clientmodel "github.com/prometheus/client_golang/model"
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

	return &dto.SampleKey{}, false
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

var defaultSampleKey = &SampleKey{}

func newSampleKeyList(cap int) *sampleKeyList {
	return &sampleKeyList{
		l: utility.NewFreeList(cap),
	}
}

func (l *sampleKeyList) Get() (*SampleKey, bool) {
	if v, ok := l.l.Get(); ok {
		return v.(*SampleKey), ok
	}

	return &SampleKey{}, false
}

func (l *sampleKeyList) Give(v *SampleKey) bool {
	*v = *defaultSampleKey

	return l.l.Give(v)
}

func (l *sampleKeyList) Close() {
	l.l.Close()
}

type valueAtTimeList struct {
	l utility.FreeList
}

func (l *valueAtTimeList) Get(fp *clientmodel.Fingerprint, time clientmodel.Timestamp) *getValuesAtTimeOp {
	var op *getValuesAtTimeOp
	v, ok := l.l.Get()
	if ok {
		op = v.(*getValuesAtTimeOp)
	} else {
		op = &getValuesAtTimeOp{}
	}
	op.fp = *fp
	op.current = time
	return op
}

var pGetValuesAtTimeOp = &getValuesAtTimeOp{}

func (l *valueAtTimeList) Give(v *getValuesAtTimeOp) bool {
	*v = *pGetValuesAtTimeOp

	return l.l.Give(v)
}

func newValueAtTimeList(cap int) *valueAtTimeList {
	return &valueAtTimeList{
		l: utility.NewFreeList(cap),
	}
}

type valueAtIntervalList struct {
	l utility.FreeList
}

func (l *valueAtIntervalList) Get(fp *clientmodel.Fingerprint, from, through clientmodel.Timestamp, interval time.Duration) *getValuesAtIntervalOp {
	var op *getValuesAtIntervalOp
	v, ok := l.l.Get()
	if ok {
		op = v.(*getValuesAtIntervalOp)
	} else {
		op = &getValuesAtIntervalOp{}
	}
	op.fp = *fp
	op.current = from
	op.through = through
	op.interval = interval
	return op
}

var pGetValuesAtIntervalOp = &getValuesAtIntervalOp{}

func (l *valueAtIntervalList) Give(v *getValuesAtIntervalOp) bool {
	*v = *pGetValuesAtIntervalOp

	return l.l.Give(v)
}

func newValueAtIntervalList(cap int) *valueAtIntervalList {
	return &valueAtIntervalList{
		l: utility.NewFreeList(cap),
	}
}

type valueAlongRangeList struct {
	l utility.FreeList
}

func (l *valueAlongRangeList) Get(fp *clientmodel.Fingerprint, from, through clientmodel.Timestamp) *getValuesAlongRangeOp {
	var op *getValuesAlongRangeOp
	v, ok := l.l.Get()
	if ok {
		op = v.(*getValuesAlongRangeOp)
	} else {
		op = &getValuesAlongRangeOp{}
	}
	op.fp = *fp
	op.current = from
	op.through = through
	return op
}

var pGetValuesAlongRangeOp = &getValuesAlongRangeOp{}

func (l *valueAlongRangeList) Give(v *getValuesAlongRangeOp) bool {
	*v = *pGetValuesAlongRangeOp

	return l.l.Give(v)
}

func newValueAlongRangeList(cap int) *valueAlongRangeList {
	return &valueAlongRangeList{
		l: utility.NewFreeList(cap),
	}
}

type valueAtIntervalAlongRangeList struct {
	l utility.FreeList
}

func (l *valueAtIntervalAlongRangeList) Get(fp *clientmodel.Fingerprint, from, through clientmodel.Timestamp, interval, rangeDuration time.Duration) *getValueRangeAtIntervalOp {
	var op *getValueRangeAtIntervalOp
	v, ok := l.l.Get()
	if ok {
		op = v.(*getValueRangeAtIntervalOp)
	} else {
		op = &getValueRangeAtIntervalOp{}
	}
	op.fp = *fp
	op.current = from
	op.rangeThrough = from.Add(rangeDuration)
	op.rangeDuration = rangeDuration
	op.interval = interval
	op.through = through
	return op
}

var pGetValueRangeAtIntervalOp = &getValueRangeAtIntervalOp{}

func (l *valueAtIntervalAlongRangeList) Give(v *getValueRangeAtIntervalOp) bool {
	*v = *pGetValueRangeAtIntervalOp

	return l.l.Give(v)
}

func newValueAtIntervalAlongRangeList(cap int) *valueAtIntervalAlongRangeList {
	return &valueAtIntervalAlongRangeList{
		l: utility.NewFreeList(cap),
	}
}
