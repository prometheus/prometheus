package storage_ng

import (
	"testing"

	clientmodel "github.com/prometheus/client_golang/model"

	"github.com/prometheus/prometheus/storage/metric"
	"github.com/prometheus/prometheus/utility/test"
)

func newTestPersistence(t *testing.T) (Persistence, test.Closer) {
	dir := test.NewTemporaryDirectory("test_persistence", t)
	p, err := NewDiskPersistence(dir.Path(), 1024)
	if err != nil {
		dir.Close()
		t.Fatal(err)
	}
	return p, test.NewCallbackCloser(func() {
		p.Close()
		dir.Close()
	})
}

func buildTestChunks() map[clientmodel.Fingerprint]chunks {
	fps := clientmodel.Fingerprints{
		clientmodel.Metric{
			"label": "value1",
		}.Fingerprint(),
		clientmodel.Metric{
			"label": "value2",
		}.Fingerprint(),
		clientmodel.Metric{
			"label": "value3",
		}.Fingerprint(),
	}
	fpToChunks := map[clientmodel.Fingerprint]chunks{}

	for _, fp := range fps {
		fpToChunks[fp] = make(chunks, 0, 10)
		for i := 0; i < 10; i++ {
			fpToChunks[fp] = append(fpToChunks[fp], newDeltaEncodedChunk(d1, d1, true).add(&metric.SamplePair{
				Timestamp: clientmodel.Timestamp(i),
				Value:     clientmodel.SampleValue(fp),
			})[0])
		}
	}
	return fpToChunks
}

func chunksEqual(c1, c2 chunk) bool {
	values2 := c2.values()
	for v1 := range c1.values() {
		v2 := <-values2
		if !v1.Equal(v2) {
			return false
		}
	}
	return true
}

func TestPersistChunk(t *testing.T) {
	p, closer := newTestPersistence(t)
	defer closer.Close()

	fpToChunks := buildTestChunks()

	for fp, chunks := range fpToChunks {
		for _, c := range chunks {
			if err := p.PersistChunk(fp, c); err != nil {
				t.Fatal(err)
			}
		}
	}

	for fp, expectedChunks := range fpToChunks {
		indexes := make([]int, 0, len(expectedChunks))
		for i := range expectedChunks {
			indexes = append(indexes, i)
		}
		actualChunks, err := p.LoadChunks(fp, indexes)
		if err != nil {
			t.Fatal(err)
		}
		for _, i := range indexes {
			if !chunksEqual(expectedChunks[i], actualChunks[i]) {
				t.Fatalf("%d. Chunks not equal.", i)
			}
		}
	}
}
