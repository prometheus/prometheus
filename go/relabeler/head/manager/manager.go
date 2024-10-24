package manager

import (
	"errors"
	"fmt"
	"github.com/google/uuid"
	"github.com/prometheus/prometheus/pp/go/relabeler"
	"github.com/prometheus/prometheus/pp/go/relabeler/config"
	"github.com/prometheus/prometheus/pp/go/relabeler/head"
	"github.com/prometheus/prometheus/pp/go/relabeler/head/catalog"
	"github.com/prometheus/client_golang/prometheus"
	"os"
	"path/filepath"
	"time"
)

type ConfigSource interface {
	Get() (inputRelabelerConfigs []*config.InputRelabelerConfig, numberOfShards uint16)
}

type Catalog interface {
	List(filter func(record catalog.Record) bool, sortLess func(lhs, rhs catalog.Record) bool) ([]catalog.Record, error)
	Create(id, dir string, numberOfShards uint16) (catalog.Record, error)
	SetStatus(id string, status catalog.Status) (catalog.Record, error)
	Delete(id string) error
}

type Manager struct {
	dir          string
	configSource ConfigSource
	catalog      Catalog
	generation   uint64
	registerer   prometheus.Registerer
}

func New(dir string, configSource ConfigSource, catalog Catalog, registerer prometheus.Registerer) (*Manager, error) {
	dirStat, err := os.Stat(dir)
	if err != nil {
		return nil, fmt.Errorf("failed to stat dir: %w", err)
	}

	if !dirStat.IsDir() {
		return nil, fmt.Errorf("%s is not directory", dir)
	}

	return &Manager{
		dir:          dir,
		configSource: configSource,
		catalog:      catalog,
		registerer:   registerer,
	}, nil
}

func (m *Manager) Restore(blockDuration time.Duration) (active relabeler.Head, rotated []relabeler.Head, err error) {
	headRecords, err := m.catalog.List(
		func(record catalog.Record) bool {
			return record.DeletedAt == 0 && record.Status != catalog.StatusPersisted
		},
		func(lhs, rhs catalog.Record) bool {
			return lhs.CreatedAt < rhs.CreatedAt
		},
	)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to list head records: %w", err)
	}

	for index, headRecord := range headRecords {
		var h relabeler.Head
		dir := filepath.Join(m.dir, headRecord.Dir)
		cfgs, _ := m.configSource.Get()
		h, err = head.Load(headRecord.ID, m.generation, dir, cfgs, headRecord.NumberOfShards, m.registerer)
		if err != nil {
			_, setStatusErr := m.catalog.SetStatus(headRecord.ID, catalog.StatusCorrupted)
			err = errors.Join(err, setStatusErr)
			return nil, nil, err
		}
		h = NewDiscardableHead(h, func(id string) error {
			var discardErr error
			if _, discardErr = m.catalog.SetStatus(id, catalog.StatusPersisted); discardErr != nil {
				return discardErr
			}
			if discardErr = os.RemoveAll(dir); discardErr != nil {
				return discardErr
			}
			if discardErr = m.catalog.Delete(id); discardErr != nil {
				return discardErr
			}
			return nil
		})

		if index == len(headRecords)-1 && time.Now().Sub(time.UnixMilli(headRecord.CreatedAt)).Milliseconds() < blockDuration.Milliseconds() {
			active = h
			continue
		}
		h.Finalize()
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
	id := uuid.New().String()
	dir := filepath.Join(m.dir, id)
	if err = os.Mkdir(dir, 0777); err != nil {
		return nil, err
	}
	defer func() {
		if err != nil {
			//err = errors.Join(err, os.RemoveAll(dir))
		}
	}()

	generation := m.generation

	h, err = head.Create(id, generation, dir, inputRelabelerConfigs, numberOfShards, m.registerer)
	if err != nil {
		return nil, fmt.Errorf("failed to create head: %w", err)
	}

	if _, err = m.catalog.Create(id, id, numberOfShards); err != nil {
		return nil, err
	}

	m.generation++

	return NewDiscardableHead(
		h,
		func(id string) error {
			var discardErr error
			if _, discardErr = m.catalog.SetStatus(id, catalog.StatusPersisted); discardErr != nil {
				return discardErr
			}
			if discardErr = os.RemoveAll(dir); discardErr != nil {
				return discardErr
			}
			if discardErr = m.catalog.Delete(id); discardErr != nil {
				return discardErr
			}
			return nil
		},
	), nil
}
