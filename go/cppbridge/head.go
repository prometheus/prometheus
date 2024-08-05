package cppbridge

import (
	"runtime"
)

type HeadDataStorage struct {
	dataStorage uintptr
}

func NewHeadDataStorage() *HeadDataStorage {
	ds := &HeadDataStorage{
		dataStorage: seriesDataDataStorageCtor(),
	}

	runtime.SetFinalizer(ds, func(ds *HeadDataStorage) {
		seriesDataDataStorageDtor(ds.dataStorage)
	})

	return ds
}

type HeadEncoder struct {
	encoder     uintptr
	dataStorage *HeadDataStorage
}

func NewHeadEncoderWithDataStorageAndOutdatedSampleEncoder(dataStorage *HeadDataStorage) *HeadEncoder {
	encoder := &HeadEncoder{
		encoder:     seriesDataEncoderCtor(dataStorage.dataStorage),
		dataStorage: dataStorage,
	}

	runtime.SetFinalizer(encoder, func(e *HeadEncoder) {
		seriesDataEncoderDtor(e.encoder)
	})

	return encoder
}

func NewHeadEncoder() *HeadEncoder {
	return NewHeadEncoderWithDataStorageAndOutdatedSampleEncoder(NewHeadDataStorage())
}

func (e *HeadEncoder) Encode(seriesID uint32, timestamp int64, value float64) {
	seriesDataEncoderEncode(e.encoder, seriesID, timestamp, value)
}

func (e *HeadEncoder) EncodeInnerSeriesSlice(innerSeriesSlice []*InnerSeries) {
	seriesDataEncoderEncodeInnerSeriesSlice(e.encoder, innerSeriesSlice)
}

type HeadDataQuerier struct {
	querier     uintptr
	dataStorage *HeadDataStorage
}

func NewHeadDataQueryable(dataStorage *HeadDataStorage) *HeadDataQuerier {
	querier := &HeadDataQuerier{
		querier:     seriesDataQuerierCtor(dataStorage.dataStorage),
		dataStorage: dataStorage,
	}

	runtime.SetFinalizer(querier, func(querier *HeadDataQuerier) {
		seriesDataQuerierDtor(querier.querier)
	})

	return querier
}
