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

func (*HeadWalEncoder) Version() uint8 {
	return EncodersVersion()
}

func (e *HeadWalEncoder) Encode(innerSeriesSlice []*InnerSeries) error {
	_, err := headWalEncoderAddInnerSeries(e.encoder, innerSeriesSlice)
	return err
}

func (e *HeadWalEncoder) Finalize() ([]byte, error) {
	_, segment, err := headWalEncoderFinalize(e.encoder)
	return segment, err
}

type HeadWalDecoder struct {
	lss     *LabelSetStorage
	decoder uintptr
}

func NewHeadWalDecoder(lss *LabelSetStorage, encoderVersion uint8) *HeadWalDecoder {
	d := &HeadWalDecoder{
		lss:     lss,
		decoder: headWalDecoderCtor(lss.Pointer(), encoderVersion),
	}

	runtime.SetFinalizer(d, func(d *HeadWalDecoder) {
		headWalDecoderDtor(d.decoder)
	})

	return d
}

func (d *HeadWalDecoder) Decode(segment []byte) (*InnerSeries, error) {
	return headWalDecoderDecode(d.decoder, segment)
}

func (d *HeadWalDecoder) CreateEncoder() *HeadWalEncoder {
	e := &HeadWalEncoder{
		lss:     d.lss,
		encoder: headWalDecoderCreateEncoder(d.decoder),
	}

	runtime.SetFinalizer(e, func(e *HeadWalEncoder) {
		headWalEncoderDtor(e.encoder)
	})

	return e
}
