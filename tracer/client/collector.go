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

package tracer

import (
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"sync"
	"time"

	"github.com/prometheus/prometheus/tracer/model"
)

var (
	traceDurations = []time.Duration{
		time.Duration(1) * time.Second,
		time.Duration(10) * time.Second,
		time.Duration(100) * time.Second,
	}
)

type index struct {
	bufferID int
	slotID   int
}

type buffer struct {
	traces []model.Trace
	next   int
	length int
}

type Collector struct {
	mtx      sync.RWMutex
	traceIDs map[uint64]*index
	buffers  []buffer
}

type Query struct {
	ServiceName string
	SpanName    string
	SearchQuery map[string]*regexp.Regexp
	MinDuration time.Duration
	MaxDuration time.Duration
	End         time.Time
	Start       time.Time
	Limit       int
}

// Initialize a new Tracer of capacity for each trace buffer
func newCollector(capacity int) *Collector {
	traces := make([]buffer, len(traceDurations)+1)

	for i := 0; i <= len(traceDurations); i++ {
		traces[i].traces = make([]model.Trace, capacity)
		traces[i].next = 0
		traces[i].length = 0
	}
	return &Collector{
		traceIDs: make(map[uint64]*index, capacity),
		buffers:  traces,
	}
}

func getBufferIndex(duration time.Duration) int {
	for i, d := range traceDurations {
		if duration <= d {
			return i
		}
	}
	return len(traceDurations)
}

func (c *Collector) getFreeSlotInBuffer(bufferID int) int {

	// Pick a slot in buffer for new trace
	idx := c.buffers[bufferID].next
	c.buffers[bufferID].next = (c.buffers[bufferID].next + 1) % cap(c.buffers[bufferID].traces) //wrap

	// If the slot is occupied, we'll need to clear the trace ID index.
	if c.buffers[bufferID].length == cap(c.buffers[bufferID].traces) {
		traceIDToDelete := c.buffers[bufferID].traces[idx].TraceId
		if traceIDToDelete != 0 {
			// most recent copy of the trace to be deleted is in this buffer
			// should not delete if trace has a copy in another buffer of larger duration
			delete(c.traceIDs, traceIDToDelete)
		}
	} else {
		c.buffers[bufferID].length++
	}

	return idx
}

// Collect stores a new span in the tracer ring buffer
func (c *Collector) Collect(span model.Span) error {
	c.mtx.Lock()
	defer c.mtx.Unlock()

	traceID := span.TraceId
	if traceID == 0 {
		return errors.New("TraceID should not be zero")
	}

	idx, ok := c.traceIDs[traceID]
	if !ok {
		duration := span.Duration()
		bufferID := getBufferIndex(duration)
		bufferSlot := c.getFreeSlotInBuffer(bufferID)
		c.traceIDs[traceID] = &index{
			bufferID: bufferID,
			slotID:   bufferSlot,
		}
		// Clear out spans from any previous trace in the buffer slot
		c.buffers[bufferID].traces[bufferSlot].Spans = c.buffers[bufferID].traces[bufferSlot].Spans[:0]
		// Add Trace details in new slot
		c.buffers[bufferID].traces[bufferSlot].TraceId = traceID
		c.buffers[bufferID].traces[bufferSlot].Spans = append(c.buffers[bufferID].traces[bufferSlot].Spans, span)
	} else {
		c.buffers[idx.bufferID].traces[idx.slotID].Spans = append(c.buffers[idx.bufferID].traces[idx.slotID].Spans, span)
		duration := c.buffers[idx.bufferID].traces[idx.slotID].Duration()
		bufferID := getBufferIndex(duration)

		if bufferID > idx.bufferID {
			// promote trace to a buffer of larger duration
			bufferSlot := c.getFreeSlotInBuffer(bufferID)

			c.buffers[bufferID].traces[bufferSlot].TraceId = traceID

			c.buffers[bufferID].traces[bufferSlot].Spans = make([]model.Span, len(c.buffers[idx.bufferID].traces[idx.slotID].Spans)) // since copy copies min(dst, src), extending dst to keep all src elements
			copy(c.buffers[bufferID].traces[bufferSlot].Spans, c.buffers[idx.bufferID].traces[idx.slotID].Spans)

			// reseting old slot
			c.buffers[idx.bufferID].traces[idx.slotID].TraceId = 0
			c.buffers[idx.bufferID].traces[idx.slotID].Spans = c.buffers[idx.bufferID].traces[idx.slotID].Spans[:0]
			idx.bufferID = bufferID
			idx.slotID = bufferSlot
		}
	}
	return nil
}

// ParseTagsQuery unmarshals user tag query as JSON and converts them into a map of regex
func ParseTagsQuery(query string) (map[string]*regexp.Regexp, error) {

	query = "{" + query + "}"

	queryByte := []byte(query)

	var objmap map[string]*json.RawMessage
	err := json.Unmarshal(queryByte, &objmap)
	if err != nil {
		return nil, err
	}

	var value string
	tag := make(map[string]*regexp.Regexp)

	for k := range objmap {
		err := json.Unmarshal(*objmap[k], &value)

		if err != nil {
			return nil, err
		}
		if r, regexErr := regexp.Compile(value); regexErr == nil {
			tag[k] = r
		} else {
			return nil, regexErr
		}
	}
	return tag, nil
}

// checkQuery checks if a trace satisfies the query parameters
func checkQuery(t *model.Trace, query *Query) bool {

	minTimestamp, maxTimestamp := t.GetMinMaxTimestamp()

	if maxTimestamp.Before(query.Start) || minTimestamp.After(query.End) {
		return false
	}

	traceDuration := maxTimestamp.Sub(minTimestamp)
	if query.MinDuration > 0 && traceDuration < query.MinDuration {
		return false
	}

	if query.MaxDuration > 0 && traceDuration > query.MaxDuration {
		return false
	}

	if query.SpanName != "" && query.SpanName != "all" {
		found := false
		for _, span := range t.Spans {
			if span.OperationName == query.SpanName {
				found = true
				break
			}
		}
		if !found {
			return false
		}
	}

	if query.SearchQuery != nil {
		found := false
		// check if there exists a span whose tags satisfy all queries in SearchQuery
		for _, span := range t.Spans {

			matchedTagQueriesFreq := 0
			for key, valueRegex := range query.SearchQuery {

				isTagQueryMatched := false
				for _, tag := range span.Tags {
					if tag.Key == key {
						if valueRegex.MatchString(tag.Value().(string)) {
							matchedTagQueriesFreq++
							isTagQueryMatched = true
							break
						}
					}
				}
				if isTagQueryMatched == false {
					break
				}
			}
			if matchedTagQueriesFreq == len(query.SearchQuery) {
				found = true
				break
			}
		}
		if !found {
			return false
		}
	}
	return true
}

func (c *Collector) GetTrace(id uint64) (*model.Trace, error) {
	c.mtx.RLock()
	defer c.mtx.RUnlock()

	idx, ok := c.traceIDs[id]
	if !ok {
		return nil, fmt.Errorf("Trace not found in buffer")
	}
	return &c.buffers[idx.bufferID].traces[idx.slotID], nil
}

func (c *Collector) GetSpanNames() []string {
	traces := c.Gather(nil)
	spanNames := make([]string, 0)
	spanNamesMap := make(map[string]struct{}, 0)
	var emptyStruct struct{}
	for _, t := range traces {
		for _, s := range t.Spans {
			if _, ok := spanNamesMap[s.OperationName]; !ok {
				spanNamesMap[s.OperationName] = emptyStruct
				spanNames = append(spanNames, s.OperationName)
			}
		}
	}
	return spanNames
}

// Gather iterates over the ring buffers and gets the traces which satisfy the query
func (c *Collector) Gather(query *Query) []model.Trace {
	c.mtx.RLock()
	defer c.mtx.RUnlock()

	traces := make([]model.Trace, 0)

	// gather traces ordered from longest duration to shortest duration
	for i := len(c.buffers) - 1; i >= 0; i-- {
		slot, count := c.buffers[i].next-c.buffers[i].length, 0
		if slot < 0 {
			slot = cap(c.buffers[i].traces) + slot
		}
		for count < c.buffers[i].length {
			slot %= cap(c.buffers[i].traces)
			if query != nil {
				// don't check outdated copies present in buffers of shorter durations
				if c.buffers[i].traces[slot].TraceId > 0 {
					if checkQuery(&c.buffers[i].traces[slot], query) {
						traces = append(traces, c.buffers[i].traces[slot])
					}
					if len(traces) == query.Limit {
						return traces
					}
				}
			} else {
				// don't check outdated copies present in buffers of shorter durations
				if c.buffers[i].traces[slot].TraceId > 0 {
					traces = append(traces, c.buffers[i].traces[slot])
				}
			}
			slot++
			count++
		}
	}
	return traces
}
