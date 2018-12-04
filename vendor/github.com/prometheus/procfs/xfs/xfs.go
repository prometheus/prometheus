// Copyright 2017 The Prometheus Authors
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

// Package xfs provides access to statistics exposed by the XFS filesystem.
package xfs

// Stats contains XFS filesystem runtime statistics, parsed from
// /proc/fs/xfs/stat.
//
// The names and meanings of each statistic were taken from
// http://xfs.org/index.php/Runtime_Stats and xfs_stats.h in the Linux
// kernel source. Most counters are uint32s (same data types used in
// xfs_stats.h), but some of the "extended precision stats" are uint64s.
type Stats struct {
	// The name of the filesystem used to source these statistics.
	// If empty, this indicates aggregated statistics for all XFS
	// filesystems on the host.
	Name string

	ExtentAllocation   ExtentAllocationStats
	AllocationBTree    BTreeStats
	BlockMapping       BlockMappingStats
	BlockMapBTree      BTreeStats
	DirectoryOperation DirectoryOperationStats
	Transaction        TransactionStats
	InodeOperation     InodeOperationStats
	LogOperation       LogOperationStats
	ReadWrite          ReadWriteStats
	AttributeOperation AttributeOperationStats
	InodeClustering    InodeClusteringStats
	Vnode              VnodeStats
	Buffer             BufferStats
	ExtendedPrecision  ExtendedPrecisionStats
}

// ExtentAllocationStats contains statistics regarding XFS extent allocations.
type ExtentAllocationStats struct {
	ExtentsAllocated uint32
	BlocksAllocated  uint32
	ExtentsFreed     uint32
	BlocksFreed      uint32
}

// BTreeStats contains statistics regarding an XFS internal B-tree.
type BTreeStats struct {
	Lookups         uint32
	Compares        uint32
	RecordsInserted uint32
	RecordsDeleted  uint32
}

// BlockMappingStats contains statistics regarding XFS block maps.
type BlockMappingStats struct {
	Reads                uint32
	Writes               uint32
	Unmaps               uint32
	ExtentListInsertions uint32
	ExtentListDeletions  uint32
	ExtentListLookups    uint32
	ExtentListCompares   uint32
}

// DirectoryOperationStats contains statistics regarding XFS directory entries.
type DirectoryOperationStats struct {
	Lookups  uint32
	Creates  uint32
	Removes  uint32
	Getdents uint32
}

// TransactionStats contains statistics regarding XFS metadata transactions.
type TransactionStats struct {
	Sync  uint32
	Async uint32
	Empty uint32
}

// InodeOperationStats contains statistics regarding XFS inode operations.
type InodeOperationStats struct {
	Attempts        uint32
	Found           uint32
	Recycle         uint32
	Missed          uint32
	Duplicate       uint32
	Reclaims        uint32
	AttributeChange uint32
}

// LogOperationStats contains statistics regarding the XFS log buffer.
type LogOperationStats struct {
	Writes            uint32
	Blocks            uint32
	NoInternalBuffers uint32
	Force             uint32
	ForceSleep        uint32
}

// ReadWriteStats contains statistics regarding the number of read and write
// system calls for XFS filesystems.
type ReadWriteStats struct {
	Read  uint32
	Write uint32
}

// AttributeOperationStats contains statistics regarding manipulation of
// XFS extended file attributes.
type AttributeOperationStats struct {
	Get    uint32
	Set    uint32
	Remove uint32
	List   uint32
}

// InodeClusteringStats contains statistics regarding XFS inode clustering
// operations.
type InodeClusteringStats struct {
	Iflush     uint32
	Flush      uint32
	FlushInode uint32
}

// VnodeStats contains statistics regarding XFS vnode operations.
type VnodeStats struct {
	Active   uint32
	Allocate uint32
	Get      uint32
	Hold     uint32
	Release  uint32
	Reclaim  uint32
	Remove   uint32
	Free     uint32
}

// BufferStats contains statistics regarding XFS read/write I/O buffers.
type BufferStats struct {
	Get             uint32
	Create          uint32
	GetLocked       uint32
	GetLockedWaited uint32
	BusyLocked      uint32
	MissLocked      uint32
	PageRetries     uint32
	PageFound       uint32
	GetRead         uint32
}

// ExtendedPrecisionStats contains high precision counters used to track the
// total number of bytes read, written, or flushed, during XFS operations.
type ExtendedPrecisionStats struct {
	FlushBytes uint64
	WriteBytes uint64
	ReadBytes  uint64
}
