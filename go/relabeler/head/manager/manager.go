package manager

import (
	"errors"
	"fmt"
	"github.com/jonboulle/clockwork"
	"github.com/prometheus/prometheus/pp/go/relabeler"
	"github.com/prometheus/prometheus/pp/go/relabeler/config"
	"github.com/prometheus/prometheus/pp/go/relabeler/head"
	"github.com/prometheus/prometheus/pp/go/relabeler/head/catalog"
	"github.com/prometheus/prometheus/pp/go/relabeler/logger"
	"github.com/prometheus/prometheus/pp/go/util"
	"github.com/prometheus/client_golang/prometheus"
	"os"
	"path/filepath"
	"sync"
	"time"
)

type ConfigSource interface {
	Get() (inputRelabelerConfigs []*config.InputRelabelerConfig, numberOfShards uint16)
}

type Catalog interface {
	List(filter func(record *catalog.Record) bool, sortLess func(lhs, rhs *catalog.Record) bool) ([]*catalog.Record, error)
	Create(numberOfShards uint16) (*catalog.Record, error)
	SetStatus(id string, status catalog.Status) (*catalog.Record, error)
	SetCorrupted(id string) (*catalog.Record, error)
}

type metrics struct {
	CreatedHeadsCount   prometheus.Counter
	RotatedHeadsCount   prometheus.Counter
	CorruptedHeadsCount prometheus.Counter
	PersistedHeadsCount prometheus.Counter
	DeletedHeadsCount   prometheus.Counter
}

type Manager struct {
	dir          string
	clock        clockwork.Clock
	configSource ConfigSource
	catalog      Catalog
	generation   uint64
	counter      *prometheus.CounterVec
	registerer   prometheus.Registerer
}

type SetLastAppendedSegmentIDFn func(segmentID uint32)

func (fn SetLastAppendedSegmentIDFn) SetLastAppendedSegmentID(segmentID uint32) {
	fn(segmentID)
}

func New(dir string, clock clockwork.Clock, configSource ConfigSource, catalog Catalog, registerer prometheus.Registerer) (*Manager, error) {
	dirStat, err := os.Stat(dir)
	if err != nil {
		return nil, fmt.Errorf("failed to stat dir: %w", err)
	}

	if !dirStat.IsDir() {
		return nil, fmt.Errorf("%s is not directory", dir)
	}

	factory := util.NewUnconflictRegisterer(registerer)

	return &Manager{
		dir:          dir,
		clock:        clock,
		configSource: configSource,
		catalog:      catalog,
		counter: factory.NewCounterVec(
			prometheus.CounterOpts{
				Name: "prompp_head_event_count",
				Help: "Number of head events",
			},
			[]string{"type"},
		),
		registerer: registerer,
	}, nil
}

type HeadLoadResult struct {
	headRecord *catalog.Record
	head       relabeler.Head
	corrupted  bool
	duration   time.Duration
	err        error
}

func (m *Manager) Restore(blockDuration time.Duration) (active relabeler.Head, rotated []relabeler.Head, err error) {
	headRecords, err := m.catalog.List(
		func(record *catalog.Record) bool {
			return record.DeletedAt() == 0 && record.Status() != catalog.StatusPersisted
		},
		func(lhs, rhs *catalog.Record) bool {
			return lhs.CreatedAt() < rhs.CreatedAt()
		},
	)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to list head records: %w", err)
	}

	wg := &sync.WaitGroup{}
	headLoadResults := make([]HeadLoadResult, len(headRecords))
	for index, headRecord := range headRecords {
		hr := headRecord
		cfgs, _ := m.configSource.Get()
		generation := m.generation
		wg.Add(1)
		go func(index int, headRecord *catalog.Record, inputRelabelerConfigs []*config.InputRelabelerConfig, generation uint64) {
			defer wg.Done()
			headLoadResults[index] = m.loadHead(headRecord, inputRelabelerConfigs, generation)
		}(index, hr, cfgs, generation)
		m.generation++
	}
	wg.Wait()

	for i, loadResult := range headLoadResults {
		if loadResult.err != nil {
			logger.Errorf("head load totally failed: %v", err)
			continue
		}

		if i == len(headLoadResults)-1 {
			statusIsAppropriate := loadResult.headRecord.Status() == catalog.StatusNew || loadResult.headRecord.Status() == catalog.StatusActive
			isInBlockTimeRange := m.clock.Now().Sub(time.UnixMilli(loadResult.headRecord.CreatedAt())).Milliseconds() < blockDuration.Milliseconds()
			isNotCorrupted := !loadResult.corrupted
			if isNotCorrupted && statusIsAppropriate && isInBlockTimeRange {
				active = loadResult.head
				continue
			}
		}

		if loadResult.headRecord.Status() != catalog.StatusRotated {
			if _, err = m.catalog.SetStatus(loadResult.headRecord.ID(), catalog.StatusRotated); err != nil {
				return nil, nil, fmt.Errorf("failed to set status: %w", err)
			}
		}

		if err = loadResult.head.Finalize(); err != nil {
			return nil, nil, fmt.Errorf("failed to finalize head: %w", err)
		}

		rotated = append(rotated, loadResult.head)
	}

	if active == nil {
		cfgs, numberOfShards := m.configSource.Get()
		active, err = m.BuildWithConfig(cfgs, numberOfShards)
		if err != nil {
			return nil, nil, fmt.Errorf("failed to build active head: %w", err)
		}
	}

	return active, rotated, nil
}

func (m *Manager) loadHead(
	headRecord *catalog.Record,
	inputRelabelerConfigs []*config.InputRelabelerConfig,
	generation uint64,
) (result HeadLoadResult) {
	start := m.clock.Now()
	defer func() {
		result.duration = m.clock.Since(start)
	}()
	result.headRecord = headRecord
	headDir := filepath.Join(m.dir, headRecord.Dir())
	h, corrupted, numberOfSegments, err := head.Load(
		headRecord.ID(),
		generation,
		headDir,
		inputRelabelerConfigs,
		headRecord.NumberOfShards(),
		SetLastAppendedSegmentIDFn(func(segmentID uint32) {
			headRecord.SetLastAppendedSegmentID(segmentID)
		}),
		m.registerer)
	if err != nil {
		result.err = err
		return result
	}

	if !corrupted {
		if headRecord.LastAppendedSegmentID() == nil {
			if numberOfSegments != 0 {
				corrupted = true
				logger.Errorf("head: %s number of segments mismatch, want: 0, have: %d", headRecord.ID(), numberOfSegments)
			}
		} else {
			if *headRecord.LastAppendedSegmentID()+1 != uint32(numberOfSegments) {
				corrupted = true
				logger.Errorf("head: %s number of segments mismatch, want: %d, have: %d", headRecord.ID(), *headRecord.LastAppendedSegmentID()+1, numberOfSegments)
			}
		}
	}

	if corrupted {
		if !headRecord.Corrupted() {
			if _, setCorruptedErr := m.catalog.SetCorrupted(headRecord.ID()); setCorruptedErr != nil {
				logger.Errorf("failed to set corrupted state, head id: %s: %v", headRecord.ID(), setCorruptedErr)
			}
		}

		m.counter.With(prometheus.Labels{"type": "corrupted"}).Inc()
	}

	headReleaseFn := headRecord.Acquire()
	drh := NewDiscardableRotatableHead(
		h,
		func(id string, err error) error {
			if _, rotateErr := m.catalog.SetStatus(id, catalog.StatusRotated); rotateErr != nil {
				return errors.Join(err, rotateErr)
			}
			m.counter.With(prometheus.Labels{"type": "rotated"}).Inc()
			return err
		},
		func(id string) error {
			var discardErr error
			if _, discardErr = m.catalog.SetStatus(id, catalog.StatusPersisted); discardErr != nil {
				return discardErr
			}
			m.counter.With(prometheus.Labels{"type": "persisted"}).Inc()
			return nil
		},
		func(id string) error {
			headReleaseFn()
			return nil
		},
	)
	m.counter.With(prometheus.Labels{"type": "created"}).Inc()

	result.head = drh
	result.corrupted = corrupted
	return result
}

func (m *Manager) Build() (relabeler.Head, error) {
	cfgs, numberOfShards := m.configSource.Get()
	return m.BuildWithConfig(cfgs, numberOfShards)
}

func (m *Manager) BuildWithConfig(inputRelabelerConfigs []*config.InputRelabelerConfig, numberOfShards uint16) (h relabeler.Head, err error) {
	headRecord, err := m.catalog.Create(numberOfShards)
	if err != nil {
		return nil, err
	}

	headDir := filepath.Join(m.dir, headRecord.ID())
	if err = os.Mkdir(headDir, 0777); err != nil {
		return nil, err
	}
	defer func() {
		if err != nil {
			err = errors.Join(err, os.RemoveAll(headDir))
		}
	}()

	generation := m.generation
	h, err = head.Create(
		headRecord.ID(),
		generation,
		headDir,
		inputRelabelerConfigs,
		numberOfShards,
		SetLastAppendedSegmentIDFn(func(segmentID uint32) {
			headRecord.SetLastAppendedSegmentID(segmentID)
		}),
		m.registerer)
	if err != nil {
		return nil, fmt.Errorf("failed to create head: %w", err)
	}

	m.generation++
	releaseHeadFn := headRecord.Acquire()

	m.counter.With(prometheus.Labels{"type": "created"}).Inc()
	return NewDiscardableRotatableHead(
		h,
		func(id string, err error) error {
			if _, rotateErr := m.catalog.SetStatus(id, catalog.StatusRotated); rotateErr != nil {
				return errors.Join(err, rotateErr)
			}
			m.counter.With(prometheus.Labels{"type": "rotated"}).Inc()
			return err
		},
		func(id string) error {
			var discardErr error
			if _, discardErr = m.catalog.SetStatus(id, catalog.StatusPersisted); discardErr != nil {
				return discardErr
			}
			m.counter.With(prometheus.Labels{"type": "persisted"}).Inc()
			return nil
		},
		func(id string) error {
			releaseHeadFn()
			return nil
		},
	), nil
}
