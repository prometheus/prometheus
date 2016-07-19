// Copyright 2016 The Prometheus Authors

package local

import (
	"github.com/prometheus/common/model"
)

type noopPersistence struct{}

func (noopPersistence) loadFPMappings() (fpMappings, model.Fingerprint, error) {
	return fpMappings{}, model.Fingerprint(0), nil
}

func (noopPersistence) checkpointFPMappings(fpMappings) error {
	return nil
}

func (noopPersistence) archivedMetric(m model.Fingerprint) (model.Metric, error) {
	return nil, nil
}
