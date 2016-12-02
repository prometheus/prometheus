// Package tsdb implements a time series storage for float64 sample data.
package tsdb

import (
	"encoding/binary"
	"sync"
	"time"

	"github.com/fabxc/tsdb/chunks"
	"github.com/prometheus/common/log"
)

// DefaultOptions used for the DB.
var DefaultOptions = &Options{
	StalenessDelta: 5 * time.Minute,
}

// Options of the DB storage.
type Options struct {
	StalenessDelta time.Duration
}

// DB is a time series storage.
type DB struct {
	logger log.Logger
	opts   *Options

	shards map[uint64]*TimeShards
}

// Open or create a new DB.
func Open(path string, l log.Logger, opts *Options) (*DB, error) {
	if opts == nil {
		opts = DefaultOptions
	}

	c := &DB{
		logger:    l,
		opts:      opts,
	}

	return c, nil
}

type Label struct {
	Name, Value string
}

// LabelSet is a sorted set of labels. Order has to be guaranteed upon
// instantiation.
type LabelSet []Label

func (ls LabelSet) Len() int { return len(ls) }
func (ls LabelSet) Swap(i, j int) { ls[i], ls[j] = ls[j], ls[i]}
func (ls LabelSet) Less(i, j int) bool { return ls[i].Name < ls[j].Name }

// NewLabelSet returns a sorted LabelSet from the given labels.
// The caller has to guarantee that all label names are unique.
func NewLabelSet(ls ...Label) LabelSet {
	set := make(LabelSet, 0, len(l))
	for _, l := range ls {
		set = append(set, l)
	}
	sort.Sort(set)

	return set
}

type Vector struct {
	LabelSets []LabelSet
	Values []float64
}

func (db *DB) AppendVector(v *Vector) error {
	return nil
}