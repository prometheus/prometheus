// Copyright The Prometheus Authors
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
	"compress/gzip"
	"errors"
	"fmt"
	"io"
	"math"
	"net/http"
	"slices"
	"sort"
	"sync"

	"github.com/gogo/protobuf/proto"
	"github.com/golang/snappy"
	"github.com/prometheus/common/model"
	"go.opentelemetry.io/collector/pdata/pmetric/pmetricotlp"

	"github.com/prometheus/prometheus/model/histogram"
	"github.com/prometheus/prometheus/model/labels"
	"github.com/prometheus/prometheus/prompb"
	writev2 "github.com/prometheus/prometheus/prompb/io/prometheus/write/v2"
	"github.com/prometheus/prometheus/storage"
	"github.com/prometheus/prometheus/tsdb/chunkenc"
	"github.com/prometheus/prometheus/tsdb/chunks"
	"github.com/prometheus/prometheus/util/annotations"
)

const (
	// decodeReadLimit is the maximum size of a read request body in bytes.
	decodeReadLimit = 32 * 1024 * 1024

	pbContentType   = "application/x-protobuf"
	jsonContentType = "application/json"
)

type HTTPError struct {
	msg    string
	status int
}

func (e HTTPError) Error() string {
	return e.msg
}

func (e HTTPError) Status() int {
	return e.status
}

// DecodeReadRequest reads a remote.Request from a http.Request.
func DecodeReadRequest(r *http.Request) (*prompb.ReadRequest, error) {
	compressed, err := io.ReadAll(io.LimitReader(r.Body, decodeReadLimit))
	if err != nil {
		return nil, err
	}

	reqBuf, err := snappy.Decode(nil, compressed)
	if err != nil {
		return nil, err
	}

	var req prompb.ReadRequest
	if err := proto.Unmarshal(reqBuf, &req); err != nil {
		return nil, err
	}

	return &req, nil
}

// EncodeReadResponse writes a remote.Response to a http.ResponseWriter.
func EncodeReadResponse(resp *prompb.ReadResponse, w http.ResponseWriter) error {
	data, err := proto.Marshal(resp)
	if err != nil {
		return err
	}

	compressed := snappy.Encode(nil, data)
	_, err = w.Write(compressed)
	return err
}

// ToQuery builds a Query proto.
func ToQuery(from, to int64, matchers []*labels.Matcher, hints *storage.SelectHints) (*prompb.Query, error) {
	ms, err := ToLabelMatchers(matchers)
	if err != nil {
		return nil, err
	}

	var rp *prompb.ReadHints
	if hints != nil {
		rp = &prompb.ReadHints{
			StartMs:  hints.Start,
			EndMs:    hints.End,
			StepMs:   hints.Step,
			Func:     hints.Func,
			Grouping: hints.Grouping,
			By:       hints.By,
			RangeMs:  hints.Range,
		}
	}

	return &prompb.Query{
		StartTimestampMs: from,
		EndTimestampMs:   to,
		Matchers:         ms,
		Hints:            rp,
	}, nil
}

// ToQueryResult builds a QueryResult proto.
func ToQueryResult(ss storage.SeriesSet, sampleLimit int) (*prompb.QueryResult, annotations.Annotations, error) {
	numSamples := 0
	resp := &prompb.QueryResult{}
	var iter chunkenc.Iterator
	for ss.Next() {
		series := ss.At()
		iter = series.Iterator(iter)

		var (
			samples    []prompb.Sample
			histograms []prompb.Histogram
		)

		for valType := iter.Next(); valType != chunkenc.ValNone; valType = iter.Next() {
			numSamples++
			if sampleLimit > 0 && numSamples > sampleLimit {
				return nil, ss.Warnings(), HTTPError{
					msg:    fmt.Sprintf("exceeded sample limit (%d)", sampleLimit),
					status: http.StatusBadRequest,
				}
			}

			switch valType {
			case chunkenc.ValFloat:
				ts, val := iter.At()
				samples = append(samples, prompb.Sample{
					Timestamp: ts,
					Value:     val,
				})
			case chunkenc.ValHistogram:
				ts, h := iter.AtHistogram(nil)
				histograms = append(histograms, prompb.FromIntHistogram(ts, h))
			case chunkenc.ValFloatHistogram:
				ts, fh := iter.AtFloatHistogram(nil)
				histograms = append(histograms, prompb.FromFloatHistogram(ts, fh))
			default:
				return nil, ss.Warnings(), fmt.Errorf("unrecognized value type: %s", valType)
			}
		}
		if err := iter.Err(); err != nil {
			return nil, ss.Warnings(), err
		}

		resp.Timeseries = append(resp.Timeseries, &prompb.TimeSeries{
			Labels:     prompb.FromLabels(series.Labels(), nil),
			Samples:    samples,
			Histograms: histograms,
		})
	}
	return resp, ss.Warnings(), ss.Err()
}

// FromQueryResult unpacks and sorts a QueryResult proto.
func FromQueryResult(sortSeries bool, res *prompb.QueryResult) storage.SeriesSet {
	b := labels.NewScratchBuilder(0)
	series := make([]storage.Series, 0, len(res.Timeseries))
	for _, ts := range res.Timeseries {
		if err := validateLabelsAndMetricName(ts.Labels); err != nil {
			return errSeriesSet{err: err}
		}
		lbls := ts.ToLabels(&b, nil)
		series = append(series, &concreteSeries{labels: lbls, floats: ts.Samples, histograms: ts.Histograms})
	}

	if sortSeries {
		slices.SortFunc(series, func(a, b storage.Series) int {
			return labels.Compare(a.Labels(), b.Labels())
		})
	}
	return &concreteSeriesSet{
		series: series,
	}
}

// NegotiateResponseType returns first accepted response type that this server supports.
// On the empty accepted list we assume that the SAMPLES response type was requested. This is to maintain backward compatibility.
func NegotiateResponseType(accepted []prompb.ReadRequest_ResponseType) (prompb.ReadRequest_ResponseType, error) {
	if len(accepted) == 0 {
		accepted = []prompb.ReadRequest_ResponseType{prompb.ReadRequest_SAMPLES}
	}

	supported := map[prompb.ReadRequest_ResponseType]struct{}{
		prompb.ReadRequest_SAMPLES:             {},
		prompb.ReadRequest_STREAMED_XOR_CHUNKS: {},
	}

	for _, resType := range accepted {
		if _, ok := supported[resType]; ok {
			return resType, nil
		}
	}
	return 0, fmt.Errorf("server does not support any of the requested response types: %v; supported: %v", accepted, supported)
}

// StreamChunkedReadResponses iterates over series, builds chunks and streams those to the caller.
// It expects Series set with populated chunks.
func StreamChunkedReadResponses(
	stream io.Writer,
	queryIndex int64,
	ss storage.ChunkSeriesSet,
	sortedExternalLabels []prompb.Label,
	maxBytesInFrame int,
	marshalPool *sync.Pool,
) (annotations.Annotations, error) {
	var (
		chks []prompb.Chunk
		lbls []prompb.Label
		iter chunks.Iterator
	)

	for ss.Next() {
		series := ss.At()
		iter = series.Iterator(iter)
		lbls = MergeLabels(prompb.FromLabels(series.Labels(), lbls), sortedExternalLabels)

		maxDataLength := maxBytesInFrame
		for _, lbl := range lbls {
			maxDataLength -= lbl.Size()
		}
		frameBytesLeft := maxDataLength

		isNext := iter.Next()

		// Send at most one series per frame; series may be split over multiple frames according to maxBytesInFrame.
		for isNext {
			chk := iter.At()

			if chk.Chunk == nil {
				return ss.Warnings(), fmt.Errorf("StreamChunkedReadResponses: found not populated chunk returned by SeriesSet at ref: %v", chk.Ref)
			}

			// Cut the chunk.
			chks = append(chks, prompb.Chunk{
				MinTimeMs: chk.MinTime,
				MaxTimeMs: chk.MaxTime,
				Type:      prompb.Chunk_Encoding(chk.Chunk.Encoding()),
				Data:      chk.Chunk.Bytes(),
			})
			frameBytesLeft -= chks[len(chks)-1].Size()

			// We are fine with minor inaccuracy of max bytes per frame. The inaccuracy will be max of full chunk size.
			isNext = iter.Next()
			if frameBytesLeft > 0 && isNext {
				continue
			}

			resp := &prompb.ChunkedReadResponse{
				ChunkedSeries: []*prompb.ChunkedSeries{
					{Labels: lbls, Chunks: chks},
				},
				QueryIndex: queryIndex,
			}

			b, err := resp.PooledMarshal(marshalPool)
			if err != nil {
				return ss.Warnings(), fmt.Errorf("marshal ChunkedReadResponse: %w", err)
			}

			if _, err := stream.Write(b); err != nil {
				return ss.Warnings(), fmt.Errorf("write to stream: %w", err)
			}

			// We immediately flush the Write() so it is safe to return to the pool.
			marshalPool.Put(&b)
			chks = chks[:0]
			frameBytesLeft = maxDataLength
		}
		if err := iter.Err(); err != nil {
			return ss.Warnings(), err
		}
	}
	return ss.Warnings(), ss.Err()
}

// MergeLabels merges two sets of sorted proto labels, preferring those in
// primary to those in secondary when there is an overlap.
func MergeLabels(primary, secondary []prompb.Label) []prompb.Label {
	result := make([]prompb.Label, 0, len(primary)+len(secondary))
	i, j := 0, 0
	for i < len(primary) && j < len(secondary) {
		switch {
		case primary[i].Name < secondary[j].Name:
			result = append(result, primary[i])
			i++
		case primary[i].Name > secondary[j].Name:
			result = append(result, secondary[j])
			j++
		default:
			result = append(result, primary[i])
			i++
			j++
		}
	}
	for ; i < len(primary); i++ {
		result = append(result, primary[i])
	}
	for ; j < len(secondary); j++ {
		result = append(result, secondary[j])
	}
	return result
}

// errSeriesSet implements storage.SeriesSet, just returning an error.
type errSeriesSet struct {
	err error
}

func (errSeriesSet) Next() bool {
	return false
}

func (errSeriesSet) At() storage.Series {
	return nil
}

func (e errSeriesSet) Err() error {
	return e.err
}

func (errSeriesSet) Warnings() annotations.Annotations { return nil }

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

func (*concreteSeriesSet) Err() error {
	return nil
}

func (*concreteSeriesSet) Warnings() annotations.Annotations { return nil }

// concreteSeries implements storage.Series.
type concreteSeries struct {
	labels     labels.Labels
	floats     []prompb.Sample
	histograms []prompb.Histogram
}

func (c *concreteSeries) Labels() labels.Labels {
	return c.labels.Copy()
}

func (c *concreteSeries) Iterator(it chunkenc.Iterator) chunkenc.Iterator {
	if csi, ok := it.(*concreteSeriesIterator); ok {
		csi.reset(c)
		return csi
	}
	return newConcreteSeriesIterator(c)
}

// concreteSeriesIterator implements storage.SeriesIterator.
type concreteSeriesIterator struct {
	floatsCur     int
	histogramsCur int
	curValType    chunkenc.ValueType
	series        *concreteSeries
	err           error

	// These are pre-filled with the current model histogram if curValType
	// is ValHistogram or ValFloatHistogram, respectively.
	curH  *histogram.Histogram
	curFH *histogram.FloatHistogram
}

func newConcreteSeriesIterator(series *concreteSeries) chunkenc.Iterator {
	return &concreteSeriesIterator{
		floatsCur:     -1,
		histogramsCur: -1,
		curValType:    chunkenc.ValNone,
		series:        series,
	}
}

func (c *concreteSeriesIterator) reset(series *concreteSeries) {
	c.floatsCur = -1
	c.histogramsCur = -1
	c.curValType = chunkenc.ValNone
	c.series = series
	c.err = nil
}

// Seek implements storage.SeriesIterator.
func (c *concreteSeriesIterator) Seek(t int64) chunkenc.ValueType {
	if c.err != nil {
		return chunkenc.ValNone
	}
	if c.floatsCur == -1 {
		c.floatsCur = 0
	}
	if c.histogramsCur == -1 {
		c.histogramsCur = 0
	}
	if c.floatsCur >= len(c.series.floats) && c.histogramsCur >= len(c.series.histograms) {
		return chunkenc.ValNone
	}

	// No-op check.
	if (c.curValType == chunkenc.ValFloat && c.series.floats[c.floatsCur].Timestamp >= t) ||
		((c.curValType == chunkenc.ValHistogram || c.curValType == chunkenc.ValFloatHistogram) && c.series.histograms[c.histogramsCur].Timestamp >= t) {
		return c.curValType
	}

	c.curValType = chunkenc.ValNone

	// Binary search between current position and end for both float and histograms samples.
	c.floatsCur += sort.Search(len(c.series.floats)-c.floatsCur, func(n int) bool {
		return c.series.floats[n+c.floatsCur].Timestamp >= t
	})
	c.histogramsCur += sort.Search(len(c.series.histograms)-c.histogramsCur, func(n int) bool {
		return c.series.histograms[n+c.histogramsCur].Timestamp >= t
	})
	switch {
	case c.floatsCur < len(c.series.floats) && c.histogramsCur < len(c.series.histograms):
		// If float samples and histogram samples have overlapping timestamps prefer the float samples.
		if c.series.floats[c.floatsCur].Timestamp <= c.series.histograms[c.histogramsCur].Timestamp {
			c.curValType = chunkenc.ValFloat
		} else {
			c.curValType = chunkenc.ValHistogram
		}
		// When the timestamps do not overlap the cursor for the non-selected sample type has advanced too
		// far; we decrement it back down here.
		if c.series.floats[c.floatsCur].Timestamp != c.series.histograms[c.histogramsCur].Timestamp {
			if c.curValType == chunkenc.ValFloat {
				c.histogramsCur--
			} else {
				c.floatsCur--
			}
		}
	case c.floatsCur < len(c.series.floats):
		c.curValType = chunkenc.ValFloat
	case c.histogramsCur < len(c.series.histograms):
		c.curValType = chunkenc.ValHistogram
	}
	if c.curValType == chunkenc.ValHistogram {
		c.setCurrentHistogram()
	}
	if c.err != nil {
		c.curValType = chunkenc.ValNone
	}
	return c.curValType
}

// setCurrentHistogram pre-fills either the curH or the curFH field with a
// converted model histogram and sets c.curValType accordingly. It validates the
// histogram and sets c.err accordingly. This all has to be done in Seek() and
// Next() already so that we know if the histogram we got from the remote-read
// source is valid or not before we allow the AtHistogram()/AtFloatHistogram()
// call.
func (c *concreteSeriesIterator) setCurrentHistogram() {
	pbH := c.series.histograms[c.histogramsCur]

	// Basic schema check first.
	schema := pbH.Schema
	if !histogram.IsKnownSchema(schema) {
		c.err = histogram.UnknownSchemaError(schema)
		return
	}

	if pbH.IsFloatHistogram() {
		c.curValType = chunkenc.ValFloatHistogram
		mFH := pbH.ToFloatHistogram()
		if mFH.Schema > histogram.ExponentialSchemaMax && mFH.Schema <= histogram.ExponentialSchemaMaxReserved {
			// This is a very slow path, but it should only happen if the
			// sample is from a newer Prometheus version that supports higher
			// resolution.
			if err := mFH.ReduceResolution(histogram.ExponentialSchemaMax); err != nil {
				c.err = err
				return
			}
		}
		if err := mFH.Validate(); err != nil {
			c.err = err
			return
		}
		c.curFH = mFH
		return
	}
	c.curValType = chunkenc.ValHistogram
	mH := pbH.ToIntHistogram()
	if mH.Schema > histogram.ExponentialSchemaMax && mH.Schema <= histogram.ExponentialSchemaMaxReserved {
		// This is a very slow path, but it should only happen if the
		// sample is from a newer Prometheus version that supports higher
		// resolution.
		if err := mH.ReduceResolution(histogram.ExponentialSchemaMax); err != nil {
			c.err = err
			return
		}
	}
	if err := mH.Validate(); err != nil {
		c.err = err
		return
	}
	c.curH = mH
}

// At implements chunkenc.Iterator.
func (c *concreteSeriesIterator) At() (t int64, v float64) {
	if c.curValType != chunkenc.ValFloat {
		panic("iterator is not on a float sample")
	}
	s := c.series.floats[c.floatsCur]
	return s.Timestamp, s.Value
}

// AtHistogram implements chunkenc.Iterator.
func (c *concreteSeriesIterator) AtHistogram(*histogram.Histogram) (int64, *histogram.Histogram) {
	if c.curValType != chunkenc.ValHistogram {
		panic("iterator is not on an integer histogram sample")
	}
	return c.series.histograms[c.histogramsCur].Timestamp, c.curH
}

// AtFloatHistogram implements chunkenc.Iterator.
func (c *concreteSeriesIterator) AtFloatHistogram(*histogram.FloatHistogram) (int64, *histogram.FloatHistogram) {
	switch c.curValType {
	case chunkenc.ValFloatHistogram:
		return c.series.histograms[c.histogramsCur].Timestamp, c.curFH
	case chunkenc.ValHistogram:
		return c.series.histograms[c.histogramsCur].Timestamp, c.curH.ToFloat(nil)
	default:
		panic("iterator is not on a histogram sample")
	}
}

// AtT implements chunkenc.Iterator.
func (c *concreteSeriesIterator) AtT() int64 {
	if c.curValType == chunkenc.ValHistogram || c.curValType == chunkenc.ValFloatHistogram {
		return c.series.histograms[c.histogramsCur].Timestamp
	}
	return c.series.floats[c.floatsCur].Timestamp
}

const noTS = int64(math.MaxInt64)

// Next implements chunkenc.Iterator.
func (c *concreteSeriesIterator) Next() chunkenc.ValueType {
	if c.err != nil {
		return chunkenc.ValNone
	}
	peekFloatTS := noTS
	if c.floatsCur+1 < len(c.series.floats) {
		peekFloatTS = c.series.floats[c.floatsCur+1].Timestamp
	}
	peekHistTS := noTS
	if c.histogramsCur+1 < len(c.series.histograms) {
		peekHistTS = c.series.histograms[c.histogramsCur+1].Timestamp
	}
	c.curValType = chunkenc.ValNone
	switch {
	case peekFloatTS < peekHistTS:
		c.floatsCur++
		c.curValType = chunkenc.ValFloat
	case peekHistTS < peekFloatTS:
		c.histogramsCur++
		c.curValType = chunkenc.ValHistogram
	case peekFloatTS == noTS && peekHistTS == noTS:
		// This only happens when the iterator is exhausted; we set the cursors off the end to prevent
		// Seek() from returning anything afterwards.
		c.floatsCur = len(c.series.floats)
		c.histogramsCur = len(c.series.histograms)
	default:
		// Prefer float samples to histogram samples if there's a conflict. We advance the cursor for histograms
		// anyway otherwise the histogram sample will get selected on the next call to Next().
		c.floatsCur++
		c.histogramsCur++
		c.curValType = chunkenc.ValFloat
	}

	if c.curValType == chunkenc.ValHistogram {
		c.setCurrentHistogram()
	}
	if c.err != nil {
		c.curValType = chunkenc.ValNone
	}
	return c.curValType
}

// Err implements chunkenc.Iterator.
func (c *concreteSeriesIterator) Err() error {
	return c.err
}

// chunkedSeriesSet implements storage.SeriesSet.
type chunkedSeriesSet struct {
	chunkedReader *ChunkedReader
	respBody      io.ReadCloser
	mint, maxt    int64
	cancel        func(error)

	current   storage.Series
	err       error
	exhausted bool
}

func NewChunkedSeriesSet(chunkedReader *ChunkedReader, respBody io.ReadCloser, mint, maxt int64, cancel func(error)) storage.SeriesSet {
	return &chunkedSeriesSet{
		chunkedReader: chunkedReader,
		respBody:      respBody,
		mint:          mint,
		maxt:          maxt,
		cancel:        cancel,
	}
}

// Next return true if there is a next series and false otherwise. It will
// block until the next series is available.
func (s *chunkedSeriesSet) Next() bool {
	if s.exhausted {
		// Don't try to read the next series again.
		// This prevents errors like "http: read on closed response body" if Next() is called after it has already returned false.
		return false
	}

	res := &prompb.ChunkedReadResponse{}

	err := s.chunkedReader.NextProto(res)
	if err != nil {
		if !errors.Is(err, io.EOF) {
			s.err = err
			_, _ = io.Copy(io.Discard, s.respBody)
		}

		_ = s.respBody.Close()
		s.cancel(err)
		s.exhausted = true

		return false
	}

	s.current = &chunkedSeries{
		ChunkedSeries: prompb.ChunkedSeries{
			Labels: res.ChunkedSeries[0].Labels,
			Chunks: res.ChunkedSeries[0].Chunks,
		},
		mint: s.mint,
		maxt: s.maxt,
	}

	return true
}

func (s *chunkedSeriesSet) At() storage.Series {
	return s.current
}

func (s *chunkedSeriesSet) Err() error {
	return s.err
}

func (*chunkedSeriesSet) Warnings() annotations.Annotations {
	return nil
}

type chunkedSeries struct {
	prompb.ChunkedSeries
	mint, maxt int64
}

var _ storage.Series = &chunkedSeries{}

func (s *chunkedSeries) Labels() labels.Labels {
	b := labels.NewScratchBuilder(0)
	return s.ToLabels(&b, nil)
}

func (s *chunkedSeries) Iterator(it chunkenc.Iterator) chunkenc.Iterator {
	csIt, ok := it.(*chunkedSeriesIterator)
	if ok {
		csIt.reset(s.Chunks, s.mint, s.maxt)
		return csIt
	}
	return newChunkedSeriesIterator(s.Chunks, s.mint, s.maxt)
}

type chunkedSeriesIterator struct {
	chunks     []prompb.Chunk
	idx        int
	cur        chunkenc.Iterator
	valType    chunkenc.ValueType
	mint, maxt int64

	err error
}

var _ chunkenc.Iterator = &chunkedSeriesIterator{}

func newChunkedSeriesIterator(chunks []prompb.Chunk, mint, maxt int64) *chunkedSeriesIterator {
	it := &chunkedSeriesIterator{}
	it.reset(chunks, mint, maxt)
	return it
}

func (it *chunkedSeriesIterator) Next() chunkenc.ValueType {
	if it.err != nil {
		return chunkenc.ValNone
	}
	if len(it.chunks) == 0 {
		return chunkenc.ValNone
	}

	for it.valType = it.cur.Next(); it.valType != chunkenc.ValNone; it.valType = it.cur.Next() {
		atT := it.AtT()
		if atT > it.maxt {
			it.chunks = nil // Exhaust this iterator so follow-up calls to Next or Seek return fast.
			return chunkenc.ValNone
		}
		if atT >= it.mint {
			return it.valType
		}
	}

	if it.idx >= len(it.chunks)-1 {
		it.valType = chunkenc.ValNone
	} else {
		it.idx++
		it.resetIterator()
		it.valType = it.Next()
	}

	return it.valType
}

func (it *chunkedSeriesIterator) Seek(t int64) chunkenc.ValueType {
	if it.err != nil {
		return chunkenc.ValNone
	}
	if len(it.chunks) == 0 {
		return chunkenc.ValNone
	}

	startIdx := it.idx
	it.idx += sort.Search(len(it.chunks)-startIdx, func(i int) bool {
		return it.chunks[startIdx+i].MaxTimeMs >= t
	})
	if it.idx > startIdx {
		it.resetIterator()
	} else {
		ts := it.cur.AtT()
		if ts >= t {
			return it.valType
		}
	}

	for it.valType = it.cur.Next(); it.valType != chunkenc.ValNone; it.valType = it.cur.Next() {
		ts := it.cur.AtT()
		if ts > it.maxt {
			it.chunks = nil // Exhaust this iterator so follow-up calls to Next or Seek return fast.
			return chunkenc.ValNone
		}
		if ts >= t && ts >= it.mint {
			return it.valType
		}
	}

	it.valType = chunkenc.ValNone
	return it.valType
}

func (it *chunkedSeriesIterator) resetIterator() {
	if it.idx < len(it.chunks) {
		chunk := it.chunks[it.idx]

		decodedChunk, err := chunkenc.FromData(chunkenc.Encoding(chunk.Type), chunk.Data)
		if err != nil {
			it.err = err
			return
		}

		it.cur = decodedChunk.Iterator(nil)
	} else {
		it.cur = chunkenc.NewNopIterator()
	}
}

func (it *chunkedSeriesIterator) reset(chunks []prompb.Chunk, mint, maxt int64) {
	it.chunks = chunks
	it.mint = mint
	it.maxt = maxt
	it.idx = 0
	if len(chunks) > 0 {
		it.resetIterator()
	}
}

func (it *chunkedSeriesIterator) At() (ts int64, v float64) {
	return it.cur.At()
}

func (it *chunkedSeriesIterator) AtHistogram(h *histogram.Histogram) (int64, *histogram.Histogram) {
	return it.cur.AtHistogram(h)
}

func (it *chunkedSeriesIterator) AtFloatHistogram(fh *histogram.FloatHistogram) (int64, *histogram.FloatHistogram) {
	return it.cur.AtFloatHistogram(fh)
}

func (it *chunkedSeriesIterator) AtT() int64 {
	return it.cur.AtT()
}

func (it *chunkedSeriesIterator) Err() error {
	return it.err
}

// validateLabelsAndMetricName validates the label names/values and metric names returned from remote read,
// also making sure that there are no labels with duplicate names.
func validateLabelsAndMetricName(ls []prompb.Label) error {
	for i, l := range ls {
		if l.Name == labels.MetricName && !model.UTF8Validation.IsValidMetricName(l.Value) {
			return fmt.Errorf("invalid metric name: %v", l.Value)
		}
		if !model.UTF8Validation.IsValidLabelName(l.Name) {
			return fmt.Errorf("invalid label name: %v", l.Name)
		}
		if !model.LabelValue(l.Value).IsValid() {
			return fmt.Errorf("invalid label value: %v", l.Value)
		}
		if i > 0 && l.Name == ls[i-1].Name {
			return fmt.Errorf("duplicate label with name: %v", l.Name)
		}
	}
	return nil
}

// ToLabelMatchers converts Prometheus label matchers to protobuf label matchers.
func ToLabelMatchers(matchers []*labels.Matcher) ([]*prompb.LabelMatcher, error) {
	pbMatchers := make([]*prompb.LabelMatcher, 0, len(matchers))
	for _, m := range matchers {
		var mType prompb.LabelMatcher_Type
		switch m.Type {
		case labels.MatchEqual:
			mType = prompb.LabelMatcher_EQ
		case labels.MatchNotEqual:
			mType = prompb.LabelMatcher_NEQ
		case labels.MatchRegexp:
			mType = prompb.LabelMatcher_RE
		case labels.MatchNotRegexp:
			mType = prompb.LabelMatcher_NRE
		default:
			return nil, errors.New("invalid matcher type")
		}
		pbMatchers = append(pbMatchers, &prompb.LabelMatcher{
			Type:  mType,
			Name:  m.Name,
			Value: m.Value,
		})
	}
	return pbMatchers, nil
}

// FromLabelMatchers converts protobuf label matchers to Prometheus label matchers.
func FromLabelMatchers(matchers []*prompb.LabelMatcher) ([]*labels.Matcher, error) {
	result := make([]*labels.Matcher, 0, len(matchers))
	for _, matcher := range matchers {
		var mtype labels.MatchType
		switch matcher.Type {
		case prompb.LabelMatcher_EQ:
			mtype = labels.MatchEqual
		case prompb.LabelMatcher_NEQ:
			mtype = labels.MatchNotEqual
		case prompb.LabelMatcher_RE:
			mtype = labels.MatchRegexp
		case prompb.LabelMatcher_NRE:
			mtype = labels.MatchNotRegexp
		default:
			return nil, errors.New("invalid matcher type")
		}
		matcher, err := labels.NewMatcher(mtype, matcher.Name, matcher.Value)
		if err != nil {
			return nil, err
		}
		result = append(result, matcher)
	}
	return result, nil
}

// DecodeWriteRequest from an io.Reader into a prompb.WriteRequest, handling
// snappy decompression.
// Used also by documentation/examples/remote_storage.
func DecodeWriteRequest(r io.Reader) (*prompb.WriteRequest, error) {
	compressed, err := io.ReadAll(r)
	if err != nil {
		return nil, err
	}

	reqBuf, err := snappy.Decode(nil, compressed)
	if err != nil {
		return nil, err
	}

	var req prompb.WriteRequest
	if err := proto.Unmarshal(reqBuf, &req); err != nil {
		return nil, err
	}

	return &req, nil
}

// DecodeWriteV2Request from an io.Reader into a writev2.Request, handling
// snappy decompression.
// Used also by documentation/examples/remote_storage.
func DecodeWriteV2Request(r io.Reader) (*writev2.Request, error) {
	compressed, err := io.ReadAll(r)
	if err != nil {
		return nil, err
	}

	reqBuf, err := snappy.Decode(nil, compressed)
	if err != nil {
		return nil, err
	}

	var req writev2.Request
	if err := proto.Unmarshal(reqBuf, &req); err != nil {
		return nil, err
	}

	return &req, nil
}

func DecodeOTLPWriteRequest(r *http.Request) (pmetricotlp.ExportRequest, error) {
	contentType := r.Header.Get("Content-Type")
	var decoderFunc func(buf []byte) (pmetricotlp.ExportRequest, error)
	switch contentType {
	case pbContentType:
		decoderFunc = func(buf []byte) (pmetricotlp.ExportRequest, error) {
			req := pmetricotlp.NewExportRequest()
			return req, req.UnmarshalProto(buf)
		}

	case jsonContentType:
		decoderFunc = func(buf []byte) (pmetricotlp.ExportRequest, error) {
			req := pmetricotlp.NewExportRequest()
			return req, req.UnmarshalJSON(buf)
		}

	default:
		return pmetricotlp.NewExportRequest(), fmt.Errorf("unsupported content type: %s, supported: [%s, %s]", contentType, jsonContentType, pbContentType)
	}

	reader := r.Body
	// Handle compression.
	switch r.Header.Get("Content-Encoding") {
	case "gzip":
		gr, err := gzip.NewReader(reader)
		if err != nil {
			return pmetricotlp.NewExportRequest(), err
		}
		reader = gr

	case "":
		// No compression.

	default:
		return pmetricotlp.NewExportRequest(), fmt.Errorf("unsupported compression: %s. Only \"gzip\" or no compression supported", r.Header.Get("Content-Encoding"))
	}

	body, err := io.ReadAll(reader)
	if err != nil {
		r.Body.Close()
		return pmetricotlp.NewExportRequest(), err
	}
	if err = r.Body.Close(); err != nil {
		return pmetricotlp.NewExportRequest(), err
	}
	otlpReq, err := decoderFunc(body)
	if err != nil {
		return pmetricotlp.NewExportRequest(), err
	}

	return otlpReq, nil
}
