package sarama

type OffsetFetchResponseBlock struct {
	Offset   int64
	Metadata string
	Err      KError
}

func (r *OffsetFetchResponseBlock) decode(pd packetDecoder) (err error) {
	r.Offset, err = pd.getInt64()
	if err != nil {
		return err
	}

	r.Metadata, err = pd.getString()
	if err != nil {
		return err
	}

	tmp, err := pd.getInt16()
	if err != nil {
		return err
	}
	r.Err = KError(tmp)

	return nil
}

func (r *OffsetFetchResponseBlock) encode(pe packetEncoder) (err error) {
	pe.putInt64(r.Offset)

	err = pe.putString(r.Metadata)
	if err != nil {
		return err
	}

	pe.putInt16(int16(r.Err))

	return nil
}

type OffsetFetchResponse struct {
	Blocks map[string]map[int32]*OffsetFetchResponseBlock
}

func (r *OffsetFetchResponse) encode(pe packetEncoder) error {
	if err := pe.putArrayLength(len(r.Blocks)); err != nil {
		return err
	}
	for topic, partitions := range r.Blocks {
		if err := pe.putString(topic); err != nil {
			return err
		}
		if err := pe.putArrayLength(len(partitions)); err != nil {
			return err
		}
		for partition, block := range partitions {
			pe.putInt32(partition)
			if err := block.encode(pe); err != nil {
				return err
			}
		}
	}
	return nil
}

func (r *OffsetFetchResponse) decode(pd packetDecoder) (err error) {
	numTopics, err := pd.getArrayLength()
	if err != nil || numTopics == 0 {
		return err
	}

	r.Blocks = make(map[string]map[int32]*OffsetFetchResponseBlock, numTopics)
	for i := 0; i < numTopics; i++ {
		name, err := pd.getString()
		if err != nil {
			return err
		}

		numBlocks, err := pd.getArrayLength()
		if err != nil {
			return err
		}

		if numBlocks == 0 {
			r.Blocks[name] = nil
			continue
		}
		r.Blocks[name] = make(map[int32]*OffsetFetchResponseBlock, numBlocks)

		for j := 0; j < numBlocks; j++ {
			id, err := pd.getInt32()
			if err != nil {
				return err
			}

			block := new(OffsetFetchResponseBlock)
			err = block.decode(pd)
			if err != nil {
				return err
			}
			r.Blocks[name][id] = block
		}
	}

	return nil
}

func (r *OffsetFetchResponse) GetBlock(topic string, partition int32) *OffsetFetchResponseBlock {
	if r.Blocks == nil {
		return nil
	}

	if r.Blocks[topic] == nil {
		return nil
	}

	return r.Blocks[topic][partition]
}

func (r *OffsetFetchResponse) AddBlock(topic string, partition int32, block *OffsetFetchResponseBlock) {
	if r.Blocks == nil {
		r.Blocks = make(map[string]map[int32]*OffsetFetchResponseBlock)
	}
	partitions := r.Blocks[topic]
	if partitions == nil {
		partitions = make(map[int32]*OffsetFetchResponseBlock)
		r.Blocks[topic] = partitions
	}
	partitions[partition] = block
}
