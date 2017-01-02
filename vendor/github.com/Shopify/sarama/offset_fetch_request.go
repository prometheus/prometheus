package sarama

type OffsetFetchRequest struct {
	ConsumerGroup string
	Version       int16
	partitions    map[string][]int32
}

func (r *OffsetFetchRequest) encode(pe packetEncoder) (err error) {
	if r.Version < 0 || r.Version > 1 {
		return PacketEncodingError{"invalid or unsupported OffsetFetchRequest version field"}
	}

	if err = pe.putString(r.ConsumerGroup); err != nil {
		return err
	}
	if err = pe.putArrayLength(len(r.partitions)); err != nil {
		return err
	}
	for topic, partitions := range r.partitions {
		if err = pe.putString(topic); err != nil {
			return err
		}
		if err = pe.putInt32Array(partitions); err != nil {
			return err
		}
	}
	return nil
}

func (r *OffsetFetchRequest) decode(pd packetDecoder, version int16) (err error) {
	r.Version = version
	if r.ConsumerGroup, err = pd.getString(); err != nil {
		return err
	}
	partitionCount, err := pd.getArrayLength()
	if err != nil {
		return err
	}
	if partitionCount == 0 {
		return nil
	}
	r.partitions = make(map[string][]int32)
	for i := 0; i < partitionCount; i++ {
		topic, err := pd.getString()
		if err != nil {
			return err
		}
		partitions, err := pd.getInt32Array()
		if err != nil {
			return err
		}
		r.partitions[topic] = partitions
	}
	return nil
}

func (r *OffsetFetchRequest) key() int16 {
	return 9
}

func (r *OffsetFetchRequest) version() int16 {
	return r.Version
}

func (r *OffsetFetchRequest) requiredVersion() KafkaVersion {
	switch r.Version {
	case 1:
		return V0_8_2_0
	default:
		return minVersion
	}
}

func (r *OffsetFetchRequest) AddPartition(topic string, partitionID int32) {
	if r.partitions == nil {
		r.partitions = make(map[string][]int32)
	}

	r.partitions[topic] = append(r.partitions[topic], partitionID)
}
