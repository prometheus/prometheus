package sarama

import "time"

type partitionSet struct {
	msgs        []*ProducerMessage
	setToSend   *MessageSet
	bufferBytes int
}

type produceSet struct {
	parent *asyncProducer
	msgs   map[string]map[int32]*partitionSet

	bufferBytes int
	bufferCount int
}

func newProduceSet(parent *asyncProducer) *produceSet {
	return &produceSet{
		msgs:   make(map[string]map[int32]*partitionSet),
		parent: parent,
	}
}

func (ps *produceSet) add(msg *ProducerMessage) error {
	var err error
	var key, val []byte

	if msg.Key != nil {
		if key, err = msg.Key.Encode(); err != nil {
			return err
		}
	}

	if msg.Value != nil {
		if val, err = msg.Value.Encode(); err != nil {
			return err
		}
	}

	partitions := ps.msgs[msg.Topic]
	if partitions == nil {
		partitions = make(map[int32]*partitionSet)
		ps.msgs[msg.Topic] = partitions
	}

	set := partitions[msg.Partition]
	if set == nil {
		set = &partitionSet{setToSend: new(MessageSet)}
		partitions[msg.Partition] = set
	}

	set.msgs = append(set.msgs, msg)
	msgToSend := &Message{Codec: CompressionNone, Key: key, Value: val}
	if ps.parent.conf.Version.IsAtLeast(V0_10_0_0) {
		if msg.Timestamp.IsZero() {
			msgToSend.Timestamp = time.Now()
		} else {
			msgToSend.Timestamp = msg.Timestamp
		}
		msgToSend.Version = 1
	}
	set.setToSend.addMessage(msgToSend)

	size := producerMessageOverhead + len(key) + len(val)
	set.bufferBytes += size
	ps.bufferBytes += size
	ps.bufferCount++

	return nil
}

func (ps *produceSet) buildRequest() *ProduceRequest {
	req := &ProduceRequest{
		RequiredAcks: ps.parent.conf.Producer.RequiredAcks,
		Timeout:      int32(ps.parent.conf.Producer.Timeout / time.Millisecond),
	}
	if ps.parent.conf.Version.IsAtLeast(V0_10_0_0) {
		req.Version = 2
	}

	for topic, partitionSet := range ps.msgs {
		for partition, set := range partitionSet {
			if ps.parent.conf.Producer.Compression == CompressionNone {
				req.AddSet(topic, partition, set.setToSend)
			} else {
				// When compression is enabled, the entire set for each partition is compressed
				// and sent as the payload of a single fake "message" with the appropriate codec
				// set and no key. When the server sees a message with a compression codec, it
				// decompresses the payload and treats the result as its message set.
				payload, err := encode(set.setToSend, ps.parent.conf.MetricRegistry)
				if err != nil {
					Logger.Println(err) // if this happens, it's basically our fault.
					panic(err)
				}
				compMsg := &Message{
					Codec: ps.parent.conf.Producer.Compression,
					Key:   nil,
					Value: payload,
					Set:   set.setToSend, // Provide the underlying message set for accurate metrics
				}
				if ps.parent.conf.Version.IsAtLeast(V0_10_0_0) {
					compMsg.Version = 1
					compMsg.Timestamp = set.setToSend.Messages[0].Msg.Timestamp
				}
				req.AddMessage(topic, partition, compMsg)
			}
		}
	}

	return req
}

func (ps *produceSet) eachPartition(cb func(topic string, partition int32, msgs []*ProducerMessage)) {
	for topic, partitionSet := range ps.msgs {
		for partition, set := range partitionSet {
			cb(topic, partition, set.msgs)
		}
	}
}

func (ps *produceSet) dropPartition(topic string, partition int32) []*ProducerMessage {
	if ps.msgs[topic] == nil {
		return nil
	}
	set := ps.msgs[topic][partition]
	if set == nil {
		return nil
	}
	ps.bufferBytes -= set.bufferBytes
	ps.bufferCount -= len(set.msgs)
	delete(ps.msgs[topic], partition)
	return set.msgs
}

func (ps *produceSet) wouldOverflow(msg *ProducerMessage) bool {
	switch {
	// Would we overflow our maximum possible size-on-the-wire? 10KiB is arbitrary overhead for safety.
	case ps.bufferBytes+msg.byteSize() >= int(MaxRequestSize-(10*1024)):
		return true
	// Would we overflow the size-limit of a compressed message-batch for this partition?
	case ps.parent.conf.Producer.Compression != CompressionNone &&
		ps.msgs[msg.Topic] != nil && ps.msgs[msg.Topic][msg.Partition] != nil &&
		ps.msgs[msg.Topic][msg.Partition].bufferBytes+msg.byteSize() >= ps.parent.conf.Producer.MaxMessageBytes:
		return true
	// Would we overflow simply in number of messages?
	case ps.parent.conf.Producer.Flush.MaxMessages > 0 && ps.bufferCount >= ps.parent.conf.Producer.Flush.MaxMessages:
		return true
	default:
		return false
	}
}

func (ps *produceSet) readyToFlush() bool {
	switch {
	// If we don't have any messages, nothing else matters
	case ps.empty():
		return false
	// If all three config values are 0, we always flush as-fast-as-possible
	case ps.parent.conf.Producer.Flush.Frequency == 0 && ps.parent.conf.Producer.Flush.Bytes == 0 && ps.parent.conf.Producer.Flush.Messages == 0:
		return true
	// If we've passed the message trigger-point
	case ps.parent.conf.Producer.Flush.Messages > 0 && ps.bufferCount >= ps.parent.conf.Producer.Flush.Messages:
		return true
	// If we've passed the byte trigger-point
	case ps.parent.conf.Producer.Flush.Bytes > 0 && ps.bufferBytes >= ps.parent.conf.Producer.Flush.Bytes:
		return true
	default:
		return false
	}
}

func (ps *produceSet) empty() bool {
	return ps.bufferCount == 0
}
