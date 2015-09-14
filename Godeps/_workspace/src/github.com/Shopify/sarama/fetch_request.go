package sarama

type fetchRequestBlock struct {
	fetchOffset int64
	maxBytes    int32
}

func (f *fetchRequestBlock) encode(pe packetEncoder) error {
	pe.putInt64(f.fetchOffset)
	pe.putInt32(f.maxBytes)
	return nil
}

func (f *fetchRequestBlock) decode(pd packetDecoder) (err error) {
	if f.fetchOffset, err = pd.getInt64(); err != nil {
		return err
	}
	if f.maxBytes, err = pd.getInt32(); err != nil {
		return err
	}
	return nil
}

type FetchRequest struct {
	MaxWaitTime int32
	MinBytes    int32
	blocks      map[string]map[int32]*fetchRequestBlock
}

func (f *FetchRequest) encode(pe packetEncoder) (err error) {
	pe.putInt32(-1) // replica ID is always -1 for clients
	pe.putInt32(f.MaxWaitTime)
	pe.putInt32(f.MinBytes)
	err = pe.putArrayLength(len(f.blocks))
	if err != nil {
		return err
	}
	for topic, blocks := range f.blocks {
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

func (f *FetchRequest) decode(pd packetDecoder) (err error) {
	if _, err = pd.getInt32(); err != nil {
		return err
	}
	if f.MaxWaitTime, err = pd.getInt32(); err != nil {
		return err
	}
	if f.MinBytes, err = pd.getInt32(); err != nil {
		return err
	}
	topicCount, err := pd.getArrayLength()
	if err != nil {
		return err
	}
	if topicCount == 0 {
		return nil
	}
	f.blocks = make(map[string]map[int32]*fetchRequestBlock)
	for i := 0; i < topicCount; i++ {
		topic, err := pd.getString()
		if err != nil {
			return err
		}
		partitionCount, err := pd.getArrayLength()
		if err != nil {
			return err
		}
		f.blocks[topic] = make(map[int32]*fetchRequestBlock)
		for j := 0; j < partitionCount; j++ {
			partition, err := pd.getInt32()
			if err != nil {
				return err
			}
			fetchBlock := &fetchRequestBlock{}
			if err = fetchBlock.decode(pd); err != nil {
				return nil
			}
			f.blocks[topic][partition] = fetchBlock
		}
	}
	return nil
}

func (f *FetchRequest) key() int16 {
	return 1
}

func (f *FetchRequest) version() int16 {
	return 0
}

func (f *FetchRequest) AddBlock(topic string, partitionID int32, fetchOffset int64, maxBytes int32) {
	if f.blocks == nil {
		f.blocks = make(map[string]map[int32]*fetchRequestBlock)
	}

	if f.blocks[topic] == nil {
		f.blocks[topic] = make(map[int32]*fetchRequestBlock)
	}

	tmp := new(fetchRequestBlock)
	tmp.maxBytes = maxBytes
	tmp.fetchOffset = fetchOffset

	f.blocks[topic][partitionID] = tmp
}
