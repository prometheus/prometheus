package rules

import (
	"encoding/json"
	"log/slog"
	"os"
	"sync"
)

// AlertStore provides persistent storage of alert state.
type AlertStore interface {
	// SetAlerts stores the provided list of alerts for a rule.
	SetAlerts(key uint64, alerts []*Alert) error
	// GetAlerts returns a list of alerts for each alerting rule,
	// alerting rule is identified by a fingerprint of its config.
	GetAlerts(key uint64) (map[uint64]*Alert, error)
}

// FileStore implements the AlertStore interface.
type FileStore struct {
	logger       *slog.Logger
	alertsByRule map[uint64][]*Alert
	//protects the `alertsByRule` map
	stateMtx sync.RWMutex
	path     string
}

func NewFileStore(l *slog.Logger, storagePath string) *FileStore {
	s := &FileStore{
		logger:       l,
		alertsByRule: make(map[uint64][]*Alert),
		path:         storagePath,
	}
	s.initState()
	return s
}

// initState reads the state from file storage into the alertsByRule map
func (s *FileStore) initState() {
	file, err := os.OpenFile(s.path, os.O_RDWR|os.O_CREATE, 0o666)
	if err != nil {
		s.logger.Error("Failed reading alerts state from file", "err", err)
		return
	}
	defer file.Close()

	var alertsByRule map[uint64][]*Alert
	err = json.NewDecoder(file).Decode(&alertsByRule)
	if err != nil {
		s.logger.Error("Failed reading alerts state from file", "err", err)
	}
	if alertsByRule == nil {
		alertsByRule = make(map[uint64][]*Alert)
	}
	s.alertsByRule = alertsByRule
}

// GetAlerts returns the stored alerts for an alerting rule
// Alert state is read from the in memory map which is populated during initialization
func (s *FileStore) GetAlerts(key uint64) (map[uint64]*Alert, error) {
	s.stateMtx.RLock()
	defer s.stateMtx.RUnlock()

	restoredAlerts, ok := s.alertsByRule[key]
	if !ok {
		return nil, nil
	}
	alerts := make(map[uint64]*Alert)
	for _, alert := range restoredAlerts {
		if alert == nil {
			continue
		}
		h := alert.Labels.Hash()
		alerts[h] = alert
	}
	return alerts, nil
}

// SetAlerts updates the stateByRule map and writes state to file storage
func (s *FileStore) SetAlerts(key uint64, alerts []*Alert) error {
	s.stateMtx.Lock()
	defer s.stateMtx.Unlock()

	// Update in memory
	if alerts != nil {
		s.alertsByRule[key] = alerts
	}
	// flush in memory state to file storage
	file, err := os.Create(s.path)
	if err != nil {
		return err
	}
	defer file.Close()

	encoder := json.NewEncoder(file)
	err = encoder.Encode(s.alertsByRule)
	if err != nil {
		return err
	}
	return nil
}
