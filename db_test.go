package tsdb

import (
	"fmt"
	"io"
	"io/ioutil"
	"math/rand"
	"os"
	"testing"
	"time"

	"github.com/fabxc/tindex"
	"github.com/prometheus/common/log"
	"github.com/prometheus/common/model"
	"github.com/prometheus/prometheus/storage/cinamon/chunk"
	"github.com/prometheus/prometheus/storage/metric"
	"github.com/stretchr/testify/require"
)

func TestE2E(t *testing.T) {
	dir, err := ioutil.TempDir("", "cinamon_test")
	require.NoError(t, err)
	defer os.RemoveAll(dir)

	c, err := Open(dir, log.Base(), nil)
	require.NoError(t, err)

	c.memChunks.indexer.timeout = 50 * time.Millisecond

	// Set indexer size to be triggered exactly when we hit the limit.
	// c.memChunks.indexer.qmax = 10

	mets := generateMetrics(100000)
	// var wg sync.WaitGroup
	// for k := 0; k < len(mets)/100+1; k++ {
	// 	wg.Add(1)
	// 	go func(mets []model.Metric) {
	var s Scrape
	for i := 0; i < 2*64; i++ {
		s.Reset(model.Time(i) * 100000)

		for _, m := range mets {
			s.Add(m, model.SampleValue(rand.Float64()))
		}
		require.NoError(t, c.Append(&s))
	}
	// 		wg.Done()
	// 	}(mets[k*100 : (k+1)*100])
	// }
	// wg.Wait()

	start := time.Now()
	c.memChunks.indexer.wait()
	fmt.Println("index wait", time.Since(start))

	start = time.Now()
	q, err := c.Querier()
	require.NoError(t, err)
	defer q.Close()

	m1, err := metric.NewLabelMatcher(metric.Equal, "job", "somejob")
	require.NoError(t, err)
	m2, err := metric.NewLabelMatcher(metric.Equal, "label2", "value0")
	require.NoError(t, err)
	m3, err := metric.NewLabelMatcher(metric.Equal, "label4", "value0")
	require.NoError(t, err)

	it, err := q.Iterator(m1, m2, m3)
	require.NoError(t, err)
	res, err := tindex.ExpandIterator(it)
	require.NoError(t, err)
	fmt.Println("result len", len(res))

	fmt.Println("querying", time.Since(start))
}

func generateMetrics(n int) (res []model.Metric) {
	for i := 0; i < n; i++ {
		res = append(res, model.Metric{
			"job":    "somejob",
			"label5": model.LabelValue(fmt.Sprintf("value%d", i%10)),
			"label4": model.LabelValue(fmt.Sprintf("value%d", i%5)),
			"label3": model.LabelValue(fmt.Sprintf("value%d", i%3)),
			"label2": model.LabelValue(fmt.Sprintf("value%d", i%2)),
			"label1": model.LabelValue(fmt.Sprintf("value%d", i)),
		})
	}
	return res
}

func TestMemChunksShardGet(t *testing.T) {
	cs := &memChunksShard{
		descs: map[model.Fingerprint][]*chunkDesc{},
		csize: 100,
	}
	cdesc1, created1 := cs.get(123, model.Metric{"x": "1"})
	require.True(t, created1)
	require.Equal(t, 1, len(cs.descs[123]))
	require.Equal(t, &chunkDesc{
		met:   model.Metric{"x": "1"},
		chunk: chunk.NewPlainChunk(100),
	}, cdesc1)

	// Add colliding metric.
	cdesc2, created2 := cs.get(123, model.Metric{"x": "2"})
	require.True(t, created2)
	require.Equal(t, 2, len(cs.descs[123]))
	require.Equal(t, &chunkDesc{
		met:   model.Metric{"x": "2"},
		chunk: chunk.NewPlainChunk(100),
	}, cdesc2)
	// First chunk desc can still be retrieved correctly.
	cdesc1, created1 = cs.get(123, model.Metric{"x": "1"})
	require.False(t, created1)
	require.Equal(t, &chunkDesc{
		met:   model.Metric{"x": "1"},
		chunk: chunk.NewPlainChunk(100),
	}, cdesc1)
}

func TestChunkSeriesIterator(t *testing.T) {
	newChunk := func(s []model.SamplePair) chunk.Chunk {
		c := chunk.NewPlainChunk(1000)
		app := c.Appender()
		for _, sp := range s {
			if err := app.Append(sp.Timestamp, sp.Value); err != nil {
				t.Fatal(err)
			}
		}
		return c
	}
	it := newChunkSeriesIterator(metric.Metric{}, []chunk.Chunk{
		newChunk([]model.SamplePair{{1, 1}, {2, 2}, {3, 3}}),
		newChunk([]model.SamplePair{{4, 4}, {5, 5}, {6, 6}}),
		newChunk([]model.SamplePair{{7, 7}, {8, 8}, {9, 9}}),
	})

	var res []model.SamplePair
	for sp, ok := it.Seek(0); ok; sp, ok = it.Next() {
		fmt.Println(sp)
		res = append(res, sp)
	}
	require.Equal(t, io.EOF, it.Err())
	require.Equal(t, []model.SamplePair{{1, 1}, {2, 2}, {3, 3}, {4, 4}, {5, 5}, {6, 6}, {7, 7}, {8, 8}, {9, 9}}, res)
}
