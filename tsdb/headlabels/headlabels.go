// Package headlabels provides a disk-backed storage solution for Prometheus series labels
// from the head block. It aims to reduce memory usage by storing labels on disk and
// referencing them with a lightweight 64-bit pointer.
package headlabels

import (
	"encoding/binary"
	"errors"
	"fmt"
	"hash/crc32"
	"io"
	"os"
	"path/filepath"
	"sync"

	"github.com/prometheus/prometheus/model/labels"
	// You would typically use a proven, thread-safe LRU cache implementation.
	// For this example, we'll use a placeholder. A popular choice is:
	// lru "github.com/hashicorp/golang-lru/v2"
)

const (
	// DefaultNumShards is the default number of files to shard label data across.
	DefaultNumShards = 16
	// The directory name within the TSDB data directory.
	dirName = "head_labels"
	// Suffix for temporary files during truncation.
	tmpSuffix = ".tmp"
	// Bit shift for packing the shard ID into the reference.
	shardIDShift = 56
	// Mask to extract the offset from the reference.
	offsetMask = (1 << shardIDShift) - 1
)

var (
	// ErrNotFound is returned when a label set is not found for a given reference.
	ErrNotFound = errors.New("labels not found")
	// crc32Table is pre-calculated for performance.
	crc32Table = crc32.MakeTable(crc32.Castagnoli)
)

// --- LRU Cache Placeholder ---
// In a real implementation, you would replace this with a proper LRU cache library.
type LRUCache interface {
	Add(key, value interface{})
	Get(key interface{}) (value interface{}, ok bool)
}

// A simple, non-performant map to act as a placeholder for the LRU cache.
type simpleMapCache struct {
	mu sync.Mutex
	m  map[uint64]labels.Labels
}

func newSimpleMapCache(size int) LRUCache {
	return &simpleMapCache{m: make(map[uint64]labels.Labels)}
}
func (c *simpleMapCache) Add(key, value interface{}) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.m[key.(uint64)] = value.(labels.Labels)
}
func (c *simpleMapCache) Get(key interface{}) (interface{}, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	val, ok := c.m[key.(uint64)]
	return val, ok
}

// --- Main Structs ---

// DiskLabels is the main manager for the sharded on-disk label storage.
type DiskLabels struct {
	dir    string
	mu     sync.RWMutex // Protects the shards slice during truncation.
	shards []*shard
	cache  LRUCache
}

// shard represents a single file for storing label sets.
type shard struct {
	mu   sync.Mutex // Protects file handle and writes.
	f    *os.File
	path string
}

// NewDiskLabels creates a new manager for on-disk head labels.
// It creates the directory and shard files if they don't exist.
func NewDiskLabels(dataDir string, numShards, cacheSize int) (*DiskLabels, error) {
	dir := filepath.Join(dataDir, dirName)
	if err := os.MkdirAll(dir, 0777); err != nil {
		return nil, fmt.Errorf("create head_labels directory %s: %w", dir, err)
	}

	shards := make([]*shard, numShards)
	for i := 0; i < numShards; i++ {
		path := filepath.Join(dir, fmt.Sprintf("%02d", i))
		f, err := os.OpenFile(path, os.O_WRONLY|os.O_APPEND|os.O_CREATE, 0666)
		if err != nil {
			// Close any already opened files before failing.
			for j := 0; j < i; j++ {
				shards[j].f.Close()
			}
			return nil, fmt.Errorf("open shard file %s: %w", path, err)
		}
		shards[i] = &shard{f: f, path: path}
	}

	// In a real implementation:
	// cache, err := lru.New(cacheSize)
	// if err != nil { ... }
	cache := newSimpleMapCache(cacheSize)

	return &DiskLabels{
		dir:    dir,
		shards: shards,
		cache:  cache,
	}, nil
}

// Close closes all open file handles for the shards.
func (dl *DiskLabels) Close() error {
	dl.mu.RLock()
	defer dl.mu.RUnlock()
	var firstErr error
	for _, s := range dl.shards {
		if err := s.f.Close(); err != nil && firstErr == nil {
			firstErr = err
		}
	}
	return firstErr
}

// --- Write Path ---

// WriteLabels encodes a label set and appends it to the appropriate shard file.
// It returns a packed 64-bit reference containing the shard ID and file offset.
func (dl *DiskLabels) WriteLabels(seriesRef, shardHash uint64, lset labels.Labels) (uint64, error) {
	dl.mu.RLock()
	defer dl.mu.RUnlock()

	shardID := shardHash % uint64(len(dl.shards))
	s := dl.shards[shardID]

	s.mu.Lock()
	defer s.mu.Unlock()

	// Get current file offset before writing.
	offset, err := s.f.Seek(0, io.SeekEnd)
	if err != nil {
		return 0, fmt.Errorf("seek to end of shard %d: %w", shardID, err)
	}

	// Encode the label set into a temporary buffer.
	buf := make([]byte, 0, 256)
	buf, err = encodeLabels(seriesRef, lset, buf)
	if err != nil {
		return 0, fmt.Errorf("encode labels: %w", err)
	}

	// Write the encoded data to the shard file.
	if _, err := s.f.Write(buf); err != nil {
		return 0, fmt.Errorf("write to shard %d: %w", shardID, err)
	}

	// Pack shard and offset into a single reference.
	ref := packRef(shardID, uint64(offset))

	// Add to cache to avoid a read-after-write disk hit.
	dl.cache.Add(seriesRef, lset)

	return ref, nil
}

// --- Read Path ---

// ReadLabels retrieves a label set using its packed reference.
// It checks the LRU cache first before falling back to disk.
func (dl *DiskLabels) ReadLabels(seriesRef, ref uint64) (labels.Labels, error) {
	// 1. Check cache first.
	if lset, ok := dl.cache.Get(seriesRef); ok {
		return lset.(labels.Labels), nil
	}

	dl.mu.RLock()
	defer dl.mu.RUnlock()

	// 2. Unpack reference and find the correct shard.
	shardID, offset := unpackRef(ref)
	if shardID >= uint64(len(dl.shards)) {
		return labels.Labels{}, fmt.Errorf("invalid shard ID %d from reference", shardID)
	}
	s := dl.shards[shardID]

	// 3. Read from disk. This requires a new file handle for reading.
	// We don't use the write handle to avoid seeks and concurrency issues.
	f, err := os.Open(s.path)
	if err != nil {
		return labels.Labels{}, fmt.Errorf("open shard %d for reading: %w", shardID, err)
	}
	defer f.Close()

	// We read the entire record into a buffer.
	// First, read the length prefix to know the record size.
	lenBuf := make([]byte, binary.MaxVarintLen32)
	if _, err := f.ReadAt(lenBuf, int64(offset)); err != nil {
		return labels.Labels{}, fmt.Errorf("read record length from shard %d at offset %d: %w", shardID, offset, err)
	}
	recordLen, n := binary.Uvarint(lenBuf)
	if n <= 0 {
		return labels.Labels{}, fmt.Errorf("invalid record length at shard %d offset %d", shardID, offset)
	}

	// Now read the full record.
	recordBuf := make([]byte, recordLen)
	if _, err := f.ReadAt(recordBuf, int64(offset)+int64(n)); err != nil {
		return labels.Labels{}, fmt.Errorf("read record data from shard %d: %w", shardID, offset, err)
	}

	// 4. Decode the record.
	decodedRef, lset, err := decodeLabels(recordBuf)
	if err != nil {
		return labels.Labels{}, fmt.Errorf("decode labels from shard %d: %w", shardID, err)
	}
	if decodedRef != seriesRef {
		return labels.Labels{}, fmt.Errorf("series ref mismatch: expected %d, got %d", seriesRef, decodedRef)
	}

	// 5. Add to cache.
	dl.cache.Add(seriesRef, lset)

	return lset, nil
}

// --- Truncation ---

// Truncate garbage collects the shard files after a head compaction.
// It iterates through all records, rewriting only the ones for which `isSeriesActive` returns true.
// It returns a map of old references to new references, which the caller MUST use to update
// the `memSeries` structs in the head.
func (dl *DiskLabels) Truncate(isSeriesActive func(seriesRef uint64) bool) (map[uint64]uint64, error) {
	dl.mu.Lock() // Exclusive lock to replace the shards.
	defer dl.mu.Unlock()

	refUpdates := make(map[uint64]uint64)
	newShards := make([]*shard, len(dl.shards))
	oldShards := dl.shards
	dl.shards = nil // Prevent access to old shards during truncation.

	// Process each shard one by one.
	for i, oldS := range oldShards {
		// Create a new temporary shard file.
		tmpPath := oldS.path + tmpSuffix
		tmpFile, err := os.OpenFile(tmpPath, os.O_WRONLY|os.O_APPEND|os.O_CREATE, 0666)
		if err != nil {
			// Abort and cleanup.
			return nil, fmt.Errorf("create tmp shard file %s: %w", tmpPath, err)
		}

		// Open the old shard for reading.
		readF, err := os.Open(oldS.path)
		if err != nil {
			tmpFile.Close()
			return nil, fmt.Errorf("open old shard file for truncation %s: %w", oldS.path, err)
		}

		var currentOffset int64 = 0
		for {
			// Read and decode each record from the old shard.
			lenBuf := make([]byte, binary.MaxVarintLen32)
			n, err := readF.Read(lenBuf)
			if err == io.EOF {
				break // End of file.
			}
			if err != nil {
				readF.Close()
				tmpFile.Close()
				return nil, fmt.Errorf("truncate: read record length from shard %d: %w", i, err)
			}

			recordLen, lenN := binary.Uvarint(lenBuf[:n])
			if lenN <= 0 {
				return nil, fmt.Errorf("truncate: invalid record length in shard %d", i)
			}

			oldRef := packRef(uint64(i), uint64(currentOffset))
			currentOffset += int64(lenN)

			recordBuf := make([]byte, recordLen)
			if _, err := io.ReadFull(readF, recordBuf); err != nil {
				return nil, fmt.Errorf("truncate: read record data from shard %d: %w", i, err)
			}
			currentOffset += int64(recordLen)

			// Decode just enough to get the series ref.
			decodedRef, _, err := decodeLabels(recordBuf)
			if err != nil {
				// Log this error but continue.
				fmt.Fprintf(os.Stderr, "WARN: skipping undecodable record in shard %d: %v\n", i, err)
				continue
			}

			// If the series is still active, rewrite it to the new shard file.
			if isSeriesActive(decodedRef) {
				newOffset, err := tmpFile.Seek(0, io.SeekEnd)
				if err != nil {
					return nil, fmt.Errorf("truncate: seek tmp shard %d: %w", i, err)
				}

				// Re-write the original record (len prefix + data).
				if _, err := tmpFile.Write(lenBuf[:lenN]); err != nil {
					return nil, err
				}
				if _, err := tmpFile.Write(recordBuf); err != nil {
					return nil, err
				}

				newRef := packRef(uint64(i), uint64(newOffset))
				refUpdates[oldRef] = newRef
			}
		}
		readF.Close()

		// Store the new shard file handle.
		newShards[i] = &shard{f: tmpFile, path: oldS.path}
	}

	// Atomically replace old files with new ones.
	for i, _ := range oldShards {
		newS := newShards[i]
		if err := newS.f.Close(); err != nil { // Close before renaming.
			return nil, err
		}
		if err := os.Rename(newS.path+tmpSuffix, newS.path); err != nil {
			return nil, err
		}
		// Re-open the new file for appending.
		f, err := os.OpenFile(newS.path, os.O_WRONLY|os.O_APPEND, 0666)
		if err != nil {
			return nil, err
		}
		newS.f = f
	}

	// Close old file handles and replace the shard slice.
	for _, s := range oldShards {
		s.f.Close()
	}
	dl.shards = newShards

	return refUpdates, nil
}

// --- Encoding/Decoding Helpers ---

// packRef combines a shard ID and an offset into a single uint64.
func packRef(shardID, offset uint64) uint64 {
	return (shardID << shardIDShift) | (offset & offsetMask)
}

// unpackRef extracts the shard ID and offset from a uint64 reference.
func unpackRef(ref uint64) (shardID, offset uint64) {
	shardID = ref >> shardIDShift
	offset = ref & offsetMask
	return
}

// encodeLabels converts a series ref and label set into the on-disk byte format.
// Format: [series_ref uvarint64 | num_labels uvarint32 | l_1_name_len uvarint32 | l_1_name | ... | crc32 uint32]
// The final returned buffer does NOT include the initial record length prefix.
func encodeLabels(seriesRef uint64, lset labels.Labels, buf []byte) ([]byte, error) {
	startLen := len(buf)

	buf = binary.AppendUvarint(buf, seriesRef)
	buf = binary.AppendUvarint(buf, uint64(lset.Len()))

	// TODO: just put the string of the lset
	lset.Range(func(l labels.Label) {
		buf = binary.AppendUvarint(buf, uint64(len(l.Name)))
		buf = append(buf, l.Name...)
		buf = binary.AppendUvarint(buf, uint64(len(l.Value)))
		buf = append(buf, l.Value...)
	})

	// Calculate CRC over the data we just wrote.
	chksum := crc32.Checksum(buf[startLen:], crc32Table)
	buf = binary.BigEndian.AppendUint32(buf, chksum)

	// Prepend the total length of the record (excluding the length prefix itself).
	recordLen := len(buf) - startLen
	lenBuf := make([]byte, binary.MaxVarintLen32)
	n := binary.PutUvarint(lenBuf, uint64(recordLen))

	finalBuf := make([]byte, n+recordLen)
	copy(finalBuf, lenBuf[:n])
	copy(finalBuf[n:], buf[startLen:])

	return finalBuf, nil
}

// decodeLabels converts bytes from disk back into a series ref and label set.
// It assumes `b` is the record data *without* the initial length prefix.
func decodeLabels(b []byte) (uint64, labels.Labels, error) {
	if len(b) < 4 {
		return 0, labels.Labels{}, io.ErrUnexpectedEOF
	}

	// Validate checksum first.
	expectedCRC := binary.BigEndian.Uint32(b[len(b)-4:])
	actualCRC := crc32.Checksum(b[:len(b)-4], crc32Table)
	if expectedCRC != actualCRC {
		return 0, labels.Labels{}, errors.New("checksum mismatch")
	}

	d := &decoder{b: b[:len(b)-4]} // Exclude CRC from decoder buffer.

	seriesRef := d.uvarint()
	numLabels := int(d.uvarint())
	if d.err != nil {
		return 0, labels.Labels{}, d.err
	}

	// TODO: update this after updating the TODO for encodeLabels
	//lset := make(labels.Labels, numLabels)
	//for i := 0; i < numLabels; i++ {
	//	lset[i].Name = d.string()
	//	lset[i].Value = d.string()
	//	if d.err != nil {
	//		return 0, labels.Labels{}, d.err
	//	}
	//}

	return seriesRef, lset, nil
}

// decoder helps safely read from a byte slice.
type decoder struct {
	b   []byte
	err error
}

func (d *decoder) uvarint() uint64 {
	if d.err != nil {
		return 0
	}
	v, n := binary.Uvarint(d.b)
	if n <= 0 {
		d.err = io.ErrUnexpectedEOF
		return 0
	}
	d.b = d.b[n:]
	return v
}

func (d *decoder) string() string {
	if d.err != nil {
		return ""
	}
	l := int(d.uvarint())
	if d.err != nil {
		return ""
	}
	if len(d.b) < l {
		d.err = io.ErrUnexpectedEOF
		return ""
	}
	s := string(d.b[:l])
	d.b = d.b[l:]
	return s
}
