package sarama

type OffsetResponseBlock struct {
	Err       KError
	Offsets   []int64 // Version 0
	Offset    int64   // Version 1
	Timestamp int64   // Version 1
}

func (b *OffsetResponseBlock) decode(pd packetDecoder, version int16) (err error) {
	tmp, err := pd.getInt16()
	if err != nil {
		return err
	}
	b.Err = KError(tmp)

	if version == 0 {
		b.Offsets, err = pd.getInt64Array()

		return err
	}

	b.Timestamp, err = pd.getInt64()
	if err != nil {
		return err
	}

	b.Offset, err = pd.getInt64()
	if err != nil {
		return err
	}

	// For backwards compatibility put the offset in the offsets array too
	b.Offsets = []int64{b.Offset}

	return nil
}

func (b *OffsetResponseBlock) encode(pe packetEncoder, version int16) (err error) {
	pe.putInt16(int16(b.Err))

	if version == 0 {
		return pe.putInt64Array(b.Offsets)
	}

	pe.putInt64(b.Timestamp)
	pe.putInt64(b.Offset)

	return nil
}

type OffsetResponse struct {
	Version int16
	Blocks  map[string]map[int32]*OffsetResponseBlock
}

func (r *OffsetResponse) decode(pd packetDecoder, version int16) (err error) {
	numTopics, err := pd.getArrayLength()
	if err != nil {
		return err
	}

	r.Blocks = make(map[string]map[int32]*OffsetResponseBlock, numTopics)
	for i := 0; i < numTopics; i++ {
		name, err := pd.getString()
		if err != nil {
			return err
		}

		numBlocks, err := pd.getArrayLength()
		if err != nil {
			return err
		}

		r.Blocks[name] = make(map[int32]*OffsetResponseBlock, numBlocks)

		for j := 0; j < numBlocks; j++ {
			id, err := pd.getInt32()
			if err != nil {
				return err
			}

			block := new(OffsetResponseBlock)
			err = block.decode(pd, version)
			if err != nil {
				return err
			}
			r.Blocks[name][id] = block
		}
	}

	return nil
}

func (r *OffsetResponse) GetBlock(topic string, partition int32) *OffsetResponseBlock {
	if r.Blocks == nil {
		return nil
	}

	if r.Blocks[topic] == nil {
		return nil
	}

	return r.Blocks[topic][partition]
}

/*
// [0 0 0 1 ntopics
0 8 109 121 95 116 111 112 105 99 topic
0 0 0 1 npartitions
0 0 0 0 id
0 0

0 0 0 1 0 0 0 0
0 1 1 1 0 0 0 1
0 8 109 121 95 116 111 112
105 99 0 0 0 1 0 0
0 0 0 0 0 0 0 1
0 0 0 0 0 1 1 1] <nil>

*/
func (r *OffsetResponse) encode(pe packetEncoder) (err error) {
	if err = pe.putArrayLength(len(r.Blocks)); err != nil {
		return err
	}

	for topic, partitions := range r.Blocks {
		if err = pe.putString(topic); err != nil {
			return err
		}
		if err = pe.putArrayLength(len(partitions)); err != nil {
			return err
		}
		for partition, block := range partitions {
			pe.putInt32(partition)
			if err = block.encode(pe, r.version()); err != nil {
				return err
			}
		}
	}

	return nil
}

func (r *OffsetResponse) key() int16 {
	return 2
}

func (r *OffsetResponse) version() int16 {
	return r.Version
}

func (r *OffsetResponse) requiredVersion() KafkaVersion {
	switch r.Version {
	case 1:
		return V0_10_1_0
	default:
		return minVersion
	}
}

// testing API

func (r *OffsetResponse) AddTopicPartition(topic string, partition int32, offset int64) {
	if r.Blocks == nil {
		r.Blocks = make(map[string]map[int32]*OffsetResponseBlock)
	}
	byTopic, ok := r.Blocks[topic]
	if !ok {
		byTopic = make(map[int32]*OffsetResponseBlock)
		r.Blocks[topic] = byTopic
	}
	byTopic[partition] = &OffsetResponseBlock{Offsets: []int64{offset}, Offset: offset}
}
