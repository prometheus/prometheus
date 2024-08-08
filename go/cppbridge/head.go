package cppbridge

import (
	"math"
	"runtime"
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

type RecodedChunk struct {
	MinT        int64
	MaxT        int64
	SeriesId    uint32
	HasMoreData bool
	ChunkData   []byte
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
