package sarama

import (
	"fmt"
	"math"

	"github.com/rcrowley/go-metrics"
)

type prepEncoder struct {
	length int
}

// primitives

func (pe *prepEncoder) putInt8(in int8) {
	pe.length++
}

func (pe *prepEncoder) putInt16(in int16) {
	pe.length += 2
}

func (pe *prepEncoder) putInt32(in int32) {
	pe.length += 4
}

func (pe *prepEncoder) putInt64(in int64) {
	pe.length += 8
}

func (pe *prepEncoder) putArrayLength(in int) error {
	if in > math.MaxInt32 {
		return PacketEncodingError{fmt.Sprintf("array too long (%d)", in)}
	}
	pe.length += 4
	return nil
}

// arrays

func (pe *prepEncoder) putBytes(in []byte) error {
	pe.length += 4
	if in == nil {
		return nil
	}
	if len(in) > math.MaxInt32 {
		return PacketEncodingError{fmt.Sprintf("byteslice too long (%d)", len(in))}
	}
	pe.length += len(in)
	return nil
}

func (pe *prepEncoder) putRawBytes(in []byte) error {
	if len(in) > math.MaxInt32 {
		return PacketEncodingError{fmt.Sprintf("byteslice too long (%d)", len(in))}
	}
	pe.length += len(in)
	return nil
}

func (pe *prepEncoder) putString(in string) error {
	pe.length += 2
	if len(in) > math.MaxInt16 {
		return PacketEncodingError{fmt.Sprintf("string too long (%d)", len(in))}
	}
	pe.length += len(in)
	return nil
}

func (pe *prepEncoder) putStringArray(in []string) error {
	err := pe.putArrayLength(len(in))
	if err != nil {
		return err
	}

	for _, str := range in {
		if err := pe.putString(str); err != nil {
			return err
		}
	}

	return nil
}

func (pe *prepEncoder) putInt32Array(in []int32) error {
	err := pe.putArrayLength(len(in))
	if err != nil {
		return err
	}
	pe.length += 4 * len(in)
	return nil
}

func (pe *prepEncoder) putInt64Array(in []int64) error {
	err := pe.putArrayLength(len(in))
	if err != nil {
		return err
	}
	pe.length += 8 * len(in)
	return nil
}

func (pe *prepEncoder) offset() int {
	return pe.length
}

// stackable

func (pe *prepEncoder) push(in pushEncoder) {
	pe.length += in.reserveLength()
}

func (pe *prepEncoder) pop() error {
	return nil
}

// we do not record metrics during the prep encoder pass
func (pe *prepEncoder) metricRegistry() metrics.Registry {
	return nil
}
