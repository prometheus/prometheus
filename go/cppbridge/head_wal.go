package cppbridge

import "runtime"

type HeadWalEncoder struct {
	lss     *LabelSetStorage
	encoder uintptr
}

func NewHeadWalEncoder(shardID uint16, logShards uint8, lss *LabelSetStorage) *HeadWalEncoder {
	e := &HeadWalEncoder{
		lss:     lss,
		encoder: headWalEncoderCtor(shardID, logShards, lss.Pointer()),
	}

	runtime.SetFinalizer(e, func(e *HeadWalEncoder) {
		headWalEncoderDtor(e.encoder)
	})

	return e
}

func (e *HeadWalEncoder) Encode(innerSeriesSlice []*InnerSeries) error {
	_, err := headWalEncoderAddInnerSeries(e.encoder, innerSeriesSlice)
	return handleException(err)
}

func (e *HeadWalEncoder) Finalize() ([]byte, error) {
	_, segment, err := headWalEncoderFinalize(e.encoder)
	return segment, handleException(err)
}

type HeadWalDecoder struct {
	lss     *LabelSetStorage
	decoder uintptr
}

func NewHeadWalDecoder()
