package relabeler

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"math"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/google/uuid"
	"github.com/jonboulle/clockwork"
	"github.com/prometheus/prometheus/pp/go/cppbridge"
	"github.com/prometheus/prometheus/pp/go/frames"
	"github.com/prometheus/prometheus/pp/go/util"
	"github.com/prometheus/client_golang/prometheus"
)

const (
	// DefaultScanInterval - default scan refill file interval.
	DefaultScanInterval = 300 * time.Second
	// DefaultMaxRefillSize - default max refill files size in bytes.
	DefaultMaxRefillSize = 300 << 20 // 300mb
)

// RefillSendManagerConfig - config for RefillSendManagerConfig.
//
// ScanInterval - refill file scan interval.
// MaxRefillSize - max refill files size in bytes.
type RefillSendManagerConfig struct {
	ScanInterval  time.Duration `yaml:"scan_interval" validate:"required"`
	MaxRefillSize int64         `yaml:"max_refill_size" validate:"required"`
}

// DefaultRefillSendManagerConfig - generate default RefillSendManagerConfig.
func DefaultRefillSendManagerConfig() RefillSendManagerConfig {
	return RefillSendManagerConfig{
		ScanInterval:  DefaultScanInterval,
		MaxRefillSize: DefaultMaxRefillSize,
	}
}

// refillSendManagerUpdate data for update changed attributes.
type refillSendManagerUpdate struct {
	workingDir string
	rsmCfg     *RefillSendManagerConfig
	dialers    []Dialer
}

// RefillSendManager - manager  for send refill to server.
type RefillSendManager struct {
	rsmCfg             RefillSendManagerConfig
	dir                string
	name               string
	dialers            map[string]Dialer
	unavailableDialers map[string]struct{}
	unavailableMX      *sync.Mutex
	clock              clockwork.Clock
	stop               chan struct{}
	done               chan struct{}
	update             chan *refillSendManagerUpdate
	// stat
	registerer      prometheus.Registerer
	fileSize        prometheus.Gauge
	numberFiles     prometheus.Gauge
	deletedFileSize *prometheus.HistogramVec
	errors          *prometheus.CounterVec
}

// NewRefillSendManager - init new RefillSendManger.
func NewRefillSendManager(
	rsmCfg RefillSendManagerConfig,
	workingDir string,
	dialers []Dialer,
	clock clockwork.Clock,
	name string,
	registerer prometheus.Registerer,
) (*RefillSendManager, error) {
	if len(dialers) == 0 {
		return nil, ErrDestinationsRequired
	}
	dialersMap := make(map[string]Dialer, len(dialers))
	for _, dialer := range dialers {
		dialersMap[dialer.String()] = dialer
	}
	factory := util.NewUnconflictRegisterer(registerer)
	constLabels := prometheus.Labels{"name": name}
	return &RefillSendManager{
		rsmCfg:             rsmCfg,
		dir:                filepath.Join(workingDir, RefillDir),
		name:               name,
		dialers:            dialersMap,
		unavailableDialers: make(map[string]struct{}, len(dialers)),
		unavailableMX:      new(sync.Mutex),
		clock:              clock,
		stop:               make(chan struct{}),
		done:               make(chan struct{}),
		update:             make(chan *refillSendManagerUpdate, 1),
		registerer:         registerer,
		fileSize: factory.NewGauge(
			prometheus.GaugeOpts{
				Name:        "prompp_delivery_refill_send_manager_file_bytes",
				Help:        "Total files size of bytes.",
				ConstLabels: constLabels,
			},
		),
		numberFiles: factory.NewGauge(
			prometheus.GaugeOpts{
				Name:        "prompp_delivery_refill_send_manager_files_count",
				Help:        "Total number of files.",
				ConstLabels: constLabels,
			},
		),
		deletedFileSize: factory.NewHistogramVec(
			prometheus.HistogramOpts{
				Name:        "prompp_delivery_refill_send_manager_deleted_file_bytes",
				Help:        "Deleted file sizes of bytes.",
				Buckets:     prometheus.ExponentialBucketsRange(50<<20, 1<<30, 10),
				ConstLabels: constLabels,
			},
			[]string{"cause"},
		),
		errors: factory.NewCounterVec(
			prometheus.CounterOpts{
				Name:        "prompp_delivery_refill_send_manager_errors",
				Help:        "Total number errors.",
				ConstLabels: constLabels,
			},
			[]string{"place"},
		),
	}, nil
}

// Run - main loop for scan refill and sending to destinations.
func (rsm *RefillSendManager) Run(ctx context.Context) {
	if err := rsm.checkTmpRefill(); err != nil {
		Errorf("fail check and rename tmp refill file: %s", err)
	}

	loopTicker := rsm.clock.NewTicker(rsm.rsmCfg.ScanInterval)
	defer loopTicker.Stop()
	defer close(rsm.done)
	for {
		select {
		case <-loopTicker.Chan():
			// scan the folder for files to send and process these files
			if err := rsm.processing(ctx); err != nil {
				if errors.Is(err, ErrShutdown) {
					return
				}
				rsm.errors.With(prometheus.Labels{"place": "processing"}).Inc()
				Errorf("fail scan and send loop: %s", err)
				continue
			}

			// delete old files if the size exceeds the maximum
			if err := rsm.clearing(); err != nil {
				rsm.errors.With(prometheus.Labels{"place": "clearing"}).Inc()
				Errorf("fail clearing: %s", err)
				continue
			}
		case rsmu := <-rsm.update:
			if len(rsmu.dialers) != 0 {
				dialersMap := make(map[string]Dialer, len(rsmu.dialers))
				for _, dialer := range rsmu.dialers {
					dialersMap[dialer.String()] = dialer
				}
				rsm.dialers = dialersMap
			}
			if rsm.rsmCfg != *rsmu.rsmCfg {
				rsm.rsmCfg = *rsmu.rsmCfg
			}

			if rsm.dir != rsmu.workingDir {
				rsm.dir = rsmu.workingDir
			}

		case <-rsm.stop:
			return
		case <-ctx.Done():
			if !errors.Is(context.Cause(ctx), ErrShutdown) {
				Errorf("scan and send loop context canceled: %s", context.Cause(ctx))
			}
			return
		}
	}
}

// ResetTo reset to changed attributes.
func (rsm *RefillSendManager) ResetTo(
	workingDir string,
	dialers []Dialer,
	rsmCfg *RefillSendManagerConfig,
) {
	rsm.update <- &refillSendManagerUpdate{
		workingDir: workingDir,
		dialers:    dialers,
		rsmCfg:     rsmCfg,
	}
}

// checkTmpRefill - checks refile files with temporary extensions and renames them to stateful.
func (rsm *RefillSendManager) checkTmpRefill() error {
	files, err := os.ReadDir(rsm.dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}

	for _, file := range files {
		if file.IsDir() || !strings.HasSuffix(file.Name(), refillIntermediateFileExtension) {
			continue
		}

		oldName := file.Name()
		newName := oldName[:len(oldName)-len(refillIntermediateFileExtension)] + refillFileExtension
		if err := os.Rename(
			filepath.Join(rsm.dir, oldName),
			filepath.Join(rsm.dir, newName),
		); err != nil {
			return err
		}
	}

	return nil
}

// processing - scan the folder for files to send and process these files.
func (rsm *RefillSendManager) processing(ctx context.Context) error {
	refillFiles, err := rsm.scanFolder()
	if err != nil {
		return fmt.Errorf("fail scan folder: %s: %w", rsm.dir, err)
	}

	for _, fileInfo := range refillFiles {
		select {
		case <-rsm.stop:
			return ErrShutdown
		case <-ctx.Done():
			return context.Cause(ctx)
		default:
		}
		// read, prepare to send, send
		if err = rsm.fileProcessing(ctx, fileInfo); err != nil {
			Errorf("fail send file '%s': %s", fileInfo.Name(), err)
			if IsPermanent(err) {
				// delete bad refill file
				_ = os.Remove(filepath.Join(rsm.dir, fileInfo.Name()))
				rsm.deletedFileSize.With(prometheus.Labels{"cause": "error"}).Observe(float64(fileInfo.Size()))
			}
		}
	}

	rsm.clearUnavailableDialers()

	return nil
}

// scanFolder - check folder for refill files.
func (rsm *RefillSendManager) scanFolder() ([]fs.FileInfo, error) {
	files, err := os.ReadDir(rsm.dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}

	isCompletedRefill := func(entry os.DirEntry) bool {
		return !entry.IsDir() &&
			strings.HasSuffix(entry.Name(), refillFileExtension) &&
			!strings.HasPrefix(entry.Name(), RefillFileName)
	}

	var fullFileSize int64
	refillFiles := make([]fs.FileInfo, 0, len(files))
	for _, file := range files {
		if isCompletedRefill(file) {
			fileInfo, err := file.Info()
			if err != nil {
				return nil, err
			}
			fullFileSize += fileInfo.Size()
			refillFiles = append(refillFiles, fileInfo)
		}
	}
	rsm.fileSize.Set(float64(fullFileSize))
	rsm.numberFiles.Set(float64(len(refillFiles)))
	return refillFiles, nil
}

// processingFile - read and preparing data and sending to destinations.
//
//revive:disable:cognitive-complexity // because there is nowhere to go
func (rsm *RefillSendManager) fileProcessing(ctx context.Context, fileInfo fs.FileInfo) error {
	reader, err := NewRefillReader(
		ctx,
		FileStorageConfig{
			Dir:      rsm.dir,
			FileName: strings.TrimSuffix(fileInfo.Name(), refillFileExtension),
		},
	)
	if err != nil {
		return err
	}
	defer reader.Close()
	// indicator that all goroutines completed successfully
	withError := new(atomic.Bool)
	wg := new(sync.WaitGroup)
	// grouping data by destinations and start send with goroutines
	groupingByDestinations := reader.MakeSendMap()
	groupingByDestinations.Range(func(dname string, shardID int, shardData []uint32) bool {
		dialer, ok := rsm.dialers[dname]
		if !ok {
			// if the dialer is not found, then we skip the data
			return true
		}
		if rsm.isUnavailableDialer(dname) {
			withError.Store(true)
			Errorf("%s: fail send: %s", dname, ErrHostIsUnavailable)
			return true
		}
		wg.Add(1)
		go func(dr Dialer, dn string, id int, data []uint32) {
			defer wg.Done()
			if err := rsm.send(ctx, dr, reader, uint16(id), data); err != nil {
				if !IsPermanent(err) {
					rsm.addUnavailableDialer(dn)
					withError.Store(true)
				}
				Errorf("%s: fail send: %s", dname, err)
				return
			}
		}(dialer, dname, shardID, shardData)
		return true
	})
	wg.Wait()
	// if any of the deliveries failed, file should be saved for next attempt
	if withError.Load() {
		return nil
	}
	rsm.deletedFileSize.With(prometheus.Labels{"cause": "delivered"}).Observe(float64(fileInfo.Size()))
	// if file has been delivered to all known destinations, it doesn't required anymore and should be deleted
	return reader.DeleteFile()
}

// isUnavailableDialer - if the dialer was unavailable
// and the allowed repeat time was not reached, then we skip the data.
func (rsm *RefillSendManager) isUnavailableDialer(dname string) bool {
	rsm.unavailableMX.Lock()
	_, ok := rsm.unavailableDialers[dname]
	rsm.unavailableMX.Unlock()
	return ok
}

// isUnavailableDialer - if the dialer was unavailable add to map.
func (rsm *RefillSendManager) addUnavailableDialer(dname string) {
	rsm.unavailableMX.Lock()
	rsm.unavailableDialers[dname] = struct{}{}
	rsm.unavailableMX.Unlock()
}

// clearUnavailableDialer - clear unavailable dialers map.
func (rsm *RefillSendManager) clearUnavailableDialers() {
	rsm.unavailableMX.Lock()
	for dname := range rsm.unavailableDialers {
		delete(rsm.unavailableDialers, dname)
	}
	rsm.unavailableMX.Unlock()
}

// send - sending to destinations.
func (rsm *RefillSendManager) send(
	ctx context.Context,
	dialer Dialer,
	source *RefillReader,
	shardID uint16,
	data []uint32,
) error {
	rs := NewRefillSender(
		dialer,
		source,
		shardID,
		data,
		rsm.name,
		rsm.registerer,
	)

	return rs.Send(ctx)
}

// clearing - delete old refills if the maximum allowable size is exceeded.
func (rsm *RefillSendManager) clearing() error {
	refillFiles, err := rsm.scanFolder()
	if err != nil {
		return err
	}

	if len(refillFiles) == 0 {
		return nil
	}

	var fullFileSize int64
	for _, fileInfo := range refillFiles {
		fullFileSize += fileInfo.Size()
	}

	for len(refillFiles) > 0 && fullFileSize > rsm.rsmCfg.MaxRefillSize {
		Errorf("'%s': remove file: %s", refillFiles[0].Name(), errRefillLimitExceeded)
		rsm.deletedFileSize.With(prometheus.Labels{"cause": "limit"}).Observe(float64(refillFiles[0].Size()))
		if err = os.Remove(filepath.Join(rsm.dir, refillFiles[0].Name())); err != nil {
			Errorf("'%s': failed to delete file: %s", refillFiles[0].Name(), err)
			refillFiles = refillFiles[1:]
			continue
		}
		fullFileSize -= refillFiles[0].Size()
		refillFiles = refillFiles[1:]
	}
	rsm.numberFiles.Set(float64(len(refillFiles)))
	rsm.fileSize.Set(float64(fullFileSize))
	return nil
}

// Shutdown - await while ScanAndSend stop send.
func (rsm *RefillSendManager) Shutdown(ctx context.Context) error {
	close(rsm.stop)

	select {
	case <-ctx.Done():
		return context.Cause(ctx)
	case <-rsm.done:
		return nil
	}
}

// PreparedData - prepared data for send.
type PreparedData struct {
	Value     *MarkupValue
	SegmentID uint32
	MsgType   frames.TypeFrame
}

// RefillSender - sender refill to server.
type RefillSender struct {
	dialer          Dialer
	source          *RefillReader
	dataToSend      []uint32
	lastSendSegment uint32
	shardMeta       ShardMeta
	// stat
	successfulDelivery prometheus.Counter
	needSend           *prometheus.HistogramVec
}

// NewRefillSender - init new RefillSender.
func NewRefillSender(
	dialer Dialer,
	source *RefillReader,
	shardID uint16,
	data []uint32,
	name string,
	registerer prometheus.Registerer,
) *RefillSender {
	factory := util.NewUnconflictRegisterer(registerer)
	return &RefillSender{
		dialer:          dialer,
		source:          source,
		dataToSend:      data,
		lastSendSegment: math.MaxUint32,
		shardMeta: ShardMeta{
			BlockID:                source.BlockID(),
			ShardID:                shardID,
			ShardsLog:              source.ShardsNumberPower(),
			SegmentEncodingVersion: source.EncodersVersion(),
		},
		successfulDelivery: factory.NewCounter(
			prometheus.CounterOpts{
				Name:        "prompp_delivery_refill_sender_successful_delivery",
				Help:        "Total successful delivery.",
				ConstLabels: prometheus.Labels{"host": dialer.String(), "name": name},
			},
		),
		needSend: factory.NewHistogramVec(
			prometheus.HistogramOpts{
				Name:        "prompp_delivery_refill_sender_need_send_bytes",
				Help:        "Amount of data to send.",
				ConstLabels: prometheus.Labels{"host": dialer.String(), "name": name},
				Buckets:     prometheus.ExponentialBucketsRange(1024, 120<<20, 10),
			},
			[]string{"type"},
		),
	}
}

// String - implements fmt.Stringer interface.
func (rs *RefillSender) String() string {
	return rs.dialer.String()
}

// Send - create a connection, prepare data for sending and send.
func (rs *RefillSender) Send(ctx context.Context) error {
	pData, contentLength, err := rs.collectedData()
	if err != nil {
		return fmt.Errorf("%s: fail prepared data: %w", rs, err)
	}

	if err = rs.dialer.SendRefill(
		ctx,
		NewSegmentReader(ctx, rs.source, pData, rs.shardMeta.ShardID),
		rs.shardMeta.WithContentLength(contentLength),
	); err != nil {
		return fmt.Errorf("%s: fail send refill data: %w", rs, err)
	}

	if errWrite := rs.source.WriteRefillShardEOF(ctx, rs.String(), rs.shardMeta.ShardID); errWrite != nil {
		Errorf("'%s': fail to write shard EOF: %s", rs, errWrite)
	}

	return nil
}

// collectedData - collects all the necessary position data for sending.
//
//revive:disable-next-line:cyclomatic  but readable
func (rs *RefillSender) collectedData() ([]PreparedData, int64, error) {
	pData := make([]PreparedData, 0, len(rs.dataToSend))

	var contentLength int64
	for _, segment := range rs.dataToSend {
		key := cppbridge.SegmentKey{ShardID: rs.shardMeta.ShardID, Segment: segment}
		mval := rs.source.SegmentPosition(key)
		if mval == nil {
			return nil, 0, SegmentNotFoundInRefill(key)
		}

		pData = append(
			pData,
			PreparedData{
				MsgType:   frames.SegmentType,
				SegmentID: segment,
				Value:     mval,
			},
		)
		segmentSize := mval.size
		contentLength += int64(segmentSize + uint32(frames.RefillSegmentSizeV4))
		if mval.contentVersion == frames.ContentVersion2 {
			contentLength -= int64(frames.SegmentInfoSize)
		}
		rs.needSend.With(prometheus.Labels{"type": "put"}).Observe(float64(segmentSize))
	}

	return pData, contentLength, nil
}

// SegmentReader - wrapper with implements io.Reader.
type SegmentReader struct {
	ctx     context.Context
	source  *RefillReader
	buf     *bytes.Buffer
	data    []PreparedData
	i       int64
	shardID uint16
}

// NewSegmentReader - init new SegmentReader.
func NewSegmentReader(ctx context.Context, source *RefillReader, data []PreparedData, shardID uint16) *SegmentReader {
	return &SegmentReader{
		ctx:     ctx,
		source:  source,
		buf:     new(bytes.Buffer),
		data:    data,
		i:       0,
		shardID: shardID,
	}
}

// Read - implements io.Reader.
func (r *SegmentReader) Read(p []byte) (int, error) {
	if r.i >= int64(len(r.data)) && r.buf.Len() == 0 {
		return 0, io.EOF
	}

	if r.buf.Len() == 0 || r.buf.Len() < len(p) {
		if err := r.fillBuffer(len(p)); err != nil {
			return 0, err
		}
	}

	return r.buf.Read(p)
}

// fillBuffer - auto fill buffer for read.
func (r *SegmentReader) fillBuffer(size int) error {
	for size > r.buf.Len() {
		if r.i >= int64(len(r.data)) {
			return nil
		}
		data := r.data[r.i]
		if data.MsgType != frames.SegmentType {
			r.i++
			continue
		}
		bb, err := r.source.Segment(r.ctx, data.Value)
		if err != nil {
			return fmt.Errorf("failed get segment: %w", err)
		}

		if _, err = frames.NewWriteRefillSegmentV4(bb.GetSegmentID(), bb.GetBody()).WriteTo(r.buf); err != nil {
			return fmt.Errorf("failed write segment: %w", err)
		}
		r.i++
	}
	return nil
}

// SendMap - map for send grouping by destinations.
type SendMap struct {
	m      map[string][][]uint32
	shards int
}

// NewSendMap - init new SendMap.
func NewSendMap(shards int) *SendMap {
	return &SendMap{
		m:      make(map[string][][]uint32),
		shards: shards,
	}
}

// Append - append to map segmentID by shardID for dname.
func (sm *SendMap) Append(dname string, shardID uint16, segmentID uint32) {
	if _, ok := sm.m[dname]; !ok {
		sm.m[dname] = make([][]uint32, sm.shards)
	}
	list := sm.m[dname][shardID]
	if n := len(list); n != 0 {
		if list[n-1] == segmentID {
			return
		}
		if list[n-1] > segmentID {
			// It is possible that AckStatusFrame is corrupted or lost.
			// It is exactly reason for this condition is true.
			// We choose send all segments with unknown state (their ack status is lost).
			// Rejected segments should be added too as unacked.
			list = list[:sort.Search(len(list), func(i int) bool { return list[i] >= segmentID })]
		}
	}

	sm.m[dname][shardID] = append(list, segmentID)
}

// Remove destination-shard data
func (sm *SendMap) Remove(dname string, shardID uint16) {
	sm.m[dname][shardID] = nil
}

// Range - calls f sequentially for each dname, shardID and shardData present in the map.
// If f returns false, range stops the iteration.
func (sm *SendMap) Range(fn func(dname string, shardID int, shardData []uint32) bool) {
	for d := range sm.m {
		for s := range sm.m[d] {
			if len(sm.m[d][s]) == 0 {
				continue
			}
			if !fn(d, s, sm.m[d][s]) {
				break
			}
		}
	}
}

// RefillReader - reader for refill files.
type RefillReader struct {
	// markup file
	markupFile *Markup
	// reader/writer for restore file
	storage *FileStorage
	// mutex for parallel writing
	mx *sync.RWMutex
	// state open file
	isOpenFile bool
	// last position when writing to
	lastWriteOffset int64
}

// NewRefillReader - init new RefillReader.
func NewRefillReader(ctx context.Context, cfg FileStorageConfig) (*RefillReader, error) {
	var err error
	rr := &RefillReader{
		mx: new(sync.RWMutex),
	}

	rr.storage, err = NewFileStorage(cfg)
	if err != nil {
		return nil, err
	}

	if err = rr.openFile(); err != nil {
		return nil, err
	}

	rr.markupFile, err = NewMarkupReader(rr.storage).ReadFile(ctx)
	if err != nil {
		switch {
		case errors.Is(err, frames.ErrUnknownFrameType):
			if err = rr.storage.Truncate(rr.markupFile.LastOffset()); err != nil {
				return nil, err
			}
		case errors.Is(err, &ErrNotContinuableRefill{}):
			if err = rr.storage.Truncate(rr.markupFile.LastOffset()); err != nil {
				return nil, err
			}
		default:
			return nil, err
		}
	}
	rr.lastWriteOffset = rr.markupFile.LastOffset()

	return rr, nil
}

// String - implements fmt.Stringer interface.
func (rr *RefillReader) String() string {
	return rr.storage.GetPath()
}

// BlockID - return if exist blockID or nil.
func (rr *RefillReader) BlockID() uuid.UUID {
	return rr.markupFile.BlockID()
}

// ShardsNumberPower - return shards of number power.
func (rr *RefillReader) ShardsNumberPower() uint8 {
	return rr.markupFile.ShardsNumberPower()
}

// EncodersVersion - return encoders version.
func (rr *RefillReader) EncodersVersion() uint8 {
	return rr.markupFile.EncodersVersion()
}

// MakeSendMap - distribute refill by destinations.
func (rr *RefillReader) MakeSendMap() *SendMap {
	// grouping by destinations
	sm := NewSendMap(rr.markupFile.Shards())

	// distribute rejects segment.
	rr.distributeRejects(sm)

	// distribute not ack segment
	rr.distributeNotAck(sm)

	// clearing data to sent
	rr.clearingToSent(sm)

	return sm
}

// distributeRejects - distribute rejects segment.
func (rr *RefillReader) distributeRejects(sm *SendMap) {
	dnames := rr.markupFile.DestinationsNames()
	for _, rj := range rr.markupFile.rejects {
		sm.Append(
			dnames.IDToString(int32(rj.NameID)),
			rj.ShardID,
			rj.Segment,
		)
	}
}

// distributeNotAck - distribute not ack segment.
func (rr *RefillReader) distributeNotAck(sm *SendMap) {
	shards := uint16(rr.markupFile.Shards())

	for _, dname := range rr.markupFile.DestinationsNames().ToString() {
		for shardID := uint16(0); shardID < shards; shardID++ {
			// we need to check if at least some segments have been recorded
			if n := rr.markupFile.maxWriteSegments[shardID]; n != math.MaxUint32 {
				for sid := rr.markupFile.Last(shardID, dname) + 1; sid <= n; sid++ {
					sm.Append(dname, shardID, sid)
				}
			}
		}
	}
}

func (rr *RefillReader) clearingToSent(sm *SendMap) {
	for d := range rr.markupFile.destinationsEOF {
		for s, eof := range rr.markupFile.destinationsEOF[d] {
			if eof {
				sm.Remove(d, uint16(s))
			}
		}
	}
}

// openFile - open file refill for read/write.
func (rr *RefillReader) openFile() error {
	if rr.isOpenFile {
		return nil
	}
	// open file
	if err := rr.storage.OpenFile(); err != nil {
		return err
	}
	rr.isOpenFile = true

	return nil
}

// Segment - return segment from storage.
func (rr *RefillReader) Segment(ctx context.Context, mval *MarkupValue) (frames.BinaryBody, error) {
	rr.mx.RLock()
	defer rr.mx.RUnlock()

	return frames.ReadFrameSegment(ctx, util.NewOffsetReader(rr.storage, mval.pos))
}

// SegmentPosition - return position in storage.
func (rr *RefillReader) SegmentPosition(segKey cppbridge.SegmentKey) *MarkupValue {
	rr.mx.RLock()
	defer rr.mx.RUnlock()

	return rr.getSegmentPosition(segKey)
}

// getSegmentPosition - return position in storage.
func (rr *RefillReader) getSegmentPosition(segKey cppbridge.SegmentKey) *MarkupValue {
	mk := MarkupKey{
		typeFrame:  frames.SegmentType,
		SegmentKey: segKey,
	}

	return rr.markupFile.GetMarkupValue(mk)
}

// WriteRefillShardEOF - write message to mark that all segments have been sent to storage.
func (rr *RefillReader) WriteRefillShardEOF(ctx context.Context, dname string, shardID uint16) error {
	rr.mx.Lock()
	defer rr.mx.Unlock()

	// check open file
	if !rr.isOpenFile {
		if err := rr.openFile(); err != nil {
			return err
		}
	}

	dnameID := rr.markupFile.StringToID(dname)
	if dnameID == frames.NotFoundName {
		return fmt.Errorf(
			"StringToID: unknown name %s",
			dname,
		)
	}

	// create frame
	fe, err := frames.NewRefillShardEOFFrame(uint32(dnameID), shardID)
	if err != nil {
		return err
	}

	// write in storage
	n, err := fe.WriteTo(rr.storage.Writer(ctx, rr.lastWriteOffset))
	if err != nil {
		return err
	}

	// move position
	rr.lastWriteOffset += n

	// set last frame
	rr.markupFile.SetLastFrame(dname, shardID)

	return nil
}

// DeleteFile - close and delete current file.
func (rr *RefillReader) DeleteFile() error {
	rr.mx.Lock()
	defer rr.mx.Unlock()

	if rr.isOpenFile {
		err := rr.storage.Close()
		if err != nil {
			return err
		}
	}

	// set flag and delete
	rr.isOpenFile = false

	return rr.storage.DeleteCurrentFile()
}

// Close - close reader.
func (rr *RefillReader) Close() error {
	rr.mx.Lock()
	defer rr.mx.Unlock()

	rr.isOpenFile = false
	return rr.storage.Close()
}
