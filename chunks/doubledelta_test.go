package chunks

import (
	"io"
	"math/rand"
	"testing"

	"github.com/prometheus/common/model"
	"github.com/stretchr/testify/require"
)

func testDoubleDeltaChunk(t *testing.T) {
	ts := model.Time(14345645)
	v := int64(123123)

	var input []model.SamplePair
	for i := 0; i < 2000; i++ {
		ts += model.Time(rand.Int63n(100) + 1)
		v += rand.Int63n(1000)
		if rand.Int() > 0 {
			v *= -1
		}

		input = append(input, model.SamplePair{
			Timestamp: ts,
			Value:     model.SampleValue(v),
		})
	}

	c := NewDoubleDeltaChunk(rand.Intn(3000))

	app := c.Appender()
	for i, s := range input {
		err := app.Append(s.Timestamp, s.Value)
		if err == ErrChunkFull {
			input = input[:i]
			break
		}
		require.NoError(t, err, "at sample %d: %v", i, s)
	}

	result := []model.SamplePair{}

	it := c.Iterator()
	for s, ok := it.First(); ok; s, ok = it.Next() {
		result = append(result, s)
	}
	if it.Err() != io.EOF {
		require.NoError(t, it.Err())
	}
	require.Equal(t, input, result)
}

func TestDoubleDeltaChunk(t *testing.T) {
	for i := 0; i < 10000; i++ {
		testDoubleDeltaChunk(t)
	}
}
