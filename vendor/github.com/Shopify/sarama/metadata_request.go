package sarama

type MetadataRequest struct {
	Topics []string
}

func (r *MetadataRequest) encode(pe packetEncoder) error {
	err := pe.putArrayLength(len(r.Topics))
	if err != nil {
		return err
	}

	for i := range r.Topics {
		err = pe.putString(r.Topics[i])
		if err != nil {
			return err
		}
	}
	return nil
}

func (r *MetadataRequest) decode(pd packetDecoder, version int16) error {
	topicCount, err := pd.getArrayLength()
	if err != nil {
		return err
	}
	if topicCount == 0 {
		return nil
	}

	r.Topics = make([]string, topicCount)
	for i := range r.Topics {
		topic, err := pd.getString()
		if err != nil {
			return err
		}
		r.Topics[i] = topic
	}
	return nil
}

func (r *MetadataRequest) key() int16 {
	return 3
}

func (r *MetadataRequest) version() int16 {
	return 0
}

func (r *MetadataRequest) requiredVersion() KafkaVersion {
	return minVersion
}
