// Copyright 2016 The Prometheus Authors

package wire

import (
	"github.com/prometheus/common/model"
)

// Chunk contains encoded timeseries data
type Chunk struct {
	ID      string       `json:"-"`
	From    model.Time   `json:"from"`
	Through model.Time   `json:"through"`
	Metric  model.Metric `json:"metric"`
	Data    []byte       `json:"-"`
}
