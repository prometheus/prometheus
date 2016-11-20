package chunks

import (
	"math/rand"
	"testing"

	"github.com/prometheus/common/model"
	"github.com/stretchr/testify/require"
)

func testXORChunk(t *testing.T) {
	ts := model.Time(124213233)
	v := int64(99954541)

	var input []model.SamplePair
	for i := 0; i < 10000; i++ {
		ts += model.Time(rand.Int63n(50000) + 1)
		v += rand.Int63n(1000)
		if rand.Int() > 0 {
			v *= -1
		}

		input = append(input, model.SamplePair{
			Timestamp: ts,
			Value:     model.SampleValue(v),
		})
	}

	c := NewXORChunk(rand.Intn(3000))

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

	it := c.Iterator().(*xorIterator)
	for {
		ok := it.NextB()
		if !ok {
			break
		}
		t, v := it.Values()
		result = append(result, model.SamplePair{Timestamp: model.Time(t), Value: model.SampleValue(v)})
	}

	require.NoError(t, it.Err())
	require.Equal(t, input, result)
}

func TestXORChunk(t *testing.T) {
	for i := 0; i < 10; i++ {
		testXORChunk(t)
	}
}
