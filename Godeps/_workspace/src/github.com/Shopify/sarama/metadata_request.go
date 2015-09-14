package sarama

type MetadataRequest struct {
	Topics []string
}

func (mr *MetadataRequest) encode(pe packetEncoder) error {
	err := pe.putArrayLength(len(mr.Topics))
	if err != nil {
		return err
	}

	for i := range mr.Topics {
		err = pe.putString(mr.Topics[i])
		if err != nil {
			return err
		}
	}
	return nil
}

func (mr *MetadataRequest) decode(pd packetDecoder) error {
	topicCount, err := pd.getArrayLength()
	if err != nil {
		return err
	}
	if topicCount == 0 {
		return nil
	}

	mr.Topics = make([]string, topicCount)
	for i := range mr.Topics {
		topic, err := pd.getString()
		if err != nil {
			return err
		}
		mr.Topics[i] = topic
	}
	return nil
}

func (mr *MetadataRequest) key() int16 {
	return 3
}

func (mr *MetadataRequest) version() int16 {
	return 0
}
