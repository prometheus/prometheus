// Copyright 2023 The Prometheus Authors

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

package index

import (
	"reflect"
	"sort"

	"github.com/pkg/errors"

	"github.com/prometheus/prometheus/model/labels"
	"github.com/prometheus/prometheus/storage"
	"github.com/prometheus/prometheus/tsdb/encoding"
	"github.com/prometheus/prometheus/util/annotations"
)

type labelValuesV2 struct {
	name      string
	cur       string
	dec       encoding.Decbuf
	matchers  []*labels.Matcher
	skip      int
	lastVal   string
	exhausted bool
	err       error
}

// newLabelValuesV2 returns an iterator over label values in a v2 index.
func (r *Reader) newLabelValuesV2(name string, matchers []*labels.Matcher) storage.LabelValues {
	p := r.postings[name]

	d := encoding.NewDecbufAt(r.b, int(r.toc.PostingsTable), nil)
	d.Skip(p[0].off)
	// These are always the same number of bytes, and it's faster to skip than to parse
	skip := d.Len()
	// Key count
	d.Uvarint()
	// Label name
	d.UvarintBytes()
	skip -= d.Len()

	return &labelValuesV2{
		name:     name,
		matchers: matchers,
		dec:      d,
		lastVal:  p[len(p)-1].value,
		skip:     skip,
	}
}

func (l *labelValuesV2) Next() bool {
	if l.err != nil || l.exhausted {
		return false
	}

	// Pick the first matching label value
	for l.dec.Err() == nil {
		// Label value
		val := yoloString(l.dec.UvarintBytes())
		isMatch := true
		for _, m := range l.matchers {
			if m.Name != l.name {
				// This should not happen
				continue
			}

			if !m.Matches(val) {
				isMatch = false
				break
			}
		}

		if isMatch {
			l.cur = val
		}
		if val == l.lastVal {
			l.exhausted = true
			return isMatch
		}

		// Offset
		l.dec.Uvarint64()
		// Skip forward to next entry
		l.dec.Skip(l.skip)

		if isMatch {
			break
		}
	}
	if l.dec.Err() != nil {
		// An error occurred decoding
		l.err = errors.Wrap(l.dec.Err(), "get postings offset entry")
		return false
	}

	return true
}

func (l *labelValuesV2) At() string {
	return l.cur
}

func (l *labelValuesV2) Err() error {
	return l.err
}

func (l *labelValuesV2) Warnings() annotations.Annotations {
	return nil
}

type labelValuesV1 struct {
	it       *reflect.MapIter
	matchers []*labels.Matcher
	name     string
}

func (l *labelValuesV1) Next() bool {
loop:
	for l.it.Next() {
		for _, m := range l.matchers {
			if m.Name != l.name {
				// This should not happen
				continue
			}

			if !m.Matches(l.At()) {
				continue loop
			}
		}

		// This entry satisfies all matchers
		return true
	}

	return false
}

func (l *labelValuesV1) At() string {
	return yoloString(l.it.Value().Bytes())
}

func (*labelValuesV1) Err() error {
	return nil
}

func (*labelValuesV1) Warnings() annotations.Annotations {
	return nil
}

func (r *Reader) LabelValuesIntersectingPostings(name string, postings Postings) storage.LabelValues {
	if r.version == FormatV1 {
		/* TODO
		e := r.postingsV1[name]
		if len(e) == 0 {
			return NewEmptyWrapPostingsWithLabelValue()
		}

		var res []PostingsWithLabelValues
		for val, off := range e {
			// Read from the postings table.
			d := encoding.NewDecbufAt(r.b, int(off), castagnoliTable)
			_, p, err := r.dec.Postings(d.Get())
			if err != nil {
				return NewErrWrapPostingsWithLabelValue(errors.Wrap(err, "decode postings"))
			}
			res = append(res, NewWrapPostingsWithLabelValue(p, val))
		}
		return newMergedPostingsWithLabelValues(res)
		*/
		return storage.EmptyLabelValues()
	}

	e := r.postings[name]
	if len(e) == 0 {
		return storage.EmptyLabelValues()
	}

	d := encoding.NewDecbufAt(r.b, int(r.toc.PostingsTable), nil)
	// Skip to start
	d.Skip(e[0].off)
	lastVal := e[len(e)-1].value

	return &intersectLabelValues{
		d:        &d,
		b:        r.b,
		dec:      r.dec,
		lastVal:  lastVal,
		postings: postings,
	}
}

type intersectLabelValues struct {
	d           *encoding.Decbuf
	b           ByteSlice
	dec         *Decoder
	postings    Postings
	curPostings bigEndianPostings
	lastVal     string
	skip        int
	cur         string
	exhausted   bool
	err         error
}

func (it *intersectLabelValues) Next() bool {
	if it.exhausted {
		return false
	}

	for !it.exhausted && it.d.Err() == nil {
		if it.skip == 0 {
			// These are always the same number of bytes,
			// and it's faster to skip than to parse.
			it.skip = it.d.Len()
			// Key count
			it.d.Uvarint()
			// Label name
			it.d.UvarintBytes()
			it.skip -= it.d.Len()
		} else {
			it.d.Skip(it.skip)
		}

		// Label value
		v := yoloString(it.d.UvarintBytes())

		postingsOff := it.d.Uvarint64()
		// Read from the postings table
		d2 := encoding.NewDecbufAt(it.b, int(postingsOff), castagnoliTable)
		_, err := it.dec.PostingsInPlace(d2.Get(), &it.curPostings)
		if err != nil {
			it.err = errors.Wrap(err, "decode postings")
			return false
		}

		it.exhausted = v == it.lastVal

		it.postings.Reset()
		if checkIntersection(&it.curPostings, it.postings) {
			it.cur = v
			return true
		}
	}
	if it.d.Err() != nil {
		it.err = errors.Wrap(it.d.Err(), "get postings offset entry")
	}

	return false
}

func (it *intersectLabelValues) At() string {
	return it.cur
}

func (it *intersectLabelValues) Err() error {
	return it.err
}

func (it *intersectLabelValues) Warnings() annotations.Annotations {
	return nil
}

// ListLabelValues is an iterator over a slice of label values.
type ListLabelValues struct {
	cur    string
	values []string
}

func NewListLabelValues(values []string) *ListLabelValues {
	return &ListLabelValues{
		values: values,
	}
}

func (l *ListLabelValues) Next() bool {
	if len(l.values) == 0 {
		return false
	}

	l.cur = l.values[0]
	l.values = l.values[1:]
	return true
}

func (l *ListLabelValues) At() string {
	return l.cur
}

func (*ListLabelValues) Err() error {
	return nil
}

func (*ListLabelValues) Warnings() annotations.Annotations {
	return nil
}

func (p *MemPostings) LabelValuesIntersectingPostings(name string, postings Postings) storage.LabelValues {
	p.mtx.RLock()
	defer p.mtx.RUnlock()

	e := p.m[name]
	if len(e) == 0 {
		return storage.EmptyLabelValues()
	}

	// With thread safety in mind and due to random key ordering in map, we have to construct the array in memory
	vals := make([]string, 0, len(e))
	for val, srs := range e {
		p.curPostings.list = srs
		p.curPostings.Reset()
		postings.Reset()

		if checkIntersection(&p.curPostings, postings) {
			vals = append(vals, val)
		}
	}

	sort.Strings(vals)
	return NewListLabelValues(vals)
}

// checkIntersection returns whether p1 and p2 have at least one series in common.
func checkIntersection(p1, p2 Postings) bool {
	if !p1.Next() || !p2.Next() {
		return false
	}

	cur := p1.At()
	if p2.At() > cur {
		cur = p2.At()
	}

	for {
		if !p1.Seek(cur) {
			break
		}
		if p1.At() > cur {
			cur = p1.At()
		}
		if !p2.Seek(cur) {
			break
		}
		if p2.At() > cur {
			cur = p2.At()
			continue
		}

		return true
	}

	return false
}
