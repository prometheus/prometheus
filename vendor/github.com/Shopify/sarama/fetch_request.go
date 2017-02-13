package sarama

type fetchRequestBlock struct {
	fetchOffset int64
	maxBytes    int32
}

func (b *fetchRequestBlock) encode(pe packetEncoder) error {
	pe.putInt64(b.fetchOffset)
	pe.putInt32(b.maxBytes)
	return nil
}

func (b *fetchRequestBlock) decode(pd packetDecoder) (err error) {
	if b.fetchOffset, err = pd.getInt64(); err != nil {
		return err
	}
	if b.maxBytes, err = pd.getInt32(); err != nil {
		return err
	}
	return nil
}

type FetchRequest struct {
	MaxWaitTime int32
	MinBytes    int32
	Version     int16
	blocks      map[string]map[int32]*fetchRequestBlock
}

func (r *FetchRequest) encode(pe packetEncoder) (err error) {
	pe.putInt32(-1) // replica ID is always -1 for clients
	pe.putInt32(r.MaxWaitTime)
	pe.putInt32(r.MinBytes)
	err = pe.putArrayLength(len(r.blocks))
	if err != nil {
		return err
	}
	for topic, blocks := range r.blocks {
		err = pe.putString(topic)
		if err != nil {
			return err
		}
		err = pe.putArrayLength(len(blocks))
		if err != nil {
			return err
		}
		for partition, block := range blocks {
			pe.putInt32(partition)
			err = block.encode(pe)
			if err != nil {
				return err
			}
		}
	}
	return nil
}

func (r *FetchRequest) decode(pd packetDecoder, version int16) (err error) {
	r.Version = version
	if _, err = pd.getInt32(); err != nil {
		return err
	}
	if r.MaxWaitTime, err = pd.getInt32(); err != nil {
		return err
	}
	if r.MinBytes, err = pd.getInt32(); err != nil {
		return err
	}
	topicCount, err := pd.getArrayLength()
	if err != nil {
		return err
	}
	if topicCount == 0 {
		return nil
	}
	r.blocks = make(map[string]map[int32]*fetchRequestBlock)
	for i := 0; i < topicCount; i++ {
		topic, err := pd.getString()
		if err != nil {
			return err
		}
		partitionCount, err := pd.getArrayLength()
		if err != nil {
			return err
		}
		r.blocks[topic] = make(map[int32]*fetchRequestBlock)
		for j := 0; j < partitionCount; j++ {
			partition, err := pd.getInt32()
			if err != nil {
				return err
			}
			fetchBlock := &fetchRequestBlock{}
			if err = fetchBlock.decode(pd); err != nil {
				return nil
			}
			r.blocks[topic][partition] = fetchBlock
		}
	}
	return nil
}

func (r *FetchRequest) key() int16 {
	return 1
}

func (r *FetchRequest) version() int16 {
	return r.Version
}

func (r *FetchRequest) requiredVersion() KafkaVersion {
	switch r.Version {
	case 1:
		return V0_9_0_0
	case 2:
		return V0_10_0_0
	default:
		return minVersion
	}
}

func (r *FetchRequest) AddBlock(topic string, partitionID int32, fetchOffset int64, maxBytes int32) {
	if r.blocks == nil {
		r.blocks = make(map[string]map[int32]*fetchRequestBlock)
	}

	if r.blocks[topic] == nil {
		r.blocks[topic] = make(map[int32]*fetchRequestBlock)
	}

	tmp := new(fetchRequestBlock)
	tmp.maxBytes = maxBytes
	tmp.fetchOffset = fetchOffset

	r.blocks[topic][partitionID] = tmp
}
