package sarama

// RequiredAcks is used in Produce Requests to tell the broker how many replica acknowledgements
// it must see before responding. Any of the constants defined here are valid. On broker versions
// prior to 0.8.2.0 any other positive int16 is also valid (the broker will wait for that many
// acknowledgements) but in 0.8.2.0 and later this will raise an exception (it has been replaced
// by setting the `min.isr` value in the brokers configuration).
type RequiredAcks int16

const (
	// NoResponse doesn't send any response, the TCP ACK is all you get.
	NoResponse RequiredAcks = 0
	// WaitForLocal waits for only the local commit to succeed before responding.
	WaitForLocal RequiredAcks = 1
	// WaitForAll waits for all replicas to commit before responding.
	WaitForAll RequiredAcks = -1
)

type ProduceRequest struct {
	RequiredAcks RequiredAcks
	Timeout      int32
	msgSets      map[string]map[int32]*MessageSet
}

func (p *ProduceRequest) encode(pe packetEncoder) error {
	pe.putInt16(int16(p.RequiredAcks))
	pe.putInt32(p.Timeout)
	err := pe.putArrayLength(len(p.msgSets))
	if err != nil {
		return err
	}
	for topic, partitions := range p.msgSets {
		err = pe.putString(topic)
		if err != nil {
			return err
		}
		err = pe.putArrayLength(len(partitions))
		if err != nil {
			return err
		}
		for id, msgSet := range partitions {
			pe.putInt32(id)
			pe.push(&lengthField{})
			err = msgSet.encode(pe)
			if err != nil {
				return err
			}
			err = pe.pop()
			if err != nil {
				return err
			}
		}
	}
	return nil
}

func (p *ProduceRequest) decode(pd packetDecoder) error {
	requiredAcks, err := pd.getInt16()
	if err != nil {
		return err
	}
	p.RequiredAcks = RequiredAcks(requiredAcks)
	if p.Timeout, err = pd.getInt32(); err != nil {
		return err
	}
	topicCount, err := pd.getArrayLength()
	if err != nil {
		return err
	}
	if topicCount == 0 {
		return nil
	}
	p.msgSets = make(map[string]map[int32]*MessageSet)
	for i := 0; i < topicCount; i++ {
		topic, err := pd.getString()
		if err != nil {
			return err
		}
		partitionCount, err := pd.getArrayLength()
		if err != nil {
			return err
		}
		p.msgSets[topic] = make(map[int32]*MessageSet)
		for j := 0; j < partitionCount; j++ {
			partition, err := pd.getInt32()
			if err != nil {
				return err
			}
			messageSetSize, err := pd.getInt32()
			if err != nil {
				return err
			}
			msgSetDecoder, err := pd.getSubset(int(messageSetSize))
			if err != nil {
				return err
			}
			msgSet := &MessageSet{}
			err = msgSet.decode(msgSetDecoder)
			if err != nil {
				return err
			}
			p.msgSets[topic][partition] = msgSet
		}
	}
	return nil
}

func (p *ProduceRequest) key() int16 {
	return 0
}

func (p *ProduceRequest) version() int16 {
	return 0
}

func (p *ProduceRequest) AddMessage(topic string, partition int32, msg *Message) {
	if p.msgSets == nil {
		p.msgSets = make(map[string]map[int32]*MessageSet)
	}

	if p.msgSets[topic] == nil {
		p.msgSets[topic] = make(map[int32]*MessageSet)
	}

	set := p.msgSets[topic][partition]

	if set == nil {
		set = new(MessageSet)
		p.msgSets[topic][partition] = set
	}

	set.addMessage(msg)
}

func (p *ProduceRequest) AddSet(topic string, partition int32, set *MessageSet) {
	if p.msgSets == nil {
		p.msgSets = make(map[string]map[int32]*MessageSet)
	}

	if p.msgSets[topic] == nil {
		p.msgSets[topic] = make(map[int32]*MessageSet)
	}

	p.msgSets[topic][partition] = set
}
