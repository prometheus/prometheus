package relabeler

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"math"

	"github.com/google/uuid"
	"github.com/prometheus/prometheus/pp/go/cppbridge"
	"github.com/prometheus/prometheus/pp/go/frames"
)

const (
	defaultBufferSize = 1 << 10
)

// MarkupKey - key for search position.
type MarkupKey struct {
	typeFrame frames.TypeFrame
	cppbridge.SegmentKey
}

// MarkupValue - value for markup map.
type MarkupValue struct {
	pos            int64
	size           uint32
	contentVersion uint8
}

// NewMarkupValue - init new *MarkupValue.
func NewMarkupValue(pos int64, size uint32, contentVersion uint8) *MarkupValue {
	return &MarkupValue{pos: pos, size: size, contentVersion: contentVersion}
}

// Position - return position data.
func (mv *MarkupValue) Position() int64 {
	return mv.pos
}

// Size - return size data.
func (mv *MarkupValue) Size() uint32 {
	return mv.size
}

// Markup - file markup.
type Markup struct {
	// title
	title frames.Title
	// last status of writers
	ackStatus *AckStatus
	// marking positions of Segments
	markupMap map[MarkupKey]*MarkupValue
	// restored all rejects
	rejects frames.RejectStatuses
	// max written segment ID for shard
	maxWriteSegments []uint32
	// last frame for all segment send for shard
	destinationsEOF map[string][]bool
	// last successfully read offset
	lastOffset int64
}

// NewMarkup - init new *Markup.
func NewMarkup(names []string, blockID uuid.UUID, shardsNumberPower, segmentEncodingVersion uint8) *Markup {
	return &Markup{
		title:            frames.NewTitleV2(shardsNumberPower, segmentEncodingVersion, blockID),
		ackStatus:        NewAckStatus(names, shardsNumberPower),
		maxWriteSegments: newShardStatuses(1 << shardsNumberPower),
		markupMap:        make(map[MarkupKey]*MarkupValue),
	}
}

// NewMarkupEmpty - init new empty *Markup.
func NewMarkupEmpty() *Markup {
	return &Markup{
		markupMap: make(map[MarkupKey]*MarkupValue),
	}
}

// Ack - increment status by destination and shard if segment is next for current value.
func (m *Markup) Ack(key cppbridge.SegmentKey, dest string) {
	m.ackStatus.Ack(key, dest)
}

// AckStatus - return last AckStatus.
func (m *Markup) AckStatus() *AckStatus {
	return m.ackStatus
}

// AckStatusLock - call mutex lock on ackStatus.
func (m *Markup) AckStatusLock() {
	m.ackStatus.Lock()
}

// AckStatusUnlock - call mutex unlock on ackStatus.
func (m *Markup) AckStatusUnlock() {
	m.ackStatus.Unlock()
}

// CopyAckStatuses - retrun copy statuses.
func (m *Markup) CopyAckStatuses() frames.Statuses {
	return m.ackStatus.GetCopyAckStatuses()
}

// RangeOnDestinationsEOF - calls fn sequentially for each dname, shardID present in the map.
// If fn returns false, range stops the iteration.
func (m *Markup) RangeOnDestinationsEOF(fn func(dname string, shardID int) bool) {
	for d := range m.destinationsEOF {
		for s, eof := range m.destinationsEOF[d] {
			if eof {
				if !fn(d, s) {
					break
				}
			}
		}
	}
}

// RotateRejects - rotate rejects statuses.
func (m *Markup) RotateRejects() frames.RejectStatuses {
	return m.ackStatus.RotateRejects()
}

// UnrotateRejects - unrotate rejects statuses.
func (m *Markup) UnrotateRejects(rejects frames.RejectStatuses) {
	m.ackStatus.UnrotateRejects(rejects)
}

// MaxWriteSegments - return last max segments.
func (m *Markup) MaxWriteSegments() []uint32 {
	return m.maxWriteSegments
}

// Reject  - add rejected segment.
func (m *Markup) Reject(key cppbridge.SegmentKey, dest string) {
	m.ackStatus.Reject(key, dest)
}

// BlockID - return if exist blockID or nil.
func (m *Markup) BlockID() uuid.UUID {
	return m.title.GetBlockID()
}

// DestinationsNames - return DestinationsNames.
func (m *Markup) DestinationsNames() *frames.DestinationsNames {
	return m.ackStatus.GetNames()
}

// Destinations - returns number of destinations.
func (m *Markup) Destinations() int {
	return m.ackStatus.Destinations()
}

// EncodersVersion - return encoders version.
func (m *Markup) EncodersVersion() uint8 {
	return m.title.GetEncodersVersion()
}

// EqualDestinationsNames - equal current DestinationsNames with new.
func (m *Markup) EqualDestinationsNames(names []string) bool {
	return m.ackStatus.names.Equal(names...)
}

// GetMarkupValue - return *MarkupValue.
func (m *Markup) GetMarkupValue(key MarkupKey) *MarkupValue {
	return m.markupMap[key]
}

// HasRejects - check if there are any rejects.
func (m *Markup) HasRejects() bool {
	return m.rejects.Len() != 0
}

// IDToString - search name for id.
func (m *Markup) IDToString(id int32) string {
	return m.ackStatus.names.IDToString(id)
}

// Last - return last ack segment by shard and destination.
func (m *Markup) Last(shardID uint16, dname string) uint32 {
	return m.ackStatus.Last(shardID, dname)
}

// LastOffset - return the last successfully read offset.
func (m *Markup) LastOffset() int64 {
	return m.lastOffset
}

// Reset - reset current file markup.
func (m *Markup) Reset() {
	m.maxWriteSegments = newShardStatuses(1 << m.title.GetShardsNumberPower())
	m.markupMap = make(map[MarkupKey]*MarkupValue)
}

// SetLastFrame - set last frame.
func (m *Markup) SetLastFrame(dname string, shardID uint16) {
	m.destinationsEOF[dname][shardID] = true
}

// SetMarkupValue - set to map *MarkupValue on MarkupKey.
func (m *Markup) SetMarkupValue(key MarkupKey, position int64, size uint32) {
	m.markupMap[key] = &MarkupValue{pos: position, size: size}
}

// SetMaxSegments - set last max segmets.
func (m *Markup) SetMaxSegments(key MarkupKey) {
	if m.maxWriteSegments[key.ShardID] == math.MaxUint32 || key.Segment > m.maxWriteSegments[key.ShardID] {
		m.maxWriteSegments[key.ShardID] = key.Segment
	}
}

// Shards - returns number of shards.
func (m *Markup) Shards() int {
	return m.ackStatus.Shards()
}

// ShardsNumberPower - return shards of number power.
func (m *Markup) ShardsNumberPower() uint8 {
	return m.title.GetShardsNumberPower()
}

// StringToID - search id for name.
func (m *Markup) StringToID(dname string) int32 {
	return m.ackStatus.names.StringToID(dname)
}

// MarkupReader - reader for compiling file markup.
type MarkupReader struct {
	r *bufio.Reader
	m *Markup
}

// NewMarkupReader - init new *MarkupReader.
func NewMarkupReader(r io.Reader) *MarkupReader {
	return &MarkupReader{
		r: bufio.NewReaderSize(r, defaultBufferSize),
		m: NewMarkupEmpty(),
	}
}

// ReadFile - read file and create markup.
func (mr *MarkupReader) ReadFile(ctx context.Context) (*Markup, error) {
	err := mr.readFull(ctx)
	if !mr.checkRestoredServiceData() {
		return nil, ErrServiceDataNotRestored{}
	}

	return mr.m, err
}

// readFull - read full file.
func (mr *MarkupReader) readFull(ctx context.Context) error {
	for {
		// read header frame
		h, err := frames.ReadHeader(ctx, mr.r)
		switch {
		case errors.Is(err, nil):
		case errors.Is(err, io.EOF):
			return nil
		case errors.Is(err, io.ErrUnexpectedEOF):
			return NotContinuableRefill(err)
		default:
			return err
		}

		if err = mr.readFromBody(ctx, h); err != nil {
			if errors.Is(err, io.EOF) || errors.Is(err, io.ErrUnexpectedEOF) {
				return NotContinuableRefill(err)
			}
			return err
		}
		mr.m.lastOffset += int64(h.FrameSize())
	}
}

// checkRestoredServiceData - check restored service data(title, destinations names),
// these data are required to be restored, without them you cant read the rest
func (mr *MarkupReader) checkRestoredServiceData() bool {
	return mr.m.title != nil && mr.m.ackStatus != nil
}

// restoreFromBody - restore from body frame.
func (mr *MarkupReader) readFromBody(ctx context.Context, h *frames.Header) error {
	// TODO restore bad frame
	switch h.GetType() {
	case frames.TitleType:
		return mr.readTitle(ctx, h)
	case frames.DestinationNamesType:
		return mr.readDestinationsNames(ctx, h)
	case frames.SegmentType:
		return mr.setSegmentPosition(ctx, h)
	case frames.StatusType:
		return mr.readStatuses(ctx, h)
	case frames.RejectStatusType:
		return mr.readRejectStatuses(ctx, h)
	case frames.RefillShardEOFType:
		return mr.readRefillShardEOF(ctx, h)
	default:
		return fmt.Errorf("%w: %d", frames.ErrUnknownFrameType, h.GetType())
	}
}

// readTitle - read title from file.
func (mr *MarkupReader) readTitle(ctx context.Context, h *frames.Header) error {
	var err error
	mr.m.title, err = frames.ReadTitle(ctx, mr.r, int(h.GetSize()), h.GetContentVersion())
	if err != nil {
		return err
	}

	// init maxWriteSegments for future reference and not to panic
	mr.m.maxWriteSegments = make([]uint32, 1<<mr.m.title.GetShardsNumberPower())
	for i := range mr.m.maxWriteSegments {
		mr.m.maxWriteSegments[i] = math.MaxUint32
	}

	return nil
}

// readDestinationsNames - read Destinations Names from file.
func (mr *MarkupReader) readDestinationsNames(ctx context.Context, h *frames.Header) error {
	dnames, err := frames.ReadDestinationsNames(ctx, mr.r, int(h.GetSize()))
	if err != nil {
		return err
	}
	mr.m.ackStatus = NewAckStatusWithDNames(dnames, mr.m.ShardsNumberPower())
	mr.makeDestinationsEOF()
	return nil
}

// makeDestinationsEOF - make DestinationsEOF.
func (mr *MarkupReader) makeDestinationsEOF() {
	shards := mr.m.Shards()
	dnames := mr.m.DestinationsNames()
	mr.m.destinationsEOF = make(map[string][]bool, dnames.Len())
	for _, dname := range dnames.ToString() {
		mr.m.destinationsEOF[dname] = make([]bool, shards)
	}
}

// setSegmentPosition - set Segment position.
func (mr *MarkupReader) setSegmentPosition(ctx context.Context, h *frames.Header) error {
	if !mr.checkRestoredServiceData() {
		return ErrServiceDataNotRestored{}
	}

	si := frames.NewSegmentInfoEmpty()
	n, err := si.ReadSegmentInfo(ctx, mr.r, h)
	if err != nil {
		return err
	}

	mk := MarkupKey{
		typeFrame: frames.SegmentType,
		SegmentKey: cppbridge.SegmentKey{
			ShardID: si.GetShardID(),
			Segment: si.GetSegmentID(),
		},
	}
	mr.m.markupMap[mk] = NewMarkupValue(mr.m.lastOffset, h.GetSize(), h.GetContentVersion())

	if mr.m.maxWriteSegments[mk.ShardID] == math.MaxUint32 ||
		mk.Segment > mr.m.maxWriteSegments[mk.ShardID] {
		mr.m.maxWriteSegments[mk.ShardID] = mk.Segment
	}

	if _, err := mr.r.Discard(int(h.GetSize()) - n); err != nil {
		return err
	}

	return nil
}

// readStatuses - read Statuses from file.
func (mr *MarkupReader) readStatuses(ctx context.Context, h *frames.Header) error {
	if !mr.checkRestoredServiceData() {
		return ErrServiceDataNotRestored{}
	}

	if ctx.Err() != nil {
		return ctx.Err()
	}
	buf := make([]byte, h.GetSize())
	if _, err := io.ReadFull(mr.r, buf); err != nil {
		return err
	}

	return mr.m.ackStatus.UnmarshalBinaryStatuses(buf)
}

// readRejectStatuses - read reject statues from file.
func (mr *MarkupReader) readRejectStatuses(ctx context.Context, h *frames.Header) error {
	if !mr.checkRestoredServiceData() {
		return ErrServiceDataNotRestored{}
	}

	if ctx.Err() != nil {
		return ctx.Err()
	}
	buf := make([]byte, h.GetSize())
	if _, err := io.ReadFull(mr.r, buf); err != nil {
		return err
	}

	var rs frames.RejectStatuses
	if err := rs.UnmarshalBinary(buf); err != nil {
		return err
	}

	mr.m.rejects = append(mr.m.rejects, rs...)

	return nil
}

// readRefillShardEOF - read refill shard EOF from file.
func (mr *MarkupReader) readRefillShardEOF(ctx context.Context, h *frames.Header) error {
	if !mr.checkRestoredServiceData() {
		return ErrServiceDataNotRestored{}
	}

	if ctx.Err() != nil {
		return ctx.Err()
	}
	buf := make([]byte, h.GetSize())
	if _, err := io.ReadFull(mr.r, buf); err != nil {
		return err
	}

	rs := frames.NewRefillShardEOFEmpty()
	if err := rs.UnmarshalBinary(buf); err != nil {
		return err
	}

	dname := mr.m.IDToString(int32(rs.NameID))
	if dname == "" {
		return nil
	}

	mr.m.destinationsEOF[dname][rs.ShardID] = true

	return nil
}

// ErrNotContinuableRefill - error refill contains not continuable data.
type ErrNotContinuableRefill struct {
	err error
}

// NotContinuableRefill - create ErrNotContinuableRefill error.
func NotContinuableRefill(err error) error {
	if err == nil {
		return nil
	}
	return &ErrNotContinuableRefill{err}
}

// Error - implements error.
func (err *ErrNotContinuableRefill) Error() string {
	return fmt.Errorf("refill contains not continuable data: %w", err.err).Error()
}

// Unwrap - implements errors.Unwrapper interface.
func (err *ErrNotContinuableRefill) Unwrap() error {
	return err.err
}

// Is - implements errors.Is interface.
func (*ErrNotContinuableRefill) Is(target error) bool {
	_, ok := target.(*ErrNotContinuableRefill)
	return ok
}
