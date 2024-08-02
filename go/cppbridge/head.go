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
