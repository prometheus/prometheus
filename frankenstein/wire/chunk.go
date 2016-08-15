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

type ChunksByID []Chunk

func (cs ChunksByID) Len() int           { return len(cs) }
func (cs ChunksByID) Swap(i, j int)      { cs[i], cs[j] = cs[j], cs[i] }
func (cs ChunksByID) Less(i, j int) bool { return cs[i].ID < cs[j].ID }
