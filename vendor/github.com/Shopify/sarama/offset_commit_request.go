package sarama

// ReceiveTime is a special value for the timestamp field of Offset Commit Requests which
// tells the broker to set the timestamp to the time at which the request was received.
// The timestamp is only used if message version 1 is used, which requires kafka 0.8.2.
const ReceiveTime int64 = -1

// GroupGenerationUndefined is a special value for the group generation field of
// Offset Commit Requests that should be used when a consumer group does not rely
// on Kafka for partition management.
const GroupGenerationUndefined = -1

type offsetCommitRequestBlock struct {
	offset    int64
	timestamp int64
	metadata  string
}

func (b *offsetCommitRequestBlock) encode(pe packetEncoder, version int16) error {
	pe.putInt64(b.offset)
	if version == 1 {
		pe.putInt64(b.timestamp)
	} else if b.timestamp != 0 {
		Logger.Println("Non-zero timestamp specified for OffsetCommitRequest not v1, it will be ignored")
	}

	return pe.putString(b.metadata)
}

func (b *offsetCommitRequestBlock) decode(pd packetDecoder, version int16) (err error) {
	if b.offset, err = pd.getInt64(); err != nil {
		return err
	}
	if version == 1 {
		if b.timestamp, err = pd.getInt64(); err != nil {
			return err
		}
	}
	b.metadata, err = pd.getString()
	return err
}

type OffsetCommitRequest struct {
	ConsumerGroup           string
	ConsumerGroupGeneration int32  // v1 or later
	ConsumerID              string // v1 or later
	RetentionTime           int64  // v2 or later

	// Version can be:
	// - 0 (kafka 0.8.1 and later)
	// - 1 (kafka 0.8.2 and later)
	// - 2 (kafka 0.9.0 and later)
	Version int16
	blocks  map[string]map[int32]*offsetCommitRequestBlock
}

func (r *OffsetCommitRequest) encode(pe packetEncoder) error {
	if r.Version < 0 || r.Version > 2 {
		return PacketEncodingError{"invalid or unsupported OffsetCommitRequest version field"}
	}

	if err := pe.putString(r.ConsumerGroup); err != nil {
		return err
	}

	if r.Version >= 1 {
		pe.putInt32(r.ConsumerGroupGeneration)
		if err := pe.putString(r.ConsumerID); err != nil {
			return err
		}
	} else {
		if r.ConsumerGroupGeneration != 0 {
			Logger.Println("Non-zero ConsumerGroupGeneration specified for OffsetCommitRequest v0, it will be ignored")
		}
		if r.ConsumerID != "" {
			Logger.Println("Non-empty ConsumerID specified for OffsetCommitRequest v0, it will be ignored")
		}
	}

	if r.Version >= 2 {
		pe.putInt64(r.RetentionTime)
	} else if r.RetentionTime != 0 {
		Logger.Println("Non-zero RetentionTime specified for OffsetCommitRequest version <2, it will be ignored")
	}

	if err := pe.putArrayLength(len(r.blocks)); err != nil {
		return err
	}
	for topic, partitions := range r.blocks {
		if err := pe.putString(topic); err != nil {
			return err
		}
		if err := pe.putArrayLength(len(partitions)); err != nil {
			return err
		}
		for partition, block := range partitions {
			pe.putInt32(partition)
			if err := block.encode(pe, r.Version); err != nil {
				return err
			}
		}
	}
	return nil
}

func (r *OffsetCommitRequest) decode(pd packetDecoder, version int16) (err error) {
	r.Version = version

	if r.ConsumerGroup, err = pd.getString(); err != nil {
		return err
	}

	if r.Version >= 1 {
		if r.ConsumerGroupGeneration, err = pd.getInt32(); err != nil {
			return err
		}
		if r.ConsumerID, err = pd.getString(); err != nil {
			return err
		}
	}

	if r.Version >= 2 {
		if r.RetentionTime, err = pd.getInt64(); err != nil {
			return err
		}
	}

	topicCount, err := pd.getArrayLength()
	if err != nil {
		return err
	}
	if topicCount == 0 {
		return nil
	}
	r.blocks = make(map[string]map[int32]*offsetCommitRequestBlock)
	for i := 0; i < topicCount; i++ {
		topic, err := pd.getString()
		if err != nil {
			return err
		}
		partitionCount, err := pd.getArrayLength()
		if err != nil {
			return err
		}
		r.blocks[topic] = make(map[int32]*offsetCommitRequestBlock)
		for j := 0; j < partitionCount; j++ {
			partition, err := pd.getInt32()
			if err != nil {
				return err
			}
			block := &offsetCommitRequestBlock{}
			if err := block.decode(pd, r.Version); err != nil {
				return err
			}
			r.blocks[topic][partition] = block
		}
	}
	return nil
}

func (r *OffsetCommitRequest) key() int16 {
	return 8
}

func (r *OffsetCommitRequest) version() int16 {
	return r.Version
}

func (r *OffsetCommitRequest) requiredVersion() KafkaVersion {
	switch r.Version {
	case 1:
		return V0_8_2_0
	case 2:
		return V0_9_0_0
	default:
		return minVersion
	}
}

func (r *OffsetCommitRequest) AddBlock(topic string, partitionID int32, offset int64, timestamp int64, metadata string) {
	if r.blocks == nil {
		r.blocks = make(map[string]map[int32]*offsetCommitRequestBlock)
	}

	if r.blocks[topic] == nil {
		r.blocks[topic] = make(map[int32]*offsetCommitRequestBlock)
	}

	r.blocks[topic][partitionID] = &offsetCommitRequestBlock{offset, timestamp, metadata}
}
