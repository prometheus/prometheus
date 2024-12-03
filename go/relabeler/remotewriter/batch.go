package remotewriter

import "github.com/prometheus/prometheus/pp/go/cppbridge"

type SegmentedDecodedRefSamples struct {
	SegmentID   uint32
	ShardedData []*cppbridge.DecodedRefSamples
}

type batch struct {
	capacity int
	size     int
	data     []SegmentedDecodedRefSamples
}

func (b *batch) Add(segmentID uint32, shardedData []*cppbridge.DecodedRefSamples) {
	b.data = append(b.data, SegmentedDecodedRefSamples{
		SegmentID:   segmentID,
		ShardedData: shardedData,
	})
	for _, samples := range shardedData {
		b.size += samples.Size()
	}
}

func (b *batch) HasDataToSend() bool {
	return b.size >= b.capacity
}

func (b *batch) DataToSend() ([]*cppbridge.DecodedRefSamples, uint32) {
	var data []*cppbridge.DecodedRefSamples
	var segmentID uint32
	for _, segment := range b.data {
		data = append(data, segment.ShardedData...)
		if segment.SegmentID > segmentID {
			segmentID = segment.SegmentID
		}
	}
	return data, segmentID
}

func (b *batch) Reset() {
	b.size = 0
	b.data = nil
}
