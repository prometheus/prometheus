package cppbridge

import (
	"math"
	"runtime"
	"unsafe"
)

const (
	MaxPointsInChunk            = 240
	Uint32Size                  = 4
	SerializedChunkMetadataSize = 13
)

type TimeInterval struct {
	MinT int64
	MaxT int64
}

// HeadDataStorage is Go wrapper around series_data::Data_storage.
type HeadDataStorage struct {
	dataStorage uintptr
}

// NewHeadDataStorage - constructor.
func NewHeadDataStorage() *HeadDataStorage {
	ds := &HeadDataStorage{
		dataStorage: seriesDataDataStorageCtor(),
	}

	runtime.SetFinalizer(ds, func(ds *HeadDataStorage) {
		seriesDataDataStorageDtor(ds.dataStorage)
	})

	return ds
}

// Reset - resets data storage.
func (ds *HeadDataStorage) Reset() {
	seriesDataDataStorageReset(ds.dataStorage)
}

func (ds *HeadDataStorage) TimeInterval() TimeInterval {
	return seriesDataDataStorageTimeInterval(ds.dataStorage)
}

func (ds *HeadDataStorage) Pointer() uintptr {
	return ds.dataStorage
}

func (ds *HeadDataStorage) AllocatedMemory() uint64 {
	return seriesDataDataStorageAllocatedMemory(ds.dataStorage)
}

// HeadEncoder is Go wrapper around series_data::Encoder.
type HeadEncoder struct {
	encoder     uintptr
	dataStorage *HeadDataStorage
}

// NewHeadEncoderWithDataStorage - constructor.
func NewHeadEncoderWithDataStorage(dataStorage *HeadDataStorage) *HeadEncoder {
	encoder := &HeadEncoder{
		encoder:     seriesDataEncoderCtor(dataStorage.dataStorage),
		dataStorage: dataStorage,
	}

	runtime.SetFinalizer(encoder, func(e *HeadEncoder) {
		seriesDataEncoderDtor(e.encoder)
	})

	return encoder
}

// NewHeadEncoder - constructor.
func NewHeadEncoder() *HeadEncoder {
	return NewHeadEncoderWithDataStorage(NewHeadDataStorage())
}

// Encode - encodes single triplet.
func (e *HeadEncoder) Encode(seriesID uint32, timestamp int64, value float64) {
	seriesDataEncoderEncode(e.encoder, seriesID, timestamp, value)
}

// EncodeInnerSeriesSlice - encodes InnerSeries slice produced by relabeler.
func (e *HeadEncoder) EncodeInnerSeriesSlice(innerSeriesSlice []*InnerSeries) {
	seriesDataEncoderEncodeInnerSeriesSlice(e.encoder, innerSeriesSlice)
}

func (e *HeadEncoder) MergeOutOfOrderChunks() {
	seriesDataEncoderMergeOutOfOrderChunks(e.encoder)
}

type RecodedChunk struct {
	TimeInterval
	SeriesId     uint32
	SamplesCount uint8
	HasMoreData  bool
	ChunkData    []byte
}

const (
	InvalidSeriesId = math.MaxUint32
)

// ChunkRecoder is Go wrapper around C++ ChunkRecoder.
type ChunkRecoder struct {
	recoder      uintptr
	recodedChunk RecodedChunk

	lss         *LabelSetStorage
	dataStorage *HeadDataStorage
}

func NewChunkRecoder(lss *LabelSetStorage, dataStorage *HeadDataStorage, timeInterval TimeInterval) *ChunkRecoder {
	chunkRecoder := &ChunkRecoder{
		recoder:     seriesDataChunkRecoderCtor(lss.Pointer(), dataStorage.dataStorage, timeInterval),
		lss:         lss,
		dataStorage: dataStorage,
	}

	runtime.SetFinalizer(chunkRecoder, func(chunkRecoder *ChunkRecoder) {
		seriesDataChunkRecoderDtor(chunkRecoder.recoder)
	})

	return chunkRecoder
}

func (recoder *ChunkRecoder) RecodeNextChunk() RecodedChunk {
	seriesDataChunkRecoderRecodeNextChunk(recoder.recoder, &recoder.recodedChunk)
	return recoder.recodedChunk
}

type HeadDataStorageQuery struct {
	StartTimestampMs int64
	EndTimestampMs   int64
	LabelSetIDs      []uint32
}

type HeadDataStorageSerializedChunks struct {
	data []byte
}

type HeadDataStorageSerializedChunkMetadata [SerializedChunkMetadataSize]byte

func (cm *HeadDataStorageSerializedChunkMetadata) SeriesID() uint32 {
	return *(*uint32)(unsafe.Pointer(&cm[0]))
}

func (r *HeadDataStorageSerializedChunks) NumberOfChunks() int {
	return int(*(*int32)(unsafe.Pointer(&r.data[0])))
}

func (r *HeadDataStorageSerializedChunks) Len() int {
	return len(r.data)
}

type HeadDataStorageSerializedChunkIndex struct {
	m map[uint32][]int
}

func (r *HeadDataStorageSerializedChunks) MakeIndex() HeadDataStorageSerializedChunkIndex {
	m := make(map[uint32][]int)
	offset := Uint32Size
	n := r.NumberOfChunks()
	for i := 0; i < n; i, offset = i+1, offset+SerializedChunkMetadataSize {
		md := HeadDataStorageSerializedChunkMetadata(r.data[offset : offset+SerializedChunkMetadataSize])
		m[md.SeriesID()] = append(m[md.SeriesID()], offset)
	}
	return HeadDataStorageSerializedChunkIndex{m}
}

func (i HeadDataStorageSerializedChunkIndex) Has(seriesID uint32) bool {
	return len(i.m[seriesID]) > 0
}

func (i HeadDataStorageSerializedChunkIndex) Len() int {
	return len(i.m)
}

func (i HeadDataStorageSerializedChunkIndex) Chunks(r *HeadDataStorageSerializedChunks, seriesID uint32) []HeadDataStorageSerializedChunkMetadata {
	offsets, ok := i.m[seriesID]
	if !ok {
		return nil
	}
	res := make([]HeadDataStorageSerializedChunkMetadata, len(offsets))
	for i, offset := range offsets {
		res[i] = HeadDataStorageSerializedChunkMetadata(r.data[offset : offset+SerializedChunkMetadataSize])
	}
	return res
}

func (ds *HeadDataStorage) Query(query HeadDataStorageQuery) *HeadDataStorageSerializedChunks {
	serializedChunks := &HeadDataStorageSerializedChunks{
		data: seriesDataDataStorageQuery(ds.dataStorage, query),
	}
	runtime.SetFinalizer(serializedChunks, func(sc *HeadDataStorageSerializedChunks) {
		freeBytes(sc.data)
	})
	return serializedChunks
}

type HeadDataStorageDeserializer struct {
	deserializer     uintptr
	serializedChunks *HeadDataStorageSerializedChunks
}

func NewHeadDataStorageDeserializer(serializedChunks *HeadDataStorageSerializedChunks) *HeadDataStorageDeserializer {
	d := &HeadDataStorageDeserializer{
		deserializer:     seriesDataDeserializerCtor(serializedChunks.data),
		serializedChunks: serializedChunks,
	}
	runtime.SetFinalizer(d, func(d *HeadDataStorageDeserializer) {
		seriesDataDeserializerDtor(d.deserializer)
	})
	return d
}

func (d *HeadDataStorageDeserializer) CreateDecodeIterator(chunkMetadata HeadDataStorageSerializedChunkMetadata) *HeadDataStorageDecodeIterator {
	decodeIterator := &HeadDataStorageDecodeIterator{
		decodeIterator: seriesDataDeserializerCreateDecodeIterator(d.deserializer, chunkMetadata[:]),
	}

	runtime.SetFinalizer(decodeIterator, func(decodeIterator *HeadDataStorageDecodeIterator) {
		seriesDataDecodeIteratorDtor(decodeIterator.decodeIterator)
	})

	return decodeIterator
}

type HeadDataStorageDecodeIterator struct {
	decodeIterator uintptr
	started        bool
	finished       bool
}

func (i *HeadDataStorageDecodeIterator) Next() bool {
	if !i.started {
		i.started = true
		return true
	}

	if i.finished {
		return false
	}

	i.finished = !seriesDataDecodeIteratorNext(i.decodeIterator)
	return !i.finished
}

func (i *HeadDataStorageDecodeIterator) Sample() (int64, float64) {
	return seriesDataDecodeIteratorSample(i.decodeIterator)
}
