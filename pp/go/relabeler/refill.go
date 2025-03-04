package relabeler

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/prometheus/prometheus/pp/go/cppbridge"
	"github.com/prometheus/client_golang/prometheus"
)

const (
	// RefillDir - default refill dir.
	RefillDir = "refill"
	// RefillFileName - default refill current file name.
	RefillFileName = "current"
)

var (
	// ErrShardsNotEqual - error when number of shards not equal.
	ErrShardsNotEqual = errors.New("number of shards not equal")
	// ErrDestinationsNamesNotEqual - error when destinations names not equal.
	ErrDestinationsNamesNotEqual = errors.New("destinations names not equal")
	// ErrSegmentEncodingVersionNotEqual - error when segment encoding version not equal.
	ErrSegmentEncodingVersionNotEqual = errors.New("segment encoding version not equal")
)

// Refill - manager for refills.
type Refill struct {
	sm             *StorageManager
	mx             *sync.RWMutex
	alwaysToRefill bool
	isContinuable  bool
}

var _ ManagerRefill = (*Refill)(nil)

// NewRefill - init new RefillManager.
func NewRefill(
	workingDir string,
	shardsNumberPower, segmentEncodingVersion uint8,
	blockID uuid.UUID,
	alwaysToRefill bool,
	name string,
	registerer prometheus.Registerer,
	names ...string,
) (*Refill, error) {
	var err error
	rm := &Refill{
		mx:             new(sync.RWMutex),
		alwaysToRefill: alwaysToRefill,
	}

	fsCfg := FileStorageConfig{
		Dir:      filepath.Join(workingDir, RefillDir),
		FileName: RefillFileName,
	}

	rm.sm, err = NewStorageManager(
		fsCfg,
		shardsNumberPower,
		segmentEncodingVersion,
		blockID,
		name,
		registerer,
		names...,
	)
	switch {
	case errors.Is(err, nil):
		rm.isContinuable = true
	case errors.Is(err, ErrShardsNotEqual):
		rm.isContinuable = false
	case errors.Is(err, ErrSegmentEncodingVersionNotEqual):
		rm.isContinuable = false
	case errors.Is(err, ErrDestinationsNamesNotEqual):
		rm.isContinuable = false
	case errors.Is(err, &ErrNotContinuableRefill{}):
		rm.isContinuable = false
	default:
		return nil, err
	}

	if !rm.sm.CheckSegmentsSent() {
		rm.isContinuable = false
	}

	return rm, nil
}

// IsContinuable - is it possible to continue the file.
func (rl *Refill) IsContinuable() bool {
	return rl.isContinuable
}

// BlockID - return if exist blockID or nil.
func (rl *Refill) BlockID() uuid.UUID {
	return rl.sm.BlockID()
}

// Shards - return number of Shards.
func (rl *Refill) Shards() int {
	return rl.sm.Shards()
}

// Destinations - return number of Destinations.
func (rl *Refill) Destinations() int {
	return rl.sm.Destinations()
}

// LastSegment - return last ack segment by shard and destination.
func (rl *Refill) LastSegment(shardID uint16, dest string) uint32 {
	return rl.sm.LastSegment(shardID, dest)
}

// Get - get segment from file.
func (rl *Refill) Get(ctx context.Context, key cppbridge.SegmentKey) (Segment, error) {
	rl.mx.RLock()
	defer rl.mx.RUnlock()

	return rl.sm.GetSegment(ctx, key)
}

// Ack - increment status by destination and shard if segment is next for current value.
func (rl *Refill) Ack(segKey cppbridge.SegmentKey, dest string) {
	rl.sm.Ack(segKey, dest)
}

// Reject - accumulates rejects and serializes and writes to refill while recording statuses.
func (rl *Refill) Reject(segKey cppbridge.SegmentKey, dest string) {
	rl.sm.Reject(segKey, dest)
}

// WriteSegment - write Segment in file.
func (rl *Refill) WriteSegment(ctx context.Context, key cppbridge.SegmentKey, seg Segment) error {
	rl.mx.Lock()
	defer rl.mx.Unlock()

	return rl.sm.WriteSegment(ctx, key, seg)
}

// WriteAckStatus - write AckStatus.
func (rl *Refill) WriteAckStatus(ctx context.Context) error {
	rl.mx.Lock()
	defer rl.mx.Unlock()

	if err := rl.sm.WriteAckStatus(ctx); err != nil {
		return err
	}

	if !rl.sm.CheckSegmentsSent() {
		return nil
	}

	if rl.alwaysToRefill {
		return nil
	}

	return rl.sm.DeleteCurrentFile()
}

// IntermediateRename - rename the current file to blockID with temporary extension for further conversion to refill.
//
// Use Rotate nor IntermediateRename + Shutdown. Not both.
func (rl *Refill) IntermediateRename() error {
	rl.mx.Lock()
	defer rl.mx.Unlock()

	return rl.sm.IntermediateRename(rl.makeCompleteFileName())
}

// Shutdown - finalize and close refill.
//
// Use Rotate nor IntermediateRename + Shutdown. Not both.
func (rl *Refill) Shutdown(_ context.Context) error {
	rl.mx.Lock()
	defer rl.mx.Unlock()
	name, ok := rl.sm.GetIntermediateName()
	if !ok {
		name = rl.makeCompleteFileName()
	}

	return errors.Join(rl.sm.Rename(name), rl.sm.Close())
}

func (rl *Refill) makeCompleteFileName() string {
	return fmt.Sprintf("%013d_%s", time.Now().UnixMilli(), rl.sm.BlockID().String())
}
