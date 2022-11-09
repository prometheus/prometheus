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
	"errors"
	"fmt"
	"io"
	"net/http"
	"sort"
	"strings"

	"github.com/gogo/protobuf/proto"
	"github.com/golang/snappy"
	"github.com/prometheus/common/model"

	"github.com/prometheus/prometheus/model/exemplar"
	"github.com/prometheus/prometheus/model/histogram"
	"github.com/prometheus/prometheus/model/labels"
	"github.com/prometheus/prometheus/model/textparse"
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
	ms, err := toLabelMatchers(matchers)
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
func ToQueryResult(ss storage.SeriesSet, sampleLimit int) (*prompb.QueryResult, storage.Warnings, error) {
	numSamples := 0
	resp := &prompb.QueryResult{}
	for ss.Next() {
		series := ss.At()
		iter := series.Iterator()
		samples := []prompb.Sample{}

		for iter.Next() == chunkenc.ValFloat {
			// TODO(beorn7): Add Histogram support.
			numSamples++
			if sampleLimit > 0 && numSamples > sampleLimit {
				return nil, ss.Warnings(), HTTPError{
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
			return nil, ss.Warnings(), err
		}

		resp.Timeseries = append(resp.Timeseries, &prompb.TimeSeries{
			Labels:  labelsToLabelsProto(series.Labels(), nil),
			Samples: samples,
		})
	}
	return resp, ss.Warnings(), ss.Err()
}

// FromQueryResult unpacks and sorts a QueryResult proto.
func FromQueryResult(sortSeries bool, res *prompb.QueryResult) storage.SeriesSet {
	series := make([]storage.Series, 0, len(res.Timeseries))
	for _, ts := range res.Timeseries {
		lbls := labelProtosToLabels(ts.Labels)
		if err := validateLabelsAndMetricName(lbls); err != nil {
			return errSeriesSet{err: err}
		}
		series = append(series, &concreteSeries{labels: lbls, samples: ts.Samples})
	}

	if sortSeries {
		sort.Sort(byLabel(series))
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
) (storage.Warnings, error) {
	var (
		chks []prompb.Chunk
		lbls []prompb.Label
	)

	for ss.Next() {
		series := ss.At()
		iter := series.Iterator()
		lbls = MergeLabels(labelsToLabelsProto(series.Labels(), lbls), sortedExternalLabels)

		frameBytesLeft := maxBytesInFrame
		for _, lbl := range lbls {
			frameBytesLeft -= lbl.Size()
		}

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

			b, err := proto.Marshal(&prompb.ChunkedReadResponse{
				ChunkedSeries: []*prompb.ChunkedSeries{
					{Labels: lbls, Chunks: chks},
				},
				QueryIndex: queryIndex,
			})
			if err != nil {
				return ss.Warnings(), fmt.Errorf("marshal ChunkedReadResponse: %w", err)
			}

			if _, err := stream.Write(b); err != nil {
				return ss.Warnings(), fmt.Errorf("write to stream: %w", err)
			}
			chks = chks[:0]
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

func (e errSeriesSet) Warnings() storage.Warnings { return nil }

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

func (c *concreteSeriesSet) Warnings() storage.Warnings { return nil }

// concreteSeries implements storage.Series.
type concreteSeries struct {
	labels  labels.Labels
	samples []prompb.Sample
}

func (c *concreteSeries) Labels() labels.Labels {
	return labels.New(c.labels...)
}

func (c *concreteSeries) Iterator() chunkenc.Iterator {
	return newConcreteSeriersIterator(c)
}

// concreteSeriesIterator implements storage.SeriesIterator.
type concreteSeriesIterator struct {
	cur    int
	series *concreteSeries
}

func newConcreteSeriersIterator(series *concreteSeries) chunkenc.Iterator {
	return &concreteSeriesIterator{
		cur:    -1,
		series: series,
	}
}

// Seek implements storage.SeriesIterator.
func (c *concreteSeriesIterator) Seek(t int64) chunkenc.ValueType {
	if c.cur == -1 {
		c.cur = 0
	}
	if c.cur >= len(c.series.samples) {
		return chunkenc.ValNone
	}
	// No-op check.
	if s := c.series.samples[c.cur]; s.Timestamp >= t {
		return chunkenc.ValFloat
	}
	// Do binary search between current position and end.
	c.cur += sort.Search(len(c.series.samples)-c.cur, func(n int) bool {
		return c.series.samples[n+c.cur].Timestamp >= t
	})
	if c.cur < len(c.series.samples) {
		return chunkenc.ValFloat
	}
	return chunkenc.ValNone
	// TODO(beorn7): Add histogram support.
}

// At implements chunkenc.Iterator.
func (c *concreteSeriesIterator) At() (t int64, v float64) {
	s := c.series.samples[c.cur]
	return s.Timestamp, s.Value
}

// AtHistogram always returns (0, nil) because there is no support for histogram
// values yet.
// TODO(beorn7): Fix that for histogram support in remote storage.
func (c *concreteSeriesIterator) AtHistogram() (int64, *histogram.Histogram) {
	return 0, nil
}

// AtFloatHistogram always returns (0, nil) because there is no support for histogram
// values yet.
// TODO(beorn7): Fix that for histogram support in remote storage.
func (c *concreteSeriesIterator) AtFloatHistogram() (int64, *histogram.FloatHistogram) {
	return 0, nil
}

// AtT implements chunkenc.Iterator.
func (c *concreteSeriesIterator) AtT() int64 {
	s := c.series.samples[c.cur]
	return s.Timestamp
}

// Next implements chunkenc.Iterator.
func (c *concreteSeriesIterator) Next() chunkenc.ValueType {
	c.cur++
	if c.cur < len(c.series.samples) {
		return chunkenc.ValFloat
	}
	return chunkenc.ValNone
	// TODO(beorn7): Add histogram support.
}

// Err implements chunkenc.Iterator.
func (c *concreteSeriesIterator) Err() error {
	return nil
}

// validateLabelsAndMetricName validates the label names/values and metric names returned from remote read,
// also making sure that there are no labels with duplicate names
func validateLabelsAndMetricName(ls labels.Labels) error {
	for i, l := range ls {
		if l.Name == labels.MetricName && !model.IsValidMetricName(model.LabelValue(l.Value)) {
			return fmt.Errorf("invalid metric name: %v", l.Value)
		}
		if !model.LabelName(l.Name).IsValid() {
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

func exemplarProtoToExemplar(ep prompb.Exemplar) exemplar.Exemplar {
	timestamp := ep.Timestamp

	return exemplar.Exemplar{
		Labels: labelProtosToLabels(ep.Labels),
		Value:  ep.Value,
		Ts:     timestamp,
		HasTs:  timestamp != 0,
	}
}

// HistogramProtoToHistogram extracts a (normal integer) Histogram from the
// provided proto message. The caller has to make sure that the proto message
// represents an interger histogram and not a float histogram.
func HistogramProtoToHistogram(hp prompb.Histogram) *histogram.Histogram {
	return &histogram.Histogram{
		Schema:          hp.Schema,
		ZeroThreshold:   hp.ZeroThreshold,
		ZeroCount:       hp.GetZeroCountInt(),
		Count:           hp.GetCountInt(),
		Sum:             hp.Sum,
		PositiveSpans:   spansProtoToSpans(hp.GetPositiveSpans()),
		PositiveBuckets: hp.GetPositiveDeltas(),
		NegativeSpans:   spansProtoToSpans(hp.GetNegativeSpans()),
		NegativeBuckets: hp.GetNegativeDeltas(),
	}
}

func spansProtoToSpans(s []*prompb.BucketSpan) []histogram.Span {
	spans := make([]histogram.Span, len(s))
	for i := 0; i < len(s); i++ {
		spans[i] = histogram.Span{Offset: s[i].Offset, Length: s[i].Length}
	}

	return spans
}

func HistogramToHistogramProto(timestamp int64, h *histogram.Histogram) prompb.Histogram {
	return prompb.Histogram{
		Count:          &prompb.Histogram_CountInt{CountInt: h.Count},
		Sum:            h.Sum,
		Schema:         h.Schema,
		ZeroThreshold:  h.ZeroThreshold,
		ZeroCount:      &prompb.Histogram_ZeroCountInt{ZeroCountInt: h.ZeroCount},
		NegativeSpans:  spansToSpansProto(h.NegativeSpans),
		NegativeDeltas: h.NegativeBuckets,
		PositiveSpans:  spansToSpansProto(h.PositiveSpans),
		PositiveDeltas: h.PositiveBuckets,
		Timestamp:      timestamp,
	}
}

func spansToSpansProto(s []histogram.Span) []*prompb.BucketSpan {
	spans := make([]*prompb.BucketSpan, len(s))
	for i := 0; i < len(s); i++ {
		spans[i] = &prompb.BucketSpan{Offset: s[i].Offset, Length: s[i].Length}
	}

	return spans
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

// metricTypeToMetricTypeProto transforms a Prometheus metricType into prompb metricType. Since the former is a string we need to transform it to an enum.
func metricTypeToMetricTypeProto(t textparse.MetricType) prompb.MetricMetadata_MetricType {
	mt := strings.ToUpper(string(t))
	v, ok := prompb.MetricMetadata_MetricType_value[mt]
	if !ok {
		return prompb.MetricMetadata_UNKNOWN
	}

	return prompb.MetricMetadata_MetricType(v)
}

// DecodeWriteRequest from an io.Reader into a prompb.WriteRequest, handling
// snappy decompression.
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
