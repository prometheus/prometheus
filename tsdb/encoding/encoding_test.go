// Copyright The Prometheus Authors
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
// http://www.apache.org/licenses/LICENSE-2.0.

package encoding

import (
	"encoding/binary"
	"math"
	"strconv"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestUvarintBytesOversizedLength(t *testing.T) {
	for _, length := range []uint64{2, 1 << 31, 1 << 32, 1 << 63, math.MaxUint64} {
		t.Run(strconv.FormatUint(length, 10), func(t *testing.T) {
			data := binary.AppendUvarint(nil, length)
			data = append(data, 0)
			d := Decbuf{B: data}
			require.NotPanics(t, func() { require.Empty(t, d.UvarintBytes()) })
			require.ErrorIs(t, d.Err(), ErrInvalidSize)
		})
	}
}
