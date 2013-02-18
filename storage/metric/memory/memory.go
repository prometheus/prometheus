package memory

import (
	"fmt"
	"github.com/prometheus/prometheus/model"
	"github.com/prometheus/prometheus/storage/metric"
	"github.com/prometheus/prometheus/utility"
	"github.com/ryszard/goskiplist/skiplist"
	"sort"
	"time"
)

const (
	reservedDelimiter = `"`
)

type skipListTime time.Time

func (t skipListTime) LessThan(o skiplist.Ordered) bool {
	//	return time.Time(t).Before(time.Time(o.(skipListTime)))
	return time.Time(o.(skipListTime)).Before(time.Time(t))
}

type stream struct {
	metric model.Metric
	values *skiplist.SkipList
}

func (s *stream) add(sample model.Sample) {
	s.values.Set(skipListTime(sample.Timestamp), sample.Value)
}

func newStream(metric model.Metric) *stream {
	return &stream{
		values: skiplist.New(),
		metric: metric,
	}
}

type memorySeriesStorage struct {
	fingerprintToSeries     map[model.Fingerprint]*stream
	labelPairToFingerprints map[string]model.Fingerprints
	labelNameToFingerprints map[model.LabelName]model.Fingerprints
}

func (s *memorySeriesStorage) AppendSample(sample model.Sample) (err error) {
	metric := sample.Metric
	fingerprint := metric.Fingerprint()
	series, ok := s.fingerprintToSeries[fingerprint]

	if !ok {
		series = newStream(metric)
		s.fingerprintToSeries[fingerprint] = series

		for k, v := range metric {
			labelPair := fmt.Sprintf("%s%s%s", k, reservedDelimiter, v)

			labelPairValues := s.labelPairToFingerprints[labelPair]
			labelPairValues = append(labelPairValues, fingerprint)
			s.labelPairToFingerprints[labelPair] = labelPairValues

			labelNameValues := s.labelNameToFingerprints[k]
			labelNameValues = append(labelNameValues, fingerprint)
			s.labelNameToFingerprints[k] = labelNameValues
		}
	}

	series.add(sample)

	return
}

func (s *memorySeriesStorage) GetFingerprintsForLabelSet(l model.LabelSet) (fingerprints model.Fingerprints, err error) {

	sets := []utility.Set{}

	for k, v := range l {
		signature := fmt.Sprintf("%s%s%s", k, reservedDelimiter, v)
		values := s.labelPairToFingerprints[signature]
		set := utility.Set{}
		for _, fingerprint := range values {
			set.Add(fingerprint)
		}
		sets = append(sets, set)
	}

	setCount := len(sets)
	if setCount == 0 {
		return
	}

	base := sets[0]
	for i := 1; i < setCount; i++ {
		base = base.Intersection(sets[i])
	}
	for _, e := range base.Elements() {
		fingerprint := e.(model.Fingerprint)
		fingerprints = append(fingerprints, fingerprint)
	}

	return
}

func (s *memorySeriesStorage) GetFingerprintsForLabelName(l model.LabelName) (fingerprints model.Fingerprints, err error) {
	values := s.labelNameToFingerprints[l]

	fingerprints = append(fingerprints, values...)

	return
}

func (s *memorySeriesStorage) GetMetricForFingerprint(f model.Fingerprint) (metric *model.Metric, err error) {
	series, ok := s.fingerprintToSeries[f]
	if !ok {
		return
	}

	metric = &series.metric

	return
}

// XXX: Terrible wart.
func interpolate(x1, x2 time.Time, y1, y2 float32, e time.Time) model.SampleValue {
	yDelta := y2 - y1
	xDelta := x2.Sub(x1)

	dDt := yDelta / float32(xDelta)
	offset := float32(e.Sub(x1))

	return model.SampleValue(y1 + (offset * dDt))
}

func (s *memorySeriesStorage) GetValueAtTime(m model.Metric, t time.Time, p metric.StalenessPolicy) (sample *model.Sample, err error) {
	fingerprint := m.Fingerprint()
	series, ok := s.fingerprintToSeries[fingerprint]
	if !ok {
		return
	}

	iterator := series.values.Seek(skipListTime(t))
	if iterator == nil {
		return
	}

	foundTime := time.Time(iterator.Key().(skipListTime))
	if foundTime.Equal(t) {
		sample = &model.Sample{
			Metric:    m,
			Value:     iterator.Value().(model.SampleValue),
			Timestamp: t,
		}

		return
	}

	if t.Sub(foundTime) > p.DeltaAllowance {
		return
	}

	secondTime := foundTime
	secondValue := iterator.Value().(model.SampleValue)

	if !iterator.Previous() {
		sample = &model.Sample{
			Metric:    m,
			Value:     iterator.Value().(model.SampleValue),
			Timestamp: t,
		}
		return
	}

	firstTime := time.Time(iterator.Key().(skipListTime))
	if t.Sub(firstTime) > p.DeltaAllowance {
		return
	}

	if firstTime.Sub(secondTime) > p.DeltaAllowance {
		return
	}

	firstValue := iterator.Value().(model.SampleValue)

	sample = &model.Sample{
		Metric:    m,
		Value:     interpolate(firstTime, secondTime, float32(firstValue), float32(secondValue), t),
		Timestamp: t,
	}

	return
}

func (s *memorySeriesStorage) GetBoundaryValues(m model.Metric, i model.Interval, p metric.StalenessPolicy) (first *model.Sample, second *model.Sample, err error) {
	first, err = s.GetValueAtTime(m, i.OldestInclusive, p)
	if err != nil {
		return
	} else if first == nil {
		return
	}

	second, err = s.GetValueAtTime(m, i.NewestInclusive, p)
	if err != nil {
		return
	} else if second == nil {
		first = nil
	}

	return
}

func (s *memorySeriesStorage) GetRangeValues(m model.Metric, i model.Interval) (samples *model.SampleSet, err error) {
	fingerprint := m.Fingerprint()
	series, ok := s.fingerprintToSeries[fingerprint]
	if !ok {
		return
	}

	samples = &model.SampleSet{
		Metric: m,
	}

	iterator := series.values.Seek(skipListTime(i.NewestInclusive))
	if iterator == nil {
		return
	}

	for {
		timestamp := time.Time(iterator.Key().(skipListTime))
		if timestamp.Before(i.OldestInclusive) {
			break
		}

		samples.Values = append(samples.Values,
			model.SamplePair{
				Value:     model.SampleValue(iterator.Value().(model.SampleValue)),
				Timestamp: timestamp,
			})

		if !iterator.Next() {
			break
		}
	}

	// XXX: We should not explicitly sort here but rather rely on the datastore.
	//      This adds appreciable overhead.
	if samples != nil {
		sort.Sort(samples.Values)
	}

	return
}

func (s *memorySeriesStorage) Close() (err error) {
	for fingerprint := range s.fingerprintToSeries {
		delete(s.fingerprintToSeries, fingerprint)
	}

	for labelPair := range s.labelPairToFingerprints {
		delete(s.labelPairToFingerprints, labelPair)
	}

	for labelName := range s.labelNameToFingerprints {
		delete(s.labelNameToFingerprints, labelName)
	}

	return
}

func (s *memorySeriesStorage) GetAllMetricNames() (names []string, err error) {
	panic("not implemented")

	return
}

func (s *memorySeriesStorage) GetAllLabelNames() (names []string, err error) {
	panic("not implemented")

	return
}

func (s *memorySeriesStorage) GetAllLabelPairs() (pairs []model.LabelSet, err error) {
	panic("not implemented")

	return
}

func (s *memorySeriesStorage) GetAllMetrics() (metrics []model.LabelSet, err error) {
	panic("not implemented")

	return
}

func NewMemorySeriesStorage() metric.MetricPersistence {
	return newMemorySeriesStorage()
}

func newMemorySeriesStorage() *memorySeriesStorage {
	return &memorySeriesStorage{
		fingerprintToSeries:     make(map[model.Fingerprint]*stream),
		labelPairToFingerprints: make(map[string]model.Fingerprints),
		labelNameToFingerprints: make(map[model.LabelName]model.Fingerprints),
	}
}
