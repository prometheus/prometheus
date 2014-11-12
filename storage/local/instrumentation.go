// Copyright 2014 Prometheus Team
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

package local

import "github.com/prometheus/client_golang/prometheus"

// Usually, a separate file for instrumentation is frowned upon. Metrics should
// be close to where they are used. However,the metrics below are set all over
// the place, so we go for a separate instrumentation file in this case.
var (
	chunkOps = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: namespace,
			Subsystem: subsystem,
			Name:      "chunk_ops_total",
			Help:      "The total number of chunk operations by their type.",
		},
		[]string{opTypeLabel},
	)
	chunkDescOps = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: namespace,
			Subsystem: subsystem,
			Name:      "chunkdesc_ops_total",
			Help:      "The total number of chunk descriptor operations by their type.",
		},
		[]string{opTypeLabel},
	)
)

const (
	namespace = "prometheus"
	subsystem = "local_storage"

	opTypeLabel = "type"

	// Op-types for seriesOps.
	create             = "create"
	archive            = "archive"
	unarchive          = "unarchive"
	memoryPurge        = "purge_from_memory"
	archivePurge       = "purge_from_archive"
	memoryMaintenance  = "maintenance_in_memory"
	archiveMaintenance = "maintenance_in_archive"

	// Op-types for chunkOps.
	createAndPin    = "create" // A chunkDesc creation with refCount=1.
	persistAndUnpin = "persist"
	pin             = "pin"   // Excluding the pin on creation.
	unpin           = "unpin" // Excluding the unpin on persisting.
	clone           = "clone"
	transcode       = "transcode"
	purge           = "purge"

	// Op-types for chunkOps and chunkDescOps.
	evict = "evict"
	load  = "load"
)

func init() {
	prometheus.MustRegister(chunkOps)
	prometheus.MustRegister(chunkDescOps)
}

var (
	// Global counters, also used internally, so not implemented as
	// metrics. Collected in memorySeriesStorage.Collect.
	numMemChunks, numMemChunkDescs int64

	// Metric descriptors for the above.
	numMemChunksDesc = prometheus.NewDesc(
		prometheus.BuildFQName(namespace, subsystem, "memory_chunks"),
		"The current number of chunks in memory, excluding cloned chunks (i.e. chunks without a descriptor).",
		nil, nil,
	)
	numMemChunkDescsDesc = prometheus.NewDesc(
		prometheus.BuildFQName(namespace, subsystem, "memory_chunkdescs"),
		"The current number of chunk descriptors in memory.",
		nil, nil,
	)
)
