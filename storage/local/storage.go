package storage_ng

import (
	"sync"
	"time"

	"github.com/golang/glog"

	clientmodel "github.com/prometheus/client_golang/model"

	"github.com/prometheus/prometheus/storage/metric"
	"github.com/prometheus/prometheus/utility"
)

const persistQueueCap = 1024

type storageState uint

const (
	storageStarting storageState = iota
	storageServing
	storageStopping
)

type memorySeriesStorage struct {
	mtx sync.RWMutex

	state       storageState
	persistDone chan bool
	stopServing chan chan<- bool

	// TODO: These have to go to persistence.
	fingerprintToSeries     map[clientmodel.Fingerprint]*memorySeries
	labelPairToFingerprints map[metric.LabelPair]utility.Set
	labelNameToLabelValues  map[clientmodel.LabelName]utility.Set

	memoryEvictionInterval time.Duration
	memoryRetentionPeriod  time.Duration

	persistencePurgeInterval   time.Duration
	persistenceRetentionPeriod time.Duration

	persistQueue chan *persistRequest
	persistence  Persistence
}

type MemorySeriesStorageOptions struct {
	Persistence                Persistence
	MemoryEvictionInterval     time.Duration
	MemoryRetentionPeriod      time.Duration
	PersistencePurgeInterval   time.Duration
	PersistenceRetentionPeriod time.Duration
}

func NewMemorySeriesStorage(o *MemorySeriesStorageOptions) (*memorySeriesStorage, error) { // TODO: change to return Storage?
	glog.Info("Loading series head chunks...")
	/*
			if err := o.Persistence.LoadHeads(i.FingerprintToSeries); err != nil {
				return nil, err
			}
		numSeries.Set(float64(len(i.FingerprintToSeries)))
	*/
	return &memorySeriesStorage{
		fingerprintToSeries:     map[clientmodel.Fingerprint]*memorySeries{},
		labelPairToFingerprints: map[metric.LabelPair]utility.Set{},
		labelNameToLabelValues:  map[clientmodel.LabelName]utility.Set{},

		persistDone: make(chan bool),
		stopServing: make(chan chan<- bool),

		memoryEvictionInterval: o.MemoryEvictionInterval,
		memoryRetentionPeriod:  o.MemoryRetentionPeriod,

		persistencePurgeInterval:   o.PersistencePurgeInterval,
		persistenceRetentionPeriod: o.PersistenceRetentionPeriod,

		persistQueue: make(chan *persistRequest, persistQueueCap),
		persistence:  o.Persistence,
	}, nil
}

type persistRequest struct {
	fingerprint clientmodel.Fingerprint
	chunkDesc   *chunkDesc
}

func (s *memorySeriesStorage) AppendSamples(samples clientmodel.Samples) {
	/*
		s.mtx.Lock()
		defer s.mtx.Unlock()
		if s.state != storageServing {
			panic("storage is not serving")
		}
		s.mtx.Unlock()
	*/

	for _, sample := range samples {
		s.appendSample(sample)
	}

	numSamples.Add(float64(len(samples)))
}

func (s *memorySeriesStorage) appendSample(sample *clientmodel.Sample) {
	s.mtx.Lock()
	defer s.mtx.Unlock()

	series := s.getOrCreateSeries(sample.Metric)
	series.add(&metric.SamplePair{
		Value:     sample.Value,
		Timestamp: sample.Timestamp,
	}, s.persistQueue)
}

func (s *memorySeriesStorage) getOrCreateSeries(m clientmodel.Metric) *memorySeries {
	fp := m.Fingerprint()
	series, ok := s.fingerprintToSeries[fp]

	if !ok {
		series = newMemorySeries(m)
		s.fingerprintToSeries[fp] = series
		numSeries.Set(float64(len(s.fingerprintToSeries)))

		for k, v := range m {
			labelPair := metric.LabelPair{
				Name:  k,
				Value: v,
			}

			fps, ok := s.labelPairToFingerprints[labelPair]
			if !ok {
				fps = utility.Set{}
				s.labelPairToFingerprints[labelPair] = fps
			}
			fps.Add(fp)

			values, ok := s.labelNameToLabelValues[k]
			if !ok {
				values = utility.Set{}
				s.labelNameToLabelValues[k] = values
			}
			values.Add(v)
		}
	}
	return series
}

/*
func (s *memorySeriesStorage) preloadChunksAtTime(fp clientmodel.Fingerprint, ts clientmodel.Timestamp) (chunkDescs, error) {
	series, ok := s.fingerprintToSeries[fp]
	if !ok {
		panic("requested preload for non-existent series")
	}
	return series.preloadChunksAtTime(ts, s.persistence)
}
*/

func (s *memorySeriesStorage) preloadChunksForRange(fp clientmodel.Fingerprint, from clientmodel.Timestamp, through clientmodel.Timestamp) (chunkDescs, error) {
	s.mtx.RLock()
	series, ok := s.fingerprintToSeries[fp]
	s.mtx.RUnlock()

	if !ok {
		panic("requested preload for non-existent series")
	}
	return series.preloadChunksForRange(from, through, s.persistence)
}

func (s *memorySeriesStorage) NewIterator(fp clientmodel.Fingerprint) SeriesIterator {
	s.mtx.RLock()
	series, ok := s.fingerprintToSeries[fp]
	s.mtx.RUnlock()

	if !ok {
		panic("requested iterator for non-existent series")
	}
	return series.newIterator()
}

func (s *memorySeriesStorage) evictMemoryChunks(ttl time.Duration) {
	s.mtx.RLock()
	defer s.mtx.RUnlock()

	for _, series := range s.fingerprintToSeries {
		series.evictOlderThan(clientmodel.TimestampFromTime(time.Now()).Add(-1 * ttl))
	}
}

func recordPersist(start time.Time, err error) {
	outcome := success
	if err != nil {
		outcome = failure
	}
	persistLatencies.WithLabelValues(outcome).Observe(float64(time.Since(start) / time.Millisecond))
}

func (s *memorySeriesStorage) handlePersistQueue() {
	for req := range s.persistQueue {
		// TODO: Make this thread-safe?
		persistQueueLength.Set(float64(len(s.persistQueue)))

		//glog.Info("Persist request: ", *req.fingerprint)
		start := time.Now()
		err := s.persistence.PersistChunk(req.fingerprint, req.chunkDesc.chunk)
		recordPersist(start, err)
		if err != nil {
			glog.Error("Error persisting chunk, requeuing: ", err)
			s.persistQueue <- req
			continue
		}
		req.chunkDesc.unpin()
	}
	s.persistDone <- true
}

// Close stops serving, flushes all pending operations, and frees all resources.
func (s *memorySeriesStorage) Close() error {
	s.mtx.Lock()
	defer s.mtx.Unlock()

	if s.state == storageStopping {
		panic("Illegal State: Attempted to restop memorySeriesStorage.")
	}

	stopped := make(chan bool)
	glog.Info("Waiting for storage to stop serving...")
	s.stopServing <- (stopped)
	glog.Info("Serving stopped.")
	<-stopped

	glog.Info("Stopping persist loop...")
	close(s.persistQueue)
	<-s.persistDone
	glog.Info("Persist loop stopped.")

	glog.Info("Persisting head chunks...")
	if err := s.persistHeads(); err != nil {
		return err
	}
	glog.Info("Done persisting head chunks.")

	for _, series := range s.fingerprintToSeries {
		series.close()
	}
	s.fingerprintToSeries = nil

	s.state = storageStopping

	return nil
}

func (s *memorySeriesStorage) persistHeads() error {
	return s.persistence.PersistHeads(s.fingerprintToSeries)
}

func (s *memorySeriesStorage) purgePeriodically(stop <-chan bool) {
	purgeTicker := time.NewTicker(s.persistencePurgeInterval)
	defer purgeTicker.Stop()

	for {
		select {
		case <-stop:
			return
		case <-purgeTicker.C:
			glog.Info("Purging old series data...")
			s.mtx.RLock()
			fps := make([]clientmodel.Fingerprint, 0, len(s.fingerprintToSeries))
			for fp := range s.fingerprintToSeries {
				fps = append(fps, fp)
			}
			s.mtx.RUnlock()

			for _, fp := range fps {
				select {
				case <-stop:
					glog.Info("Interrupted running series purge.")
					return
				default:
					s.purgeSeries(&fp)
				}
			}
			glog.Info("Done purging old series data.")
		}
	}
}

func (s *memorySeriesStorage) purgeSeries(fp *clientmodel.Fingerprint) {
	s.mtx.RLock()
	series, ok := s.fingerprintToSeries[*fp]
	if !ok {
		return
	}
	s.mtx.RUnlock()

	drop, err := series.purgeOlderThan(clientmodel.TimestampFromTime(time.Now()).Add(-1*s.persistenceRetentionPeriod), s.persistence)
	if err != nil {
		glog.Error("Error purging series data: ", err)
	}
	if drop {
		s.dropSeries(fp)
	}
}

// Drop a label value from the label names to label values index.
func (s *memorySeriesStorage) dropLabelValue(l clientmodel.LabelName, v clientmodel.LabelValue) {
	if set, ok := s.labelNameToLabelValues[l]; ok {
		set.Remove(v)
		if len(set) == 0 {
			delete(s.labelNameToLabelValues, l)
		}
	}
}

// Drop all references to a series, including any samples.
func (s *memorySeriesStorage) dropSeries(fp *clientmodel.Fingerprint) {
	s.mtx.Lock()
	defer s.mtx.Unlock()

	series, ok := s.fingerprintToSeries[*fp]
	if !ok {
		return
	}

	for k, v := range series.metric {
		labelPair := metric.LabelPair{
			Name:  k,
			Value: v,
		}
		if set, ok := s.labelPairToFingerprints[labelPair]; ok {
			set.Remove(*fp)
			if len(set) == 0 {
				delete(s.labelPairToFingerprints, labelPair)
				s.dropLabelValue(k, v)
			}
		}
	}
	delete(s.fingerprintToSeries, *fp)
}

func (s *memorySeriesStorage) Serve(started chan<- bool) {
	s.mtx.Lock()
	if s.state != storageStarting {
		panic("Illegal State: Attempted to restart memorySeriesStorage.")
	}
	s.state = storageServing
	s.mtx.Unlock()

	evictMemoryTicker := time.NewTicker(s.memoryEvictionInterval)
	defer evictMemoryTicker.Stop()

	go s.handlePersistQueue()

	stopPurge := make(chan bool)
	go s.purgePeriodically(stopPurge)

	started <- true
	for {
		select {
		case <-evictMemoryTicker.C:
			s.evictMemoryChunks(s.memoryRetentionPeriod)
		case stopped := <-s.stopServing:
			stopPurge <- true
			stopped <- true
			return
		}
	}
}

func (s *memorySeriesStorage) NewPreloader() Preloader {
	return &memorySeriesPreloader{
		storage: s,
	}
}

func (s *memorySeriesStorage) GetFingerprintsForLabelMatchers(labelMatchers metric.LabelMatchers) clientmodel.Fingerprints {
	s.mtx.RLock()
	defer s.mtx.RUnlock()

	sets := []utility.Set{}
	for _, matcher := range labelMatchers {
		switch matcher.Type {
		case metric.Equal:
			set, ok := s.labelPairToFingerprints[metric.LabelPair{
				Name:  matcher.Name,
				Value: matcher.Value,
			}]
			if !ok {
				return nil
			}
			sets = append(sets, set)
		default:
			values := s.getLabelValuesForLabelName(matcher.Name)
			matches := matcher.Filter(values)
			if len(matches) == 0 {
				return nil
			}
			set := utility.Set{}
			for _, v := range matches {
				subset, ok := s.labelPairToFingerprints[metric.LabelPair{
					Name:  matcher.Name,
					Value: v,
				}]
				if !ok {
					return nil
				}
				for fp := range subset {
					set.Add(fp)
				}
			}
			sets = append(sets, set)
		}
	}

	setCount := len(sets)
	if setCount == 0 {
		return nil
	}

	base := sets[0]
	for i := 1; i < setCount; i++ {
		base = base.Intersection(sets[i])
	}

	fingerprints := clientmodel.Fingerprints{}
	for _, e := range base.Elements() {
		fingerprints = append(fingerprints, e.(clientmodel.Fingerprint))
	}

	return fingerprints
}

func (s *memorySeriesStorage) GetLabelValuesForLabelName(labelName clientmodel.LabelName) clientmodel.LabelValues {
	s.mtx.RLock()
	defer s.mtx.RUnlock()

	return s.getLabelValuesForLabelName(labelName)
}

func (s *memorySeriesStorage) getLabelValuesForLabelName(labelName clientmodel.LabelName) clientmodel.LabelValues {
	set, ok := s.labelNameToLabelValues[labelName]
	if !ok {
		return nil
	}

	values := make(clientmodel.LabelValues, 0, len(set))
	for e := range set {
		val := e.(clientmodel.LabelValue)
		values = append(values, val)
	}
	return values
}

func (s *memorySeriesStorage) GetMetricForFingerprint(fp clientmodel.Fingerprint) clientmodel.Metric {
	s.mtx.RLock()
	defer s.mtx.RUnlock()

	series, ok := s.fingerprintToSeries[fp]
	if !ok {
		return nil
	}

	metric := clientmodel.Metric{}
	for label, value := range series.metric {
		metric[label] = value
	}

	return metric
}

func (s *memorySeriesStorage) GetAllValuesForLabel(labelName clientmodel.LabelName) clientmodel.LabelValues {
	s.mtx.RLock()
	defer s.mtx.RUnlock()

	var values clientmodel.LabelValues
	valueSet := map[clientmodel.LabelValue]struct{}{}
	for _, series := range s.fingerprintToSeries {
		if value, ok := series.metric[labelName]; ok {
			if _, ok := valueSet[value]; !ok {
				values = append(values, value)
				valueSet[value] = struct{}{}
			}
		}
	}

	return values
}
