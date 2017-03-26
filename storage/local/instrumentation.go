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

package local

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
	requestedPurge     = "purge_on_request"
	memoryMaintenance  = "maintenance_in_memory"
	archiveMaintenance = "maintenance_in_archive"
	completedQurantine = "quarantine_completed"
	droppedQuarantine  = "quarantine_dropped"
	failedQuarantine   = "quarantine_failed"

	seriesLocationLabel = "location"

	// Maintenance types for maintainSeriesDuration.
	maintainInMemory = "memory"
	maintainArchived = "archived"

	discardReasonLabel = "reason"

	// Reasons to discard samples.
	outOfOrderTimestamp = "timestamp_out_of_order"
	duplicateSample     = "multiple_values_for_timestamp"
)
