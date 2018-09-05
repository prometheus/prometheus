// Copyright 2018 The Prometheus Authors
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

package remote

import (
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"sort"

	"github.com/gogo/protobuf/proto"
	"github.com/golang/snappy"
	"github.com/matttproud/golang_protobuf_extensions/pbutil"
	"github.com/pkg/errors"
	"github.com/prometheus/prometheus/pkg/labels"
	"github.com/prometheus/prometheus/prompb"
	"github.com/prometheus/prometheus/storage"
)

// QuerySet iterates over set of SeriesSet for each remote read query.
// Similar to storage.SeriesSet, it's caller responsibility to ensure it is always emptied (`Next()` is invoked until false)
// or error is return from `Err()`. This ensures that all used resources are released.
//
// Example usage:
//	 ```
//	 for q.Next {
//		ss := q.At()
//
//		(...)
//		if someErr {
//   			// Empty seriesSet to release all resources.
//			for q.Next() {}
//
//			// (...) handle someErr.
//			return
//		}
//
//	 }
//     if err := q.Err(); err != nil {
//		// (...) handle err.
//		return
//	 }
//	 ```
type QuerySet interface {
	Next() bool
	At() storage.SeriesSet
	Err() error
}

// errQuerySet implements QuerySet, just returning an error.
type errQuerySet struct {
	err error
}

func (errQuerySet) Next() bool            { return false }
func (errQuerySet) At() storage.SeriesSet { return nil }
func (e errQuerySet) Err() error          { return e.err }

// DecodeReadResponse return QuerySet that allows to iterate over remote read results.
// In case of stream content type and encoding, it returns purely stream QuerySet that reads
// response body on each series iteration.
func DecodeReadResponse(resHeader http.Header, r io.ReadCloser) QuerySet {
	contentType := resHeader.Get("Content-Type")
	contentEncoding := resHeader.Get("Content-Encoding")

	if contentType == protoContentType && contentEncoding == snappyContentEnc {
		// Prometheus 1.x non-streamed remote read decode.
		return newBufferedQuerySet(r)
	}

	if contentType == protoDelimitedContentType && contentEncoding == snappyFramedContentEnc {
		return newStreamQuerySet(r)
	}

	r.Close()
	return errQuerySet{err: errors.Errorf("not supported content type %s and encoding %s", contentType, contentEncoding)}
}

func newBufferedQuerySet(r io.ReadCloser) QuerySet {
	defer r.Close()

	compressed, err := ioutil.ReadAll(r)
	if err != nil {
		return errQuerySet{err: fmt.Errorf("error reading response: %v", err)}
	}

	uncompressed, err := snappy.Decode(nil, compressed)
	if err != nil {
		return errQuerySet{err: fmt.Errorf("error decoding response: %v", err)}
	}

	var resp prompb.ReadResponse
	if err := proto.Unmarshal(uncompressed, &resp); err != nil {
		return errQuerySet{err: fmt.Errorf("unable to unmarshal response body: %v", err)}
	}

	return &bufferedQuerySet{results: resp.Results}
}

// bufferedQuerySet implements QuerySet.
// It returns series sorted by labels.
type bufferedQuerySet struct {
	cur     int
	results []*prompb.QueryResult
	err     error

	at storage.SeriesSet
}

func (c *bufferedQuerySet) Next() bool {
	c.cur++
	if c.cur > len(c.results) {
		return false
	}

	res := c.results[c.cur-1]
	series := make([]storage.Series, 0, len(res.Timeseries))
	for _, ts := range res.Timeseries {
		labels := labelProtosToLabels(ts.Labels)
		if err := validateLabelsAndMetricName(labels); err != nil {
			c.err = err
			return false
		}

		series = append(series, &concreteSeries{
			labels:  labels,
			samples: ts.Samples,
		})
	}
	sort.Sort(byLabel(series))
	c.at = &concreteSeriesSet{
		series: series,
	}

	return true
}

func (c *bufferedQuerySet) At() storage.SeriesSet { return c.at }
func (c *bufferedQuerySet) Err() error            { return errors.Wrap(c.err, "bufferedQuerySet:") }

type byLabel []storage.Series

func (a byLabel) Len() int           { return len(a) }
func (a byLabel) Swap(i, j int)      { a[i], a[j] = a[j], a[i] }
func (a byLabel) Less(i, j int) bool { return labels.Compare(a[i].Labels(), a[j].Labels()) < 0 }

// concreteSeriesSet implements storage.SeriesSet.
type concreteSeriesSet struct {
	cur    int
	series []storage.Series
}

func (c *concreteSeriesSet) Next() bool {
	c.cur++
	return c.cur-1 < len(c.series)
}

func (c *concreteSeriesSet) At() storage.Series {
	return c.series[c.cur-1]
}

func (c *concreteSeriesSet) Err() error {
	return nil
}

// concreteSeries implements storage.Series.
type concreteSeries struct {
	labels  labels.Labels
	samples []*prompb.Sample
}

func (c *concreteSeries) Labels() labels.Labels {
	return labels.New(c.labels...)
}

func (c *concreteSeries) Iterator() storage.SeriesIterator {
	return newConcreteSeriersIterator(c)
}

// concreteSeriesIterator implements storage.SeriesIterator.
type concreteSeriesIterator struct {
	cur    int
	series *concreteSeries
}

func newConcreteSeriersIterator(series *concreteSeries) storage.SeriesIterator {
	return &concreteSeriesIterator{
		cur:    -1,
		series: series,
	}
}

// Seek implements storage.SeriesIterator.
func (c *concreteSeriesIterator) Seek(t int64) bool {
	c.cur = sort.Search(len(c.series.samples), func(n int) bool {
		return c.series.samples[n].Timestamp >= t
	})
	return c.cur < len(c.series.samples)
}

// At implements storage.SeriesIterator.
func (c *concreteSeriesIterator) At() (t int64, v float64) {
	s := c.series.samples[c.cur]
	return s.Timestamp, s.Value
}

// Next implements storage.SeriesIterator.
func (c *concreteSeriesIterator) Next() bool {
	c.cur++
	return c.cur < len(c.series.samples)
}

// Err implements storage.SeriesIterator.
func (c *concreteSeriesIterator) Err() error {
	return nil
}

// streamQuerySet implements QuerySet.
// It assumes multiple QueryResults for single query and results for different queries coming in order.
type streamQuerySet struct {
	c io.Closer
	r io.Reader

	s         storage.SeriesSet
	curQuery  int64
	curSeries int
	buffRes   *prompb.QueryResult
	err       error
}

func newStreamQuerySet(r io.ReadCloser) *streamQuerySet {
	q := &streamQuerySet{
		c: r,
		r: snappy.NewReader(r),
	}
	q.s = &streamSeriesSet{q: q}
	return q
}

func (c *streamQuerySet) Next() bool {
	if c.buffRes == nil {
		return c.nextQueryResult()
	}

	// Loop until next query (if new query is not already set by streamSeriesSet, skip series until next one) or EOF.
	for {
		if c.curQuery != c.buffRes.QueryNum {
			c.curSeries = 0
			c.curQuery++
			return true
		}

		if !c.nextQueryResult() {
			return false
		}
	}
}

func (c *streamQuerySet) nextQueryResult() bool {
	c.buffRes = &prompb.QueryResult{}
	if _, err := pbutil.ReadDelimited(c.r, c.buffRes); err != nil {
		if err != io.EOF {
			c.err = err
		}
		c.buffRes = nil
		io.Copy(ioutil.Discard, c.r)
		c.c.Close()
		return false
	}
	return true
}

func (c *streamQuerySet) At() storage.SeriesSet { return c.s }
func (c *streamQuerySet) Err() error            { return errors.Wrap(c.err, "streamQuerySet:") }

// streamSeriesSet implements storage.SeriesSet.
type streamSeriesSet struct {
	q *streamQuerySet

	at  storage.Series
	err error
}

func (c *streamSeriesSet) Next() bool {
	if c.q.buffRes == nil || c.q.curQuery != c.q.buffRes.QueryNum {
		return false
	}

	c.q.curSeries++
	if c.q.curSeries > len(c.q.buffRes.Timeseries) {
		if !c.q.nextQueryResult() {
			return false
		}

		if c.q.curQuery != c.q.buffRes.QueryNum {
			// Different query.
			return false
		}
		c.q.curSeries = 1
	}

	ts := c.q.buffRes.Timeseries[c.q.curSeries-1]
	labels := labelProtosToLabels(ts.Labels)
	if err := validateLabelsAndMetricName(labels); err != nil {
		c.err = err
		return false
	}

	c.at = &concreteSeries{
		labels:  labels,
		samples: ts.Samples,
	}
	return true
}

func (c *streamSeriesSet) At() storage.Series { return c.at }
func (c *streamSeriesSet) Err() error         { return errors.Wrap(c.err, "streamSeriesSet:") }
