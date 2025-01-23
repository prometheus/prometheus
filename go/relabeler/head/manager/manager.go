package manager

import (
	"errors"
	"fmt"
	"github.com/prometheus/prometheus/pp/go/relabeler"
	"github.com/prometheus/prometheus/pp/go/relabeler/config"
	"github.com/prometheus/prometheus/pp/go/relabeler/head"
	"github.com/prometheus/prometheus/pp/go/relabeler/head/catalog"
	"github.com/prometheus/prometheus/pp/go/relabeler/logger"
	"github.com/prometheus/prometheus/pp/go/util"
	"github.com/prometheus/client_golang/prometheus"
	"os"
	"path/filepath"
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

func New(dir string, configSource ConfigSource, catalog Catalog, registerer prometheus.Registerer) (*Manager, error) {
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

func (m *Manager) Restore(blockDuration time.Duration) (active relabeler.Head, rotated []relabeler.Head, err error) {
	headRecords, err := m.catalog.List(
		func(record *catalog.Record) bool {
			return record.DeletedAt() == 0
		},
		func(lhs, rhs *catalog.Record) bool {
			return lhs.CreatedAt() < rhs.CreatedAt()
		},
	)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to list head records: %w", err)
	}

	for index, headRecord := range headRecords {
		var h relabeler.Head
		headDir := filepath.Join(m.dir, headRecord.Dir())
		if headRecord.Status() == catalog.StatusPersisted {
			continue
		}

		cfgs, _ := m.configSource.Get()
		var corrupted bool
		hr := headRecord
		h, corrupted, err = head.Load(
			headRecord.ID(),
			m.generation,
			headDir,
			cfgs,
			headRecord.NumberOfShards(),
			SetLastAppendedSegmentIDFn(func(segmentID uint32) {
				hr.SetLastAppendedSegmentID(segmentID)
			}),
			m.registerer)
		if err != nil {
			logger.Errorf("head load totally failed: %v", err)
			m.counter.With(prometheus.Labels{"type": "corrupted"}).Inc()
			continue
		}

		if corrupted {
			if _, setCorruptedErr := m.catalog.SetCorrupted(headRecord.ID()); setCorruptedErr != nil {
				return nil, nil, errors.Join(err, setCorruptedErr)
			}
		}

		m.generation++
		headReleaseFn := headRecord.Acquire()
		h = NewDiscardableRotatableHead(
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
		if !corrupted && index == len(headRecords)-1 && time.Now().Sub(time.UnixMilli(headRecord.CreatedAt())).Milliseconds() < blockDuration.Milliseconds() {
			active = h
			continue
		}
		h.Finalize()
		if headRecord.Status() != catalog.StatusRotated {
			if _, err = m.catalog.SetStatus(headRecord.ID(), catalog.StatusRotated); err != nil {
				return nil, nil, fmt.Errorf("failed to set status: %w", err)
			}
			m.counter.With(prometheus.Labels{"type": "rotated"}).Inc()
		}
		rotated = append(rotated, h)
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
