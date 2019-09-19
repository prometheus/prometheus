// Copyright 2017 The Prometheus Authors
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
	"strings"

	"github.com/gogo/protobuf/proto"
	"github.com/golang/snappy"
	"github.com/pkg/errors"
	"github.com/prometheus/common/model"
	"github.com/prometheus/prometheus/pkg/labels"
	"github.com/prometheus/prometheus/prompb"
	"github.com/prometheus/prometheus/storage"
	"github.com/prometheus/prometheus/tsdb/chunkenc"
)

// decodeReadLimit is the maximum size of a read request body in bytes.
const decodeReadLimit = 32 * 1024 * 1024

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
	compressed, err := ioutil.ReadAll(io.LimitReader(r.Body, decodeReadLimit))
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
func ToQuery(from, to int64, matchers []*labels.Matcher, p *storage.SelectParams) (*prompb.Query, error) {
	ms, err := toLabelMatchers(matchers)
	if err != nil {
		return nil, err
	}

	var rp *prompb.ReadHints
	if p != nil {
		rp = &prompb.ReadHints{
			StepMs:  p.Step,
			Func:    p.Func,
			StartMs: p.Start,
			EndMs:   p.End,
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
func ToQueryResult(ss storage.SeriesSet, sampleLimit int) (*prompb.QueryResult, error) {
	numSamples := 0
	resp := &prompb.QueryResult{}
	for ss.Next() {
		series := ss.At()
		iter := series.Iterator()
		samples := []prompb.Sample{}

		for iter.Next() {
			numSamples++
			if sampleLimit > 0 && numSamples > sampleLimit {
				return nil, HTTPError{
					msg:    fmt.Sprintf("exceeded sample limit (%d)", sampleLimit),
					status: http.StatusBadRequest,
				}
			}
			ts, val := iter.At()
			samples = append(samples, prompb.Sample{
				Timestamp: ts,
				Value:     val,
			})
		}
		if err := iter.Err(); err != nil {
			return nil, err
		}

		resp.Timeseries = append(resp.Timeseries, &prompb.TimeSeries{
			Labels:  labelsToLabelsProto(series.Labels(), nil),
			Samples: samples,
		})
	}
	if err := ss.Err(); err != nil {
		return nil, err
	}
	return resp, nil
}

// FromQueryResult unpacks a QueryResult proto.
func FromQueryResult(res *prompb.QueryResult) storage.SeriesSet {
	series := make([]storage.Series, 0, len(res.Timeseries))
	for _, ts := range res.Timeseries {
		labels := labelProtosToLabels(ts.Labels)
		if err := validateLabelsAndMetricName(labels); err != nil {
			return errSeriesSet{err: err}
		}

		series = append(series, &concreteSeries{
			labels:  labels,
			samples: ts.Samples,
		})
	}
	sort.Sort(byLabel(series))
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
	return 0, errors.Errorf("server does not support any of the requested response types: %v; supported: %v", accepted, supported)
}

// StreamChunkedReadResponses iterates over series, builds chunks and streams those to the caller.
// TODO(bwplotka): Encode only what's needed. Fetch the encoded series from blocks instead of re-encoding everything.
func StreamChunkedReadResponses(
	stream io.Writer,
	queryIndex int64,
	ss storage.SeriesSet,
	sortedExternalLabels []prompb.Label,
	maxBytesInFrame int,
) error {
	var (
		chks     []prompb.Chunk
		lbls     []prompb.Label
		err      error
		lblsSize int
	)

	for ss.Next() {
		series := ss.At()
		iter := series.Iterator()
		lbls = MergeLabels(labelsToLabelsProto(series.Labels(), lbls), sortedExternalLabels)

		lblsSize = 0
		for _, lbl := range lbls {
			lblsSize += lbl.Size()
		}

		// Send at most one series per frame; series may be split over multiple frames according to maxBytesInFrame.
		for {
			// TODO(bwplotka): Use ChunkIterator once available in TSDB instead of re-encoding: https://github.com/prometheus/prometheus/pull/5882
			chks, err = encodeChunks(iter, chks, maxBytesInFrame-lblsSize)
			if err != nil {
				return err
			}

			if len(chks) == 0 {
				break
			}

			b, err := proto.Marshal(&prompb.ChunkedReadResponse{
				ChunkedSeries: []*prompb.ChunkedSeries{
					{
						Labels: lbls,
						Chunks: chks,
					},
				},
				QueryIndex: queryIndex,
			})
			if err != nil {
				return errors.Wrap(err, "marshal ChunkedReadResponse")
			}

			if _, err := stream.Write(b); err != nil {
				return errors.Wrap(err, "write to stream")
			}

			chks = chks[:0]
		}

		if err := iter.Err(); err != nil {
			return err
		}
	}
	if err := ss.Err(); err != nil {
		return err
	}

	return nil
}

// encodeChunks expects iterator to be ready to use (aka iter.Next() called before invoking).
func encodeChunks(iter storage.SeriesIterator, chks []prompb.Chunk, frameBytesLeft int) ([]prompb.Chunk, error) {
	const maxSamplesInChunk = 120

	var (
		chkMint int64
		chkMaxt int64
		chk     *chunkenc.XORChunk
		app     chunkenc.Appender
		err     error
	)

	for iter.Next() {
		if chk == nil {
			chk = chunkenc.NewXORChunk()
			app, err = chk.Appender()
			if err != nil {
				return nil, err
			}
			chkMint, _ = iter.At()
		}

		app.Append(iter.At())
		chkMaxt, _ = iter.At()

		if chk.NumSamples() < maxSamplesInChunk {
			continue
		}

		// Cut the chunk.
		chks = append(chks, prompb.Chunk{
			MinTimeMs: chkMint,
			MaxTimeMs: chkMaxt,
			Type:      prompb.Chunk_Encoding(chk.Encoding()),
			Data:      chk.Bytes(),
		})
		chk = nil
		frameBytesLeft -= chks[len(chks)-1].Size()

		// We are fine with minor inaccuracy of max bytes per frame. The inaccuracy will be max of full chunk size.
		if frameBytesLeft <= 0 {
			break
		}
	}
	if iter.Err() != nil {
		return nil, errors.Wrap(iter.Err(), "iter TSDB series")
	}

	if chk != nil {
		// Cut the chunk if exists.
		chks = append(chks, prompb.Chunk{
			MinTimeMs: chkMint,
			MaxTimeMs: chkMaxt,
			Type:      prompb.Chunk_Encoding(chk.Encoding()),
			Data:      chk.Bytes(),
		})
	}
	return chks, nil
}

// MergeLabels merges two sets of sorted proto labels, preferring those in
// primary to those in secondary when there is an overlap.
func MergeLabels(primary, secondary []prompb.Label) []prompb.Label {
	result := make([]prompb.Label, 0, len(primary)+len(secondary))
	i, j := 0, 0
	for i < len(primary) && j < len(secondary) {
		if primary[i].Name < secondary[j].Name {
			result = append(result, primary[i])
			i++
		} else if primary[i].Name > secondary[j].Name {
			result = append(result, secondary[j])
			j++
		} else {
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

type byLabel []storage.Series

func (a byLabel) Len() int           { return len(a) }
func (a byLabel) Swap(i, j int)      { a[i], a[j] = a[j], a[i] }
func (a byLabel) Less(i, j int) bool { return labels.Compare(a[i].Labels(), a[j].Labels()) < 0 }

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

// errSeriesIterator implements storage.SeriesIterator, just returning an error.
type errSeriesIterator struct {
	err error
}

func (it *errSeriesIterator) Seek(int64) bool      { return false }
func (it *errSeriesIterator) Next() bool           { return false }
func (it *errSeriesIterator) At() (int64, float64) { return 0, 0 }
func (it *errSeriesIterator) Err() error           { return it.err }

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
	samples []prompb.Sample
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

// chunkSeriesSet implements storage.SeriesSet.
type chunkSeriesSet struct {
	stream    *ChunkedReader
	initiated bool
	done      bool
	err       error

	mint, maxt int64

	lastSeries string
	currLset   []prompb.Label
	currChunks []prompb.Chunk

	closer io.Closer
}

func (c *chunkSeriesSet) Next() bool {
	var (
		err error
		tmp []string
	)

	defer func() {
		if err != nil {
			c.closer.Close()
			c.done = true
			if err != io.EOF {
				c.err = err
			}
		}
	}()

	res := &prompb.ChunkedReadResponse{}
	if !c.initiated {
		c.initiated = true
		err := c.stream.NextProto(res)
		if err == io.EOF {
			return false
		}
		if err != nil {
			return false
		}

		if len(res.ChunkedSeries) == 0 {
			err = errors.Errorf("length of ChunkedSeries in ChunkedReadResponse should not be 0")
		}
		if err != nil {
			return false
		}

		for _, l := range res.ChunkedSeries[0].Labels {
			tmp = append(tmp, l.String())
		}
		currSeries := strings.Join(tmp, ";")
		c.lastSeries = currSeries
		c.currLset = res.ChunkedSeries[0].Labels
	}

	if c.done {
		return false
	}

	c.currChunks = c.currChunks[:0]

	for {
		err = c.stream.AtProto(res)
		if err != nil {
			return false
		}

		if len(res.ChunkedSeries) == 0 {
			err = errors.Errorf("length of ChunkedSeries in ChunkedReadResponse should not be 0")
		}
		if err != nil {
			return false
		}

		tmp = tmp[:0]
		for _, l := range res.ChunkedSeries[0].Labels {
			tmp = append(tmp, l.String())
		}
		currSeries := strings.Join(tmp, ";")
		if currSeries != c.lastSeries {
			c.lastSeries = currSeries
			c.currLset = res.ChunkedSeries[0].Labels
			break
		}

		for _, series := range res.ChunkedSeries {
			c.currChunks = append(c.currChunks, series.Chunks...)
		}

		_, err = c.stream.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return false
		}
	}

	return true
}

func (c *chunkSeriesSet) At() storage.Series {
	if !c.initiated || c.err != nil {
		return nil
	}

	return &chunkSeries{
		labels: c.currLset,
		chunks: c.currChunks,
		mint:   c.mint,
		maxt:   c.maxt,
	}
}

func (c *chunkSeriesSet) Err() error {
	return c.err
}

// chunkSeries implements storage.Series.
type chunkSeries struct {
	labels     []prompb.Label
	chunks     []prompb.Chunk
	mint, maxt int64
}

func (c *chunkSeries) Labels() labels.Labels {
	return labelProtosToLabels(c.labels)
}

func (c *chunkSeries) Iterator() storage.SeriesIterator {
	sit := newChunkSeriesIterator(c.chunks)
	return newBoundedSeriesIterator(sit, c.mint, c.maxt)
}

// chunkSeriesIterator implements storage.SeriesIterator.
type chunkSeriesIterator struct {
	cur    int
	chunks []chunkenc.Iterator
}

func newChunkSeriesIterator(chunks []prompb.Chunk) storage.SeriesIterator {
	chks := []chunkenc.Iterator{}
	for _, chunk := range chunks {
		chk, err := chunkenc.FromData(chunkenc.Encoding(chunk.Type), chunk.Data)
		if err != nil {
			return &errSeriesIterator{err}
		}
		chks = append(chks, chk.Iterator(nil))
	}
	return &chunkSeriesIterator{chunks: chks}
}

func (it *chunkSeriesIterator) Seek(t int64) bool {
	// We generally expect the chunks already to be cut donw
	// to the range we are interested in. There's not much to be gained from
	// hoping across chunks so we just call next until we reach t.
	for {
		ct, _ := it.At()
		if ct >= t {
			return true
		}
		if !it.Next() {
			return false
		}
	}
}

func (it *chunkSeriesIterator) At() (int64, float64) {
	return it.chunks[it.cur].At()
}

func (it *chunkSeriesIterator) Next() bool {
	lastT, _ := it.At()

	if it.chunks[it.cur].Next() {
		return true
	}
	if it.Err() != nil {
		return false
	}
	if it.cur >= len(it.chunks)-1 {
		return false
	}
	// Chunks are guaranteed to be ordered but not generally guaranteed to not overlap.
	// We must ensure to skip any overlapping range between adjacent chunks.
	it.cur++
	return it.Seek(lastT + 1)
}

func (it *chunkSeriesIterator) Err() error {
	return it.chunks[it.cur].Err()
}

// boundedSeriesIterator wraps a series iterator and ensures that it only emits
// samples within a fixed time range.
type boundedSeriesIterator struct {
	it         storage.SeriesIterator
	mint, maxt int64
}

func newBoundedSeriesIterator(it storage.SeriesIterator, mint, maxt int64) *boundedSeriesIterator {
	return &boundedSeriesIterator{it: it, mint: mint, maxt: maxt}
}

func (it *boundedSeriesIterator) Seek(t int64) bool {
	if t > it.maxt {
		return false
	}
	if t < it.mint {
		t = it.mint
	}
	return it.it.Seek(t)
}

func (it *boundedSeriesIterator) At() (t int64, v float64) {
	return it.it.At()
}

func (it *boundedSeriesIterator) Next() bool {
	if !it.it.Next() {
		return false
	}
	t, _ := it.it.At()

	// Advance the iterator if we are before the valid interval.
	if t < it.mint {
		if !it.Seek(it.mint) {
			return false
		}
		t, _ = it.it.At()
	}
	// Once we passed the valid interval, there is no going back.
	return t <= it.maxt
}

func (it *boundedSeriesIterator) Err() error {
	return it.it.Err()
}

// validateLabelsAndMetricName validates the label names/values and metric names returned from remote read,
// also making sure that there are no labels with duplicate names
func validateLabelsAndMetricName(ls labels.Labels) error {
	for i, l := range ls {
		if l.Name == labels.MetricName && !model.IsValidMetricName(model.LabelValue(l.Value)) {
			return errors.Errorf("invalid metric name: %v", l.Value)
		}
		if !model.LabelName(l.Name).IsValid() {
			return errors.Errorf("invalid label name: %v", l.Name)
		}
		if !model.LabelValue(l.Value).IsValid() {
			return errors.Errorf("invalid label value: %v", l.Value)
		}
		if i > 0 && l.Name == ls[i-1].Name {
			return errors.Errorf("duplicate label with name: %v", l.Name)
		}
	}
	return nil
}

func toLabelMatchers(matchers []*labels.Matcher) ([]*prompb.LabelMatcher, error) {
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

// FromLabelMatchers parses protobuf label matchers to Prometheus label matchers.
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

// LabelProtosToMetric unpack a []*prompb.Label to a model.Metric
func LabelProtosToMetric(labelPairs []*prompb.Label) model.Metric {
	metric := make(model.Metric, len(labelPairs))
	for _, l := range labelPairs {
		metric[model.LabelName(l.Name)] = model.LabelValue(l.Value)
	}
	return metric
}

func labelProtosToLabels(labelPairs []prompb.Label) labels.Labels {
	result := make(labels.Labels, 0, len(labelPairs))
	for _, l := range labelPairs {
		result = append(result, labels.Label{
			Name:  l.Name,
			Value: l.Value,
		})
	}
	sort.Sort(result)
	return result
}

// labelsToLabelsProto transforms labels into prompb labels. The buffer slice
// will be used to avoid allocations if it is big enough to store the labels.
func labelsToLabelsProto(labels labels.Labels, buf []prompb.Label) []prompb.Label {
	result := buf[:0]
	if cap(buf) < len(labels) {
		result = make([]prompb.Label, 0, len(labels))
	}
	for _, l := range labels {
		result = append(result, prompb.Label{
			Name:  l.Name,
			Value: l.Value,
		})
	}
	return result
}
