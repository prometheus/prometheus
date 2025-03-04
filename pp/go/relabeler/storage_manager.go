package relabeler

import (
	"context"
	"errors"
	"fmt"
	"io"
	"math"
	"sync"
	"sync/atomic"

	"github.com/google/uuid"
	"github.com/prometheus/prometheus/pp/go/cppbridge"
	"github.com/prometheus/prometheus/pp/go/frames"
	"github.com/prometheus/prometheus/pp/go/util"
	"github.com/prometheus/client_golang/prometheus"
)

const posNotFound int64 = -1

// StorageManager - manager for file refill. Contains file markup for quick access to data.
type StorageManager struct {
	// markup file
	markupFile *Markup
	// storage for save data
	storage *FileStorage
	// last statuses write, need write frame if have differents
	statuses frames.Statuses
	// last position when writing to
	lastWriteOffset int64
	// state open file
	isOpenFile bool
	// are there any rejects
	hasRejects atomic.Bool
	// stat
	currentSize prometheus.Gauge
	readBytes   prometheus.Histogram
	writeBytes  prometheus.Histogram
}

// NewStorageManager - init new MarkupMap.
//
//revive:disable-next-line:cyclomatic  but readable
//revive:disable-next-line:function-length long but readable
func NewStorageManager(
	cfg FileStorageConfig,
	shardsNumberPower, segmentEncodingVersion uint8,
	blockID uuid.UUID,
	name string,
	registerer prometheus.Registerer,
	names ...string,
) (*StorageManager, error) {
	var err error
	factory := util.NewUnconflictRegisterer(registerer)
	constLabels := prometheus.Labels{"name": name}
	sm := &StorageManager{
		hasRejects: atomic.Bool{},
		currentSize: factory.NewGauge(
			prometheus.GaugeOpts{
				Name:        "prompp_delivery_storage_manager_current_refill_size",
				Help:        "Size of current refill.",
				ConstLabels: constLabels,
			},
		),
		readBytes: factory.NewHistogram(
			prometheus.HistogramOpts{
				Name:        "prompp_delivery_storage_manager_read_bytes",
				Help:        "Number of read bytes.",
				Buckets:     prometheus.ExponentialBucketsRange(1024, 125829120, 10),
				ConstLabels: constLabels,
			},
		),
		writeBytes: factory.NewHistogram(
			prometheus.HistogramOpts{
				Name:        "prompp_delivery_storage_manager_write_bytes",
				Help:        "Number of write bytes.",
				Buckets:     prometheus.ExponentialBucketsRange(1024, 125829120, 10),
				ConstLabels: constLabels,
			},
		),
	}

	// init storage
	sm.storage, err = NewFileStorage(cfg)
	if err != nil {
		return nil, err
	}

	// trying to recover from a storage
	ok, err := sm.restore()
	if err != nil {
		switch {
		case errors.Is(err, ErrServiceDataNotRestored{}):
			if errt := sm.storage.Truncate(0); errt != nil {
				return nil, errt
			}
		case errors.Is(err, frames.ErrUnknownFrameType):
			if errt := sm.storage.Truncate(sm.markupFile.LastOffset()); errt != nil {
				return nil, errt
			}
			ok = true
		case errors.Is(err, &ErrNotContinuableRefill{}):
			if errt := sm.storage.Truncate(sm.markupFile.LastOffset()); errt != nil {
				return nil, errt
			}
			sm.statuses = sm.markupFile.CopyAckStatuses()
			return sm, err
		default:
			return nil, err
		}
	}
	if !ok {
		sm.markupFile = NewMarkup(names, blockID, shardsNumberPower, segmentEncodingVersion)
		sm.setLastWriteOffset(0)
	}

	sm.statuses = sm.markupFile.CopyAckStatuses()

	if sm.markupFile.EncodersVersion() != segmentEncodingVersion {
		return sm, ErrSegmentEncodingVersionNotEqual
	}

	if sm.markupFile.Shards() != 1<<shardsNumberPower {
		return sm, ErrShardsNotEqual
	}

	if !sm.markupFile.EqualDestinationsNames(names) {
		return sm, ErrDestinationsNamesNotEqual
	}

	return sm, nil
}

// BlockID - return if exist blockID or nil.
func (sm *StorageManager) BlockID() uuid.UUID {
	return sm.markupFile.BlockID()
}

// Shards - return number of Shards.
func (sm *StorageManager) Shards() int {
	return sm.markupFile.Shards()
}

// Destinations - return number of Destinations.
func (sm *StorageManager) Destinations() int {
	return sm.markupFile.Destinations()
}

// LastSegment - return last ack segment by shard and destination.
func (sm *StorageManager) LastSegment(shardID uint16, dest string) uint32 {
	return sm.markupFile.Last(shardID, dest)
}

// CheckSegmentsSent - checking if all segments have been sent.
func (sm *StorageManager) CheckSegmentsSent() bool {
	// if there is at least 1 reject, then you cannot delete the refill
	if sm.hasRejects.Load() {
		return false
	}

	maxWriteSegments := sm.markupFile.MaxWriteSegments()
	for d := 0; d < sm.markupFile.Destinations(); d++ {
		for shardID := range maxWriteSegments {
			i := d*len(maxWriteSegments) + shardID
			// check first status
			if sm.statuses[i] == math.MaxUint32 && maxWriteSegments[shardID] != math.MaxUint32 {
				return false
			}

			// check not first status
			if sm.statuses[i] < maxWriteSegments[shardID] {
				return false
			}
		}
	}

	return true
}

// Ack - increment status by destination and shard if segment is next for current value.
func (sm *StorageManager) Ack(key cppbridge.SegmentKey, dest string) {
	sm.markupFile.Ack(key, dest)
}

// Reject - accumulates rejects and serializes and writes to refill while recording statuses.
func (sm *StorageManager) Reject(segKey cppbridge.SegmentKey, dest string) {
	sm.hasRejects.Store(true)
	sm.markupFile.Reject(segKey, dest)
}

// getSegmentPosition - return position in storage.
func (sm *StorageManager) getSegmentPosition(segKey cppbridge.SegmentKey) int64 {
	mk := MarkupKey{
		typeFrame:  frames.SegmentType,
		SegmentKey: segKey,
	}
	mval := sm.markupFile.GetMarkupValue(mk)
	if mval == nil {
		return posNotFound
	}

	return mval.pos
}

// setSegmentPosition - set segment position in storage.
func (sm *StorageManager) setSegmentPosition(segKey cppbridge.SegmentKey, position int64) {
	mk := MarkupKey{
		typeFrame:  frames.SegmentType,
		SegmentKey: segKey,
	}

	sm.markupFile.SetMarkupValue(mk, position, 0)
	sm.markupFile.SetMaxSegments(mk)
}

// GetSegment - return segment from storage.
func (sm *StorageManager) GetSegment(ctx context.Context, segKey cppbridge.SegmentKey) (Segment, error) {
	// get position
	pos := sm.getSegmentPosition(segKey)
	if pos == posNotFound {
		return nil, SegmentNotFoundInRefill(segKey)
	}

	// read frame
	bb, err := frames.ReadFrameSegment(ctx, util.NewOffsetReader(sm.storage, pos))
	if err != nil {
		return nil, err
	}

	sm.readBytes.Observe(float64(bb.GetBody().Size()))

	return bb.GetBody(), nil
}

// GetAckStatus - return last AckStatus.
func (sm *StorageManager) GetAckStatus() *AckStatus {
	return sm.markupFile.AckStatus()
}

// restore - restore MarkupMap from storage.
func (sm *StorageManager) restore() (bool, error) {
	ctx := context.Background()
	// check file
	ok, err := sm.storage.FileExist()
	if err != nil {
		return false, err
	}
	if !ok {
		return false, nil
	}

	// open file
	if err = sm.storage.OpenFile(); err != nil {
		return false, err
	}
	sm.isOpenFile = true

	sm.markupFile, err = NewMarkupReader(sm.storage).ReadFile(ctx)
	if err != nil {
		return false, err
	}

	// seek to end file
	if _, err = sm.storage.Seek(0, io.SeekEnd); err != nil {
		return false, err
	}

	size, err := sm.storage.Size()
	if err != nil {
		return false, err
	}
	sm.setLastWriteOffset(size)
	sm.hasRejects.Store(sm.markupFile.HasRejects())

	return true, nil
}

// writeTitle - write title in storage.
func (sm *StorageManager) writeTitle(ctx context.Context) error {
	fe, err := frames.NewTitleFrameV2(
		sm.markupFile.ShardsNumberPower(),
		sm.markupFile.EncodersVersion(),
		sm.BlockID(),
	)
	if err != nil {
		return err
	}
	n, err := fe.WriteTo(sm.storage.Writer(ctx, sm.lastWriteOffset))
	sm.writeBytes.Observe(float64(n))
	if err != nil {
		return err
	}

	// move position
	sm.moveLastWriteOffset(n)

	return nil
}

// writeDestinationNames - write destination names in storage.
func (sm *StorageManager) writeDestinationNames(ctx context.Context) error {
	fe, err := frames.NewDestinationsNamesFrameWithMsg(protocolVersionSocket, sm.markupFile.DestinationsNames())
	if err != nil {
		return err
	}

	n, err := fe.WriteTo(sm.storage.Writer(ctx, sm.lastWriteOffset))
	sm.writeBytes.Observe(float64(n))
	if err != nil {
		return err
	}

	// move position
	sm.moveLastWriteOffset(n)

	return nil
}

// openNewFile - open new file for new writes and write first frames.
func (sm *StorageManager) openNewFile(ctx context.Context) error {
	// open file
	if err := sm.storage.OpenFile(); err != nil {
		return err
	}

	// write title data
	if err := sm.writeTitle(ctx); err != nil {
		return err
	}

	// write destinations names
	if err := sm.writeDestinationNames(ctx); err != nil {
		return err
	}

	// set flag
	sm.isOpenFile = true

	return nil
}

// WriteSegment - write Segment in storage.
func (sm *StorageManager) WriteSegment(ctx context.Context, key cppbridge.SegmentKey, seg Segment) error {
	if !sm.isOpenFile {
		if err := sm.openNewFile(ctx); err != nil {
			return err
		}
	}

	fe, err := frames.NewWriteFrame(
		protocolVersionSocket,
		frames.ContentVersion2,
		frames.SegmentType,
		frames.NewBinaryWrapper(key.ShardID, key.Segment, seg),
	)
	if err != nil {
		return err
	}
	n, err := fe.WriteTo(sm.storage.Writer(ctx, sm.lastWriteOffset))
	sm.writeBytes.Observe(float64(n))
	if err != nil {
		return err
	}

	sm.setSegmentPosition(key, sm.lastWriteOffset)
	sm.moveLastWriteOffset(n)

	return nil
}

// WriteAckStatus - write AckStatus to storage if there are differences in the manager.
func (sm *StorageManager) WriteAckStatus(ctx context.Context) error {
	// check open file
	if !sm.isOpenFile {
		return nil
	}
	// lock and get copy all statuses
	// TODO: separate acks and rejects
	sm.markupFile.AckStatusLock()
	ss := sm.markupFile.CopyAckStatuses()
	rejects := sm.markupFile.RotateRejects()
	sm.markupFile.AckStatusUnlock()

	if len(rejects) != 0 {
		fe, err := frames.NewRejectStatusesFrame(rejects)
		if err != nil {
			sm.markupFile.UnrotateRejects(rejects)
			return err
		}
		n, err := fe.WriteTo(sm.storage.Writer(ctx, sm.lastWriteOffset))
		sm.writeBytes.Observe(float64(n))
		if err != nil {
			sm.markupFile.UnrotateRejects(rejects)
			return err
		}
		sm.moveLastWriteOffset(n)
	}
	if !sm.statuses.Equal(ss) {
		fe, err := frames.NewStatusesFrame(ss)
		if err != nil {
			// TODO: unrotate
			return err
		}
		n, err := fe.WriteTo(sm.storage.Writer(ctx, sm.lastWriteOffset))
		sm.writeBytes.Observe(float64(n))
		if err != nil {
			// TODO: unrotate
			return err
		}
		sm.moveLastWriteOffset(n)
		sm.statuses = ss
	}
	return nil
}

// moveLastWriteOffset - increase the last offset position by n.
func (sm *StorageManager) moveLastWriteOffset(n int64) {
	sm.lastWriteOffset += n
	sm.currentSize.Set(float64(sm.lastWriteOffset))
}

// setLastWriteOffset - set last offset position to n.
func (sm *StorageManager) setLastWriteOffset(offset int64) {
	sm.lastWriteOffset = offset
	sm.currentSize.Set(float64(sm.lastWriteOffset))
}

// FileExist - check file exist.
func (sm *StorageManager) FileExist() (bool, error) {
	return sm.storage.FileExist()
}

// Rename file
func (sm *StorageManager) Rename(name string) error {
	return sm.storage.Rename(name)
}

// IntermediateRename - rename the current file to blockID with temporary extension for further conversion to refill.
func (sm *StorageManager) IntermediateRename(name string) error {
	return sm.storage.IntermediateRename(name)
}

// GetIntermediateName returns true with file name if file was intermediate renamed
func (sm *StorageManager) GetIntermediateName() (string, bool) {
	return sm.storage.GetIntermediateName()
}

// DeleteCurrentFile - close and delete current file.
func (sm *StorageManager) DeleteCurrentFile() error {
	// reinit, because file deleted
	sm.markupFile.Reset()
	sm.statuses.Reset()
	sm.setLastWriteOffset(0)

	// check open file
	if !sm.isOpenFile {
		return nil
	}

	// set flag and delete
	sm.isOpenFile = false

	return sm.storage.DeleteCurrentFile()
}

// Close - close storage.
func (sm *StorageManager) Close() error {
	// set flag and close
	sm.isOpenFile = false
	err := sm.storage.Close()
	if err != nil {
		return err
	}

	return nil
}

// newShardStatuses - init empty shards status.
func newShardStatuses(shardsNumber int) []uint32 {
	status := make([]uint32, shardsNumber)
	for i := range status {
		status[i] = math.MaxUint32
	}
	return status
}

// Rejects - reject statuses with rotates for concurrency.
type Rejects struct {
	mutex   *sync.Mutex
	active  []frames.Reject
	reserve []frames.Reject
}

// NewRejects - init new Rejects.
func NewRejects() *Rejects {
	return &Rejects{
		mutex:   new(sync.Mutex),
		active:  frames.NewRejectStatusesEmpty(),
		reserve: frames.NewRejectStatusesEmpty(),
	}
}

// Rotate - rotate statuses.
func (rjs *Rejects) Rotate() frames.RejectStatuses {
	rjs.mutex.Lock()
	defer rjs.mutex.Unlock()

	rjs.active, rjs.reserve = rjs.reserve[:0], rjs.active
	return rjs.reserve
}

// Unrotate refill given rejects back.
func (rjs *Rejects) Unrotate(rejects frames.RejectStatuses) {
	rjs.mutex.Lock()
	defer rjs.mutex.Unlock()

	rjs.active, rjs.reserve = append(rejects, rjs.active...), rjs.active[:0]
}

// Add - add reject.
func (rjs *Rejects) Add(nameID, segment uint32, shardID uint16) {
	rjs.mutex.Lock()
	defer rjs.mutex.Unlock()

	rjs.active = append(
		rjs.active,
		frames.Reject{
			NameID:  nameID,
			Segment: segment,
			ShardID: shardID,
		},
	)
}

// AckStatus keeps destinations list with last acked segments numbers per shard
type AckStatus struct {
	names   *frames.DestinationsNames
	status  frames.Statuses
	rejects *Rejects
	rwmx    *sync.RWMutex
}

// NewAckStatus is a constructor
func NewAckStatus(destinations []string, shardsNumberPower uint8) *AckStatus {
	dn := frames.NewDestinationsNames(destinations...)
	shardsNumber := 1 << shardsNumberPower
	status := frames.NewStatusesEmpty(shardsNumberPower, len(destinations))

	dn.Range(func(_ string, id int) bool {
		for i := 0; i < shardsNumber; i++ {
			status[id*shardsNumber+i] = math.MaxUint32
		}
		return true
	})

	return &AckStatus{
		names:   dn,
		status:  status,
		rejects: NewRejects(),
		rwmx:    new(sync.RWMutex),
	}
}

// NewAckStatusEmpty - init empty AckStatus.
func NewAckStatusEmpty(shardsNumberPower uint8) *AckStatus {
	return &AckStatus{
		names:   frames.NewDestinationsNamesEmpty(),
		status:  frames.NewStatusesEmpty(shardsNumberPower, 0),
		rejects: NewRejects(),
		rwmx:    new(sync.RWMutex),
	}
}

// NewAckStatusWithDNames - init AckStatus with DestinationsNames.
func NewAckStatusWithDNames(dnames *frames.DestinationsNames, shardsNumberPower uint8) *AckStatus {
	shardsNumber := 1 << shardsNumberPower
	status := frames.NewStatusesEmpty(shardsNumberPower, dnames.Len())
	dnames.Range(func(_ string, id int) bool {
		for i := 0; i < shardsNumber; i++ {
			status[id*shardsNumber+i] = math.MaxUint32
		}
		return true
	})

	return &AckStatus{
		names:   dnames,
		status:  status,
		rejects: NewRejects(),
		rwmx:    new(sync.RWMutex),
	}
}

// Destinations returns number of destinations
func (as *AckStatus) Destinations() int {
	return as.names.Len()
}

// Shards returns number of shards
func (as *AckStatus) Shards() int {
	return len(as.status) / as.names.Len()
}

// Ack increment status by destination and shard if segment is next for current value
func (as *AckStatus) Ack(key cppbridge.SegmentKey, dest string) {
	id := as.names.StringToID(dest)
	if id == frames.NotFoundName {
		panic(fmt.Sprintf(
			"AckStatus: ack unexpected destination name %s",
			dest,
		))
	}

	as.rwmx.RLock()
	old := atomic.SwapUint32(
		&as.status[id*int32(as.Shards())+int32(key.ShardID)],
		key.Segment,
	)
	as.rwmx.RUnlock()
	if old != math.MaxUint32 && old > key.Segment {
		panic(fmt.Sprintf(
			"AckStatus: ack segment %d less old %d",
			key.Segment,
			old,
		))
	}
}

// Reject - add rejected segment.
func (as *AckStatus) Reject(segKey cppbridge.SegmentKey, dest string) {
	id := as.names.StringToID(dest)
	if id == frames.NotFoundName {
		panic(fmt.Sprintf(
			"AckStatus: ack unexpected destination name %s",
			dest,
		))
	}

	as.rwmx.RLock()
	as.rejects.Add(uint32(id), segKey.Segment, segKey.ShardID)
	as.rwmx.RUnlock()
}

// Index returns destination index by name
func (as *AckStatus) Index(dest string) (int, bool) {
	id := as.names.StringToID(dest)
	if id == frames.NotFoundName {
		return -1, false
	}

	return int(id), true
}

// Last return last ack segment by shard and destination
func (as *AckStatus) Last(shardID uint16, dest string) uint32 {
	id := as.names.StringToID(dest)
	if id == frames.NotFoundName {
		return 0
	}

	return atomic.LoadUint32(&as.status[id*int32(as.Shards())+int32(shardID)])
}

// IsAck returns true if segment ack by all destinations
func (as *AckStatus) IsAck(key cppbridge.SegmentKey) bool {
	for id := 0; id < as.Destinations(); id++ {
		if atomic.LoadUint32(&as.status[id*as.Shards()+int(key.ShardID)])+1 <= key.Segment {
			return false
		}
	}

	return true
}

// GetNames - return DestinationsNames.
func (as *AckStatus) GetNames() *frames.DestinationsNames {
	return as.names
}

// GetCopyAckStatuses - retrun copy statuses.
func (as *AckStatus) GetCopyAckStatuses() frames.Statuses {
	newss := make(frames.Statuses, len(as.status))
	copy(newss, as.status)
	return newss
}

// RotateRejects - rotate rejects statuses.
func (as *AckStatus) RotateRejects() frames.RejectStatuses {
	return as.rejects.Rotate()
}

// UnrotateRejects - unrotate rejects statuses.
func (as *AckStatus) UnrotateRejects(rejects frames.RejectStatuses) {
	as.rejects.Unrotate(rejects)
}

// ReadDestinationsNames - read body frame to DestinationsNames.
func (as *AckStatus) ReadDestinationsNames(ctx context.Context, r io.ReaderAt, off int64, size int) error {
	if err := as.names.ReadAt(ctx, r, off, size); err != nil {
		return err
	}

	as.status = make(frames.Statuses, as.names.Len()*len(as.status))
	for i := range as.status {
		as.status[i] = math.MaxUint32
	}

	return nil
}

// UnmarshalBinaryStatuses - unmarshal statuses from bytes.
func (as *AckStatus) UnmarshalBinaryStatuses(data []byte) error {
	return as.status.UnmarshalBinary(data)
}

// Lock - locks rw for writing.
func (as *AckStatus) Lock() {
	as.rwmx.Lock()
}

// Unlock - unlocks rw for writing.
func (as *AckStatus) Unlock() {
	as.rwmx.Unlock()
}
