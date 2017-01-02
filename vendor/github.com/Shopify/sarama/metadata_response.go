package sarama

type PartitionMetadata struct {
	Err      KError
	ID       int32
	Leader   int32
	Replicas []int32
	Isr      []int32
}

func (pm *PartitionMetadata) decode(pd packetDecoder) (err error) {
	tmp, err := pd.getInt16()
	if err != nil {
		return err
	}
	pm.Err = KError(tmp)

	pm.ID, err = pd.getInt32()
	if err != nil {
		return err
	}

	pm.Leader, err = pd.getInt32()
	if err != nil {
		return err
	}

	pm.Replicas, err = pd.getInt32Array()
	if err != nil {
		return err
	}

	pm.Isr, err = pd.getInt32Array()
	if err != nil {
		return err
	}

	return nil
}

func (pm *PartitionMetadata) encode(pe packetEncoder) (err error) {
	pe.putInt16(int16(pm.Err))
	pe.putInt32(pm.ID)
	pe.putInt32(pm.Leader)

	err = pe.putInt32Array(pm.Replicas)
	if err != nil {
		return err
	}

	err = pe.putInt32Array(pm.Isr)
	if err != nil {
		return err
	}

	return nil
}

type TopicMetadata struct {
	Err        KError
	Name       string
	Partitions []*PartitionMetadata
}

func (tm *TopicMetadata) decode(pd packetDecoder) (err error) {
	tmp, err := pd.getInt16()
	if err != nil {
		return err
	}
	tm.Err = KError(tmp)

	tm.Name, err = pd.getString()
	if err != nil {
		return err
	}

	n, err := pd.getArrayLength()
	if err != nil {
		return err
	}
	tm.Partitions = make([]*PartitionMetadata, n)
	for i := 0; i < n; i++ {
		tm.Partitions[i] = new(PartitionMetadata)
		err = tm.Partitions[i].decode(pd)
		if err != nil {
			return err
		}
	}

	return nil
}

func (tm *TopicMetadata) encode(pe packetEncoder) (err error) {
	pe.putInt16(int16(tm.Err))

	err = pe.putString(tm.Name)
	if err != nil {
		return err
	}

	err = pe.putArrayLength(len(tm.Partitions))
	if err != nil {
		return err
	}

	for _, pm := range tm.Partitions {
		err = pm.encode(pe)
		if err != nil {
			return err
		}
	}

	return nil
}

type MetadataResponse struct {
	Brokers []*Broker
	Topics  []*TopicMetadata
}

func (r *MetadataResponse) decode(pd packetDecoder, version int16) (err error) {
	n, err := pd.getArrayLength()
	if err != nil {
		return err
	}

	r.Brokers = make([]*Broker, n)
	for i := 0; i < n; i++ {
		r.Brokers[i] = new(Broker)
		err = r.Brokers[i].decode(pd)
		if err != nil {
			return err
		}
	}

	n, err = pd.getArrayLength()
	if err != nil {
		return err
	}

	r.Topics = make([]*TopicMetadata, n)
	for i := 0; i < n; i++ {
		r.Topics[i] = new(TopicMetadata)
		err = r.Topics[i].decode(pd)
		if err != nil {
			return err
		}
	}

	return nil
}

func (r *MetadataResponse) encode(pe packetEncoder) error {
	err := pe.putArrayLength(len(r.Brokers))
	if err != nil {
		return err
	}
	for _, broker := range r.Brokers {
		err = broker.encode(pe)
		if err != nil {
			return err
		}
	}

	err = pe.putArrayLength(len(r.Topics))
	if err != nil {
		return err
	}
	for _, tm := range r.Topics {
		err = tm.encode(pe)
		if err != nil {
			return err
		}
	}

	return nil
}

func (r *MetadataResponse) key() int16 {
	return 3
}

func (r *MetadataResponse) version() int16 {
	return 0
}

func (r *MetadataResponse) requiredVersion() KafkaVersion {
	return minVersion
}

// testing API

func (r *MetadataResponse) AddBroker(addr string, id int32) {
	r.Brokers = append(r.Brokers, &Broker{id: id, addr: addr})
}

func (r *MetadataResponse) AddTopic(topic string, err KError) *TopicMetadata {
	var tmatch *TopicMetadata

	for _, tm := range r.Topics {
		if tm.Name == topic {
			tmatch = tm
			goto foundTopic
		}
	}

	tmatch = new(TopicMetadata)
	tmatch.Name = topic
	r.Topics = append(r.Topics, tmatch)

foundTopic:

	tmatch.Err = err
	return tmatch
}

func (r *MetadataResponse) AddTopicPartition(topic string, partition, brokerID int32, replicas, isr []int32, err KError) {
	tmatch := r.AddTopic(topic, ErrNoError)
	var pmatch *PartitionMetadata

	for _, pm := range tmatch.Partitions {
		if pm.ID == partition {
			pmatch = pm
			goto foundPartition
		}
	}

	pmatch = new(PartitionMetadata)
	pmatch.ID = partition
	tmatch.Partitions = append(tmatch.Partitions, pmatch)

foundPartition:

	pmatch.Leader = brokerID
	pmatch.Replicas = replicas
	pmatch.Isr = isr
	pmatch.Err = err

}
