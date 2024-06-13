package rules

import (
	"encoding/json"
	"os"
	"sync"

	"github.com/go-kit/log"
	"github.com/go-kit/log/level"
)

type State struct {
	alerts []*Alert
	key    uint64
}

type AlertStore interface {
	// SetAlerts stores the provided list of alerts for a rule
	SetAlerts(key uint64, alerts []*Alert)
	// GetAlerts returns a list of alerts for each rule in group,
	// rule is identified by a hash of its config.
	GetAlerts(key uint64) map[uint64]*Alert
}

type FileStore struct {
	logger      log.Logger
	stateByRule map[uint64][]*Alert
	//protects the `stateByRule` map
	stateMtx sync.RWMutex
	path     string
}

func NewFileStore(l log.Logger, storagePath string) *FileStore {
	s := &FileStore{
		logger:      l,
		stateByRule: make(map[uint64][]*Alert),
		path:        storagePath,
	}
	s.initState()
	return s
}

// initState reads the state from file storage into the stateByRule map
func (s *FileStore) initState() {
	file, err := os.Open(s.path)
	if err != nil {
		level.Error(s.logger).Log("Error reading alerts state from file", err)
	}
	defer file.Close()

	var stateByRule map[uint64][]*Alert
	err = json.NewDecoder(file).Decode(&stateByRule)
	if err != nil {
		level.Error(s.logger).Log("Error reading alerts state from file", err)
	}
	if stateByRule == nil {
		stateByRule = make(map[uint64][]*Alert)
	}
	s.stateByRule = stateByRule
}

// GetAlerts returns the stored alerts for an alerting rule
// Alert state is read from the in memory map which is populated during initialization
func (s *FileStore) GetAlerts(key uint64) map[uint64]*Alert {
	s.stateMtx.RLock()
	defer s.stateMtx.RUnlock()

	restoredAlerts, ok := s.stateByRule[key]
	if !ok {
		return nil
	}
	alerts := make(map[uint64]*Alert)
	for _, alert := range restoredAlerts {
		if alert == nil {
			continue
		}
		h := alert.Labels.Hash()
		alerts[h] = alert
	}

	return alerts
}

// SetAlerts updates the stateByRule map and writes state to file storage
func (s *FileStore) SetAlerts(key uint64, alerts []*Alert) {
	s.stateMtx.Lock()
	defer s.stateMtx.Unlock()
	level.Debug(s.logger).Log("msg", "saving alert state", "key", key, "alerts", len(alerts))
	// Update in memory
	if alerts != nil {
		s.stateByRule[key] = alerts
	}

	// flush in memory state to file storage
	file, err := os.Create(s.path)
	if err != nil {
		level.Error(s.logger).Log("Error writing alerts state to file", err)
	}
	defer file.Close()

	encoder := json.NewEncoder(file)
	err = encoder.Encode(s.stateByRule)

	if err != nil {
		level.Error(s.logger).Log("Error writing alerts state to file", err)
	}
}
