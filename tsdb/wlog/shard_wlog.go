package wlog

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"log/slog"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/common/promslog"
	"github.com/prometheus/prometheus/util/compression"
)

const ShardDirPrefix = "shard-"

// ShardDir returns "<root>/<ShardDirPrefix>%03d".
func ShardDir(root string, shard int) string {
	return filepath.Join(root, fmt.Sprintf("%s%03d", ShardDirPrefix, shard))
}

// DetectShardedLayout returns the sorted list of shard directories under root (e.g. wal/)
// and a boolean indicating whether sharded layout is present.
func DetectShardedLayout(root string) (dirs []string, ok bool, _ error) {
	entries, err := os.ReadDir(root)
	if os.IsNotExist(err) {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, err
	}
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		if strings.HasPrefix(e.Name(), ShardDirPrefix) {
			dirs = append(dirs, filepath.Join(root, e.Name()))
		}
	}
	sort.Strings(dirs)
	return dirs, len(dirs) > 0, nil
}

// Sharded owns N independent WLs (one per shard dir).
// It does not interpret records; it only orchestrates per-shard WL lifecycles.
//
// IMPORTANT: This is a helper for callers that know which shard to write to.
// Reading/replay must enumerate shards explicitly.
type Sharded struct {
	logger   *slog.Logger
	register prometheus.Registerer
	root     string // e.g. .../wal
	segment  int    // segment size in bytes (same as WL)
	comp     compression.Type

	wals []*WL // len == shards
}

// OpenSharded opens existing shard dirs under root. Fails if none are present.
func OpenSharded(logger *slog.Logger, root string) (*Sharded, error) {
	shardDirs, ok, err := DetectShardedLayout(root)
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, fmt.Errorf("no shard-* directories found under %s", root)
	}
	if logger == nil {
		logger = promslog.NewNopLogger()
	}
	s := &Sharded{logger: logger, root: root}
	s.wals = make([]*WL, 0, len(shardDirs))
	for _, d := range shardDirs {
		w, err := Open(logger, d)
		if err != nil {
			return nil, err
		}
		s.wals = append(s.wals, w)
	}
	return s, nil
}

// NewSharded creates N shard directories (idempotent) and opens N independent WLs.
// Each shard uses identical segment size and compression type.
func NewSharded(logger *slog.Logger, r prometheus.Registerer, root string, shards int, segmentSize int, comp compression.Type) (*Sharded, error) {
	if shards <= 0 {
		return nil, fmt.Errorf("shards must be > 0")
	}
	if logger == nil {
		logger = promslog.NewNopLogger()
	}
	if err := os.MkdirAll(root, 0o777); err != nil {
		return nil, err
	}
	s := &Sharded{
		logger:   logger,
		register: r,
		root:     root,
		segment:  segmentSize,
		comp:     comp,
		wals:     make([]*WL, shards),
	}
	for i := 0; i < shards; i++ {
		dir := ShardDir(root, i)
		if err := os.MkdirAll(dir, 0o777); err != nil {
			return nil, fmt.Errorf("create shard dir %s: %w", dir, err)
		}
		w, err := NewSize(logger, r, dir, segmentSize, comp)
		if err != nil {
			return nil, err
		}
		s.wals[i] = w
	}
	return s, nil
}

func (s *Sharded) ShardCount() int { return len(s.wals) }
func (s *Sharded) Shards() []*WL   { return s.wals }

// WAL returns the *WL for a given shard index.
func (s *Sharded) WAL(i int) *WL { return s.wals[i] }

// Close all shard WALs (best-effort).
func (s *Sharded) Close() error {
	var firstErr error
	for _, w := range s.wals {
		if w == nil {
			continue
		}
		if err := w.Close(); err != nil && firstErr == nil {
			firstErr = err
		}
	}
	return firstErr
}

// TruncateBefore truncates all shards up to (and excluding) segment i in each shard.
// Callers should pass per-shard cutovers if they differ (do a per-shard loop and call WAL(i).Truncate).
func (s *Sharded) TruncateBefore(cutover int) error {
	for _, w := range s.wals {
		if err := w.Truncate(cutover); err != nil {
			return err
		}
	}
	return nil
}

// LastSegments returns the last segment and offset per shard.
func (s *Sharded) LastSegments() (segs []int, offsets []int, _ error) {
	segs = make([]int, len(s.wals))
	offsets = make([]int, len(s.wals))
	for i, w := range s.wals {
		seg, off, err := w.LastSegmentAndOffset()
		if err != nil {
			return nil, nil, err
		}
		segs[i], offsets[i] = seg, off
	}
	return segs, offsets, nil
}

// Root returns the sharded root (e.g. ".../wal").
func (s *Sharded) Root() string { return s.root }
