package sarama

import (
	"testing"
)

// MockResponse is a response builder interface it defines one method that
// allows generating a response based on a request body.
type MockResponse interface {
	For(reqBody decoder) (res encoder)
}

type mockWrapper struct {
	res encoder
}

func (mw *mockWrapper) For(reqBody decoder) (res encoder) {
	return mw.res
}

func newMockWrapper(res encoder) *mockWrapper {
	return &mockWrapper{res: res}
}

// mockMetadataResponse is a `MetadataResponse` builder.
type mockMetadataResponse struct {
	leaders map[string]map[int32]int32
	brokers map[string]int32
	t       *testing.T
}

func newMockMetadataResponse(t *testing.T) *mockMetadataResponse {
	return &mockMetadataResponse{
		leaders: make(map[string]map[int32]int32),
		brokers: make(map[string]int32),
		t:       t,
	}
}

func (mmr *mockMetadataResponse) SetLeader(topic string, partition, brokerID int32) *mockMetadataResponse {
	partitions := mmr.leaders[topic]
	if partitions == nil {
		partitions = make(map[int32]int32)
		mmr.leaders[topic] = partitions
	}
	partitions[partition] = brokerID
	return mmr
}

func (mmr *mockMetadataResponse) SetBroker(addr string, brokerID int32) *mockMetadataResponse {
	mmr.brokers[addr] = brokerID
	return mmr
}

func (mor *mockMetadataResponse) For(reqBody decoder) encoder {
	metadataRequest := reqBody.(*MetadataRequest)
	metadataResponse := &MetadataResponse{}
	for addr, brokerID := range mor.brokers {
		metadataResponse.AddBroker(addr, brokerID)
	}
	if len(metadataRequest.Topics) == 0 {
		for topic, partitions := range mor.leaders {
			for partition, brokerID := range partitions {
				metadataResponse.AddTopicPartition(topic, partition, brokerID, nil, nil, ErrNoError)
			}
		}
		return metadataResponse
	}
	for _, topic := range metadataRequest.Topics {
		for partition, brokerID := range mor.leaders[topic] {
			metadataResponse.AddTopicPartition(topic, partition, brokerID, nil, nil, ErrNoError)
		}
	}
	return metadataResponse
}

// mockOffsetResponse is an `OffsetResponse` builder.
type mockOffsetResponse struct {
	offsets map[string]map[int32]map[int64]int64
	t       *testing.T
}

func newMockOffsetResponse(t *testing.T) *mockOffsetResponse {
	return &mockOffsetResponse{
		offsets: make(map[string]map[int32]map[int64]int64),
		t:       t,
	}
}

func (mor *mockOffsetResponse) SetOffset(topic string, partition int32, time, offset int64) *mockOffsetResponse {
	partitions := mor.offsets[topic]
	if partitions == nil {
		partitions = make(map[int32]map[int64]int64)
		mor.offsets[topic] = partitions
	}
	times := partitions[partition]
	if times == nil {
		times = make(map[int64]int64)
		partitions[partition] = times
	}
	times[time] = offset
	return mor
}

func (mor *mockOffsetResponse) For(reqBody decoder) encoder {
	offsetRequest := reqBody.(*OffsetRequest)
	offsetResponse := &OffsetResponse{}
	for topic, partitions := range offsetRequest.blocks {
		for partition, block := range partitions {
			offset := mor.getOffset(topic, partition, block.time)
			offsetResponse.AddTopicPartition(topic, partition, offset)
		}
	}
	return offsetResponse
}

func (mor *mockOffsetResponse) getOffset(topic string, partition int32, time int64) int64 {
	partitions := mor.offsets[topic]
	if partitions == nil {
		mor.t.Errorf("missing topic: %s", topic)
	}
	times := partitions[partition]
	if times == nil {
		mor.t.Errorf("missing partition: %d", partition)
	}
	offset, ok := times[time]
	if !ok {
		mor.t.Errorf("missing time: %d", time)
	}
	return offset
}

// mockFetchResponse is a `FetchResponse` builder.
type mockFetchResponse struct {
	messages       map[string]map[int32]map[int64]Encoder
	highWaterMarks map[string]map[int32]int64
	t              *testing.T
	batchSize      int
}

func newMockFetchResponse(t *testing.T, batchSize int) *mockFetchResponse {
	return &mockFetchResponse{
		messages:       make(map[string]map[int32]map[int64]Encoder),
		highWaterMarks: make(map[string]map[int32]int64),
		t:              t,
		batchSize:      batchSize,
	}
}

func (mfr *mockFetchResponse) SetMessage(topic string, partition int32, offset int64, msg Encoder) *mockFetchResponse {
	partitions := mfr.messages[topic]
	if partitions == nil {
		partitions = make(map[int32]map[int64]Encoder)
		mfr.messages[topic] = partitions
	}
	messages := partitions[partition]
	if messages == nil {
		messages = make(map[int64]Encoder)
		partitions[partition] = messages
	}
	messages[offset] = msg
	return mfr
}

func (mfr *mockFetchResponse) SetHighWaterMark(topic string, partition int32, offset int64) *mockFetchResponse {
	partitions := mfr.highWaterMarks[topic]
	if partitions == nil {
		partitions = make(map[int32]int64)
		mfr.highWaterMarks[topic] = partitions
	}
	partitions[partition] = offset
	return mfr
}

func (mfr *mockFetchResponse) For(reqBody decoder) encoder {
	fetchRequest := reqBody.(*FetchRequest)
	res := &FetchResponse{}
	for topic, partitions := range fetchRequest.blocks {
		for partition, block := range partitions {
			initialOffset := block.fetchOffset
			offset := initialOffset
			maxOffset := initialOffset + int64(mfr.getMessageCount(topic, partition))
			for i := 0; i < mfr.batchSize && offset < maxOffset; {
				msg := mfr.getMessage(topic, partition, offset)
				if msg != nil {
					res.AddMessage(topic, partition, nil, msg, offset)
					i++
				}
				offset++
			}
			fb := res.GetBlock(topic, partition)
			if fb == nil {
				res.AddError(topic, partition, ErrNoError)
				fb = res.GetBlock(topic, partition)
			}
			fb.HighWaterMarkOffset = mfr.getHighWaterMark(topic, partition)
		}
	}
	return res
}

func (mfr *mockFetchResponse) getMessage(topic string, partition int32, offset int64) Encoder {
	partitions := mfr.messages[topic]
	if partitions == nil {
		return nil
	}
	messages := partitions[partition]
	if messages == nil {
		return nil
	}
	return messages[offset]
}

func (mfr *mockFetchResponse) getMessageCount(topic string, partition int32) int {
	partitions := mfr.messages[topic]
	if partitions == nil {
		return 0
	}
	messages := partitions[partition]
	if messages == nil {
		return 0
	}
	return len(messages)
}

func (mfr *mockFetchResponse) getHighWaterMark(topic string, partition int32) int64 {
	partitions := mfr.highWaterMarks[topic]
	if partitions == nil {
		return 0
	}
	return partitions[partition]
}

// mockConsumerMetadataResponse is a `ConsumerMetadataResponse` builder.
type mockConsumerMetadataResponse struct {
	coordinators map[string]interface{}
	t            *testing.T
}

func newMockConsumerMetadataResponse(t *testing.T) *mockConsumerMetadataResponse {
	return &mockConsumerMetadataResponse{
		coordinators: make(map[string]interface{}),
		t:            t,
	}
}

func (mr *mockConsumerMetadataResponse) SetCoordinator(group string, broker *mockBroker) *mockConsumerMetadataResponse {
	mr.coordinators[group] = broker
	return mr
}

func (mr *mockConsumerMetadataResponse) SetError(group string, kerror KError) *mockConsumerMetadataResponse {
	mr.coordinators[group] = kerror
	return mr
}

func (mr *mockConsumerMetadataResponse) For(reqBody decoder) encoder {
	req := reqBody.(*ConsumerMetadataRequest)
	group := req.ConsumerGroup
	res := &ConsumerMetadataResponse{}
	v := mr.coordinators[group]
	switch v := v.(type) {
	case *mockBroker:
		res.Coordinator = &Broker{id: v.BrokerID(), addr: v.Addr()}
	case KError:
		res.Err = v
	}
	return res
}

// mockOffsetCommitResponse is a `OffsetCommitResponse` builder.
type mockOffsetCommitResponse struct {
	errors map[string]map[string]map[int32]KError
	t      *testing.T
}

func newMockOffsetCommitResponse(t *testing.T) *mockOffsetCommitResponse {
	return &mockOffsetCommitResponse{t: t}
}

func (mr *mockOffsetCommitResponse) SetError(group, topic string, partition int32, kerror KError) *mockOffsetCommitResponse {
	if mr.errors == nil {
		mr.errors = make(map[string]map[string]map[int32]KError)
	}
	topics := mr.errors[group]
	if topics == nil {
		topics = make(map[string]map[int32]KError)
		mr.errors[group] = topics
	}
	partitions := topics[topic]
	if partitions == nil {
		partitions = make(map[int32]KError)
		topics[topic] = partitions
	}
	partitions[partition] = kerror
	return mr
}

func (mr *mockOffsetCommitResponse) For(reqBody decoder) encoder {
	req := reqBody.(*OffsetCommitRequest)
	group := req.ConsumerGroup
	res := &OffsetCommitResponse{}
	for topic, partitions := range req.blocks {
		for partition := range partitions {
			res.AddError(topic, partition, mr.getError(group, topic, partition))
		}
	}
	return res
}

func (mr *mockOffsetCommitResponse) getError(group, topic string, partition int32) KError {
	topics := mr.errors[group]
	if topics == nil {
		return ErrNoError
	}
	partitions := topics[topic]
	if partitions == nil {
		return ErrNoError
	}
	kerror, ok := partitions[partition]
	if !ok {
		return ErrNoError
	}
	return kerror
}

// mockProduceResponse is a `ProduceResponse` builder.
type mockProduceResponse struct {
	errors map[string]map[int32]KError
	t      *testing.T
}

func newMockProduceResponse(t *testing.T) *mockProduceResponse {
	return &mockProduceResponse{t: t}
}

func (mr *mockProduceResponse) SetError(topic string, partition int32, kerror KError) *mockProduceResponse {
	if mr.errors == nil {
		mr.errors = make(map[string]map[int32]KError)
	}
	partitions := mr.errors[topic]
	if partitions == nil {
		partitions = make(map[int32]KError)
		mr.errors[topic] = partitions
	}
	partitions[partition] = kerror
	return mr
}

func (mr *mockProduceResponse) For(reqBody decoder) encoder {
	req := reqBody.(*ProduceRequest)
	res := &ProduceResponse{}
	for topic, partitions := range req.msgSets {
		for partition := range partitions {
			res.AddTopicPartition(topic, partition, mr.getError(topic, partition))
		}
	}
	return res
}

func (mr *mockProduceResponse) getError(topic string, partition int32) KError {
	partitions := mr.errors[topic]
	if partitions == nil {
		return ErrNoError
	}
	kerror, ok := partitions[partition]
	if !ok {
		return ErrNoError
	}
	return kerror
}

// mockOffsetFetchResponse is a `OffsetFetchResponse` builder.
type mockOffsetFetchResponse struct {
	offsets map[string]map[string]map[int32]*OffsetFetchResponseBlock
	t       *testing.T
}

func newMockOffsetFetchResponse(t *testing.T) *mockOffsetFetchResponse {
	return &mockOffsetFetchResponse{t: t}
}

func (mr *mockOffsetFetchResponse) SetOffset(group, topic string, partition int32, offset int64, metadata string, kerror KError) *mockOffsetFetchResponse {
	if mr.offsets == nil {
		mr.offsets = make(map[string]map[string]map[int32]*OffsetFetchResponseBlock)
	}
	topics := mr.offsets[group]
	if topics == nil {
		topics = make(map[string]map[int32]*OffsetFetchResponseBlock)
		mr.offsets[group] = topics
	}
	partitions := topics[topic]
	if partitions == nil {
		partitions = make(map[int32]*OffsetFetchResponseBlock)
		topics[topic] = partitions
	}
	partitions[partition] = &OffsetFetchResponseBlock{offset, metadata, kerror}
	return mr
}

func (mr *mockOffsetFetchResponse) For(reqBody decoder) encoder {
	req := reqBody.(*OffsetFetchRequest)
	group := req.ConsumerGroup
	res := &OffsetFetchResponse{}
	for topic, partitions := range mr.offsets[group] {
		for partition, block := range partitions {
			res.AddBlock(topic, partition, block)
		}
	}
	return res
}
