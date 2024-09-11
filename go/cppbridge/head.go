package cppbridge

import (
	"fmt"
	"math"
	"runtime"
	"unsafe"
)

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
	MinT         int64
	MaxT         int64
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

	dataStorage *HeadDataStorage
}

func NewChunkRecoder(dataStorage *HeadDataStorage) *ChunkRecoder {
	chunkRecoder := &ChunkRecoder{
		recoder:     seriesDataChunkRecoderCtor(dataStorage.dataStorage),
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

type HeadDataStorageSerializedChunkMetadata [13]byte

func (cm *HeadDataStorageSerializedChunkMetadata) SeriesID() uint32 {
	return *(*uint32)(unsafe.Pointer(&cm[0]))
}

func (r *HeadDataStorageSerializedChunks) Data() []byte {
	return r.data
}

func (r *HeadDataStorageSerializedChunks) numberOfChunks() int {
	return int(*(*int32)(unsafe.Pointer(&r.data[0])))
}

func (r *HeadDataStorageSerializedChunks) ChunkMetadataList() []HeadDataStorageSerializedChunkMetadata {
	offset := 4
	chunkMetadataList := make([]HeadDataStorageSerializedChunkMetadata, 0, r.numberOfChunks())
	for i := 0; i < r.numberOfChunks(); i++ {
		chunkMetadataList = append(chunkMetadataList, HeadDataStorageSerializedChunkMetadata(r.data[offset:offset+13]))
		offset += 13
	}
	return chunkMetadataList
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
		deserializer:     seriesDataDeserializerCtor(serializedChunks.Data()),
		serializedChunks: serializedChunks,
	}
	runtime.SetFinalizer(d, func(d *HeadDataStorageDeserializer) {
		fmt.Println("Deserializer destroyed")
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
