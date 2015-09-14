package sarama

type FetchResponseBlock struct {
	Err                 KError
	HighWaterMarkOffset int64
	MsgSet              MessageSet
}

func (pr *FetchResponseBlock) decode(pd packetDecoder) (err error) {
	tmp, err := pd.getInt16()
	if err != nil {
		return err
	}
	pr.Err = KError(tmp)

	pr.HighWaterMarkOffset, err = pd.getInt64()
	if err != nil {
		return err
	}

	msgSetSize, err := pd.getInt32()
	if err != nil {
		return err
	}

	msgSetDecoder, err := pd.getSubset(int(msgSetSize))
	if err != nil {
		return err
	}
	err = (&pr.MsgSet).decode(msgSetDecoder)

	return err
}

type FetchResponse struct {
	Blocks map[string]map[int32]*FetchResponseBlock
}

func (pr *FetchResponseBlock) encode(pe packetEncoder) (err error) {
	pe.putInt16(int16(pr.Err))

	pe.putInt64(pr.HighWaterMarkOffset)

	pe.push(&lengthField{})
	err = pr.MsgSet.encode(pe)
	if err != nil {
		return err
	}
	return pe.pop()
}

func (fr *FetchResponse) decode(pd packetDecoder) (err error) {
	numTopics, err := pd.getArrayLength()
	if err != nil {
		return err
	}

	fr.Blocks = make(map[string]map[int32]*FetchResponseBlock, numTopics)
	for i := 0; i < numTopics; i++ {
		name, err := pd.getString()
		if err != nil {
			return err
		}

		numBlocks, err := pd.getArrayLength()
		if err != nil {
			return err
		}

		fr.Blocks[name] = make(map[int32]*FetchResponseBlock, numBlocks)

		for j := 0; j < numBlocks; j++ {
			id, err := pd.getInt32()
			if err != nil {
				return err
			}

			block := new(FetchResponseBlock)
			err = block.decode(pd)
			if err != nil {
				return err
			}
			fr.Blocks[name][id] = block
		}
	}

	return nil
}

func (fr *FetchResponse) encode(pe packetEncoder) (err error) {
	err = pe.putArrayLength(len(fr.Blocks))
	if err != nil {
		return err
	}

	for topic, partitions := range fr.Blocks {
		err = pe.putString(topic)
		if err != nil {
			return err
		}

		err = pe.putArrayLength(len(partitions))
		if err != nil {
			return err
		}

		for id, block := range partitions {
			pe.putInt32(id)
			err = block.encode(pe)
			if err != nil {
				return err
			}
		}

	}
	return nil
}

func (fr *FetchResponse) GetBlock(topic string, partition int32) *FetchResponseBlock {
	if fr.Blocks == nil {
		return nil
	}

	if fr.Blocks[topic] == nil {
		return nil
	}

	return fr.Blocks[topic][partition]
}

func (fr *FetchResponse) AddError(topic string, partition int32, err KError) {
	if fr.Blocks == nil {
		fr.Blocks = make(map[string]map[int32]*FetchResponseBlock)
	}
	partitions, ok := fr.Blocks[topic]
	if !ok {
		partitions = make(map[int32]*FetchResponseBlock)
		fr.Blocks[topic] = partitions
	}
	frb, ok := partitions[partition]
	if !ok {
		frb = new(FetchResponseBlock)
		partitions[partition] = frb
	}
	frb.Err = err
}

func (fr *FetchResponse) AddMessage(topic string, partition int32, key, value Encoder, offset int64) {
	if fr.Blocks == nil {
		fr.Blocks = make(map[string]map[int32]*FetchResponseBlock)
	}
	partitions, ok := fr.Blocks[topic]
	if !ok {
		partitions = make(map[int32]*FetchResponseBlock)
		fr.Blocks[topic] = partitions
	}
	frb, ok := partitions[partition]
	if !ok {
		frb = new(FetchResponseBlock)
		partitions[partition] = frb
	}
	var kb []byte
	var vb []byte
	if key != nil {
		kb, _ = key.Encode()
	}
	if value != nil {
		vb, _ = value.Encode()
	}
	msg := &Message{Key: kb, Value: vb}
	msgBlock := &MessageBlock{Msg: msg, Offset: offset}
	frb.MsgSet.Messages = append(frb.MsgSet.Messages, msgBlock)
}
