// Copyright 2014 The Prometheus Authors
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
// http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package chunk

import "github.com/prometheus/client_golang/prometheus"

// Usually, a separate file for instrumentation is frowned upon. Metrics should
// be close to where they are used. However, the metrics below are set all over
// the place, so we go for a separate instrumentation file in this case.
var (
	Ops = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: namespace,
			Subsystem: subsystem,
			Name:      "chunk_ops_total",
			Help:      "The total number of chunk operations by their type.",
		},
		[]string{OpTypeLabel},
	)
	DescOps = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: namespace,
			Subsystem: subsystem,
			Name:      "chunkdesc_ops_total",
			Help:      "The total number of chunk descriptor operations by their type.",
		},
		[]string{OpTypeLabel},
	)
	NumMemDescs = prometheus.NewGauge(prometheus.GaugeOpts{
		Namespace: namespace,
		Subsystem: subsystem,
		Name:      "memory_chunkdescs",
		Help:      "The current number of chunk descriptors in memory.",
	})
)

const (
	namespace = "prometheus"
	subsystem = "local_storage"

	// OpTypeLabel is the label name for chunk operation types.
	OpTypeLabel = "type"

	// Op-types for ChunkOps.

	// CreateAndPin is the label value for create-and-pin chunk ops.
	CreateAndPin = "create" // A Desc creation with refCount=1.
	// PersistAndUnpin is the label value for persist chunk ops.
	PersistAndUnpin = "persist"
	// Pin is the label value for pin chunk ops (excludes pin on creation).
	Pin = "pin"
	// Unpin is the label value for unpin chunk ops (excludes the unpin on persisting).
	Unpin = "unpin"
	// Clone is the label value for clone chunk ops.
	Clone = "clone"
	// Transcode is the label value for transcode chunk ops.
	Transcode = "transcode"
	// Drop is the label value for drop chunk ops.
	Drop = "drop"

	// Op-types for ChunkOps and ChunkDescOps.

	// Evict is the label value for evict chunk desc ops.
	Evict = "evict"
	// Load is the label value for load chunk and chunk desc ops.
	Load = "load"
)

func init() {
	prometheus.MustRegister(Ops)
	prometheus.MustRegister(DescOps)
	prometheus.MustRegister(NumMemDescs)
}

// NumMemChunks is the total number of chunks in memory. This is a global
// counter, also used internally, so not implemented as metrics. Collected in
// MemorySeriesStorage.
// TODO(beorn7): Having this as an exported global variable is really bad.
var NumMemChunks int64
