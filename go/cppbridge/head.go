package cppbridge

import (
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
