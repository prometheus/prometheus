package storage_ng

import (
	"io"

	clientmodel "github.com/prometheus/client_golang/model"

	"github.com/prometheus/prometheus/storage/metric"
)

type chunks []chunk

type chunk interface {
	add(*metric.SamplePair) chunks
	clone() chunk
	firstTime() clientmodel.Timestamp
	lastTime() clientmodel.Timestamp
	newIterator() chunkIterator
	marshal(io.Writer) error
	unmarshal(io.Reader) error
	close()

	// TODO: remove?
	values() <-chan *metric.SamplePair
}

type chunkIterator interface {
	getValueAtTime(clientmodel.Timestamp) metric.Values
	getBoundaryValues(metric.Interval) metric.Values
	getRangeValues(metric.Interval) metric.Values
	contains(clientmodel.Timestamp) bool
}

func transcodeAndAdd(dst chunk, src chunk, s *metric.SamplePair) chunks {
	numTranscodes.Inc()
	defer src.close()

	head := dst
	body := chunks{}
	for v := range src.values() {
		newChunks := head.add(v)
		body = append(body, newChunks[:len(newChunks)-1]...)
		head = newChunks[len(newChunks)-1]
	}
	newChunks := head.add(s)
	body = append(body, newChunks[:len(newChunks)-1]...)
	head = newChunks[len(newChunks)-1]
	return append(body, head)
}

func chunkType(c chunk) byte {
	switch c.(type) {
	case *deltaEncodedChunk:
		return 0
	default:
		panic("unknown chunk type")
	}
}

func chunkForType(chunkType byte) chunk {
	switch chunkType {
	case 0:
		return newDeltaEncodedChunk(1, 1, false)
	default:
		panic("unknown chunk type")
	}
}
