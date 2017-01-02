package sarama

import (
	"fmt"
)

// TestReporter has methods matching go's testing.T to avoid importing
// `testing` in the main part of the library.
type TestReporter interface {
	Error(...interface{})
	Errorf(string, ...interface{})
	Fatal(...interface{})
	Fatalf(string, ...interface{})
}

// MockResponse is a response builder interface it defines one method that
// allows generating a response based on a request body. MockResponses are used
// to program behavior of MockBroker in tests.
type MockResponse interface {
	For(reqBody versionedDecoder) (res encoder)
}

// MockWrapper is a mock response builder that returns a particular concrete
// response regardless of the actual request passed to the `For` method.
type MockWrapper struct {
	res encoder
}

func (mw *MockWrapper) For(reqBody versionedDecoder) (res encoder) {
	return mw.res
}

func NewMockWrapper(res encoder) *MockWrapper {
	return &MockWrapper{res: res}
}

// MockSequence is a mock response builder that is created from a sequence of
// concrete responses. Every time when a `MockBroker` calls its `For` method
// the next response from the sequence is returned. When the end of the
// sequence is reached the last element from the sequence is returned.
type MockSequence struct {
	responses []MockResponse
}

func NewMockSequence(responses ...interface{}) *MockSequence {
	ms := &MockSequence{}
	ms.responses = make([]MockResponse, len(responses))
	for i, res := range responses {
		switch res := res.(type) {
		case MockResponse:
			ms.responses[i] = res
		case encoder:
			ms.responses[i] = NewMockWrapper(res)
		default:
			panic(fmt.Sprintf("Unexpected response type: %T", res))
		}
	}
	return ms
}

func (mc *MockSequence) For(reqBody versionedDecoder) (res encoder) {
	res = mc.responses[0].For(reqBody)
	if len(mc.responses) > 1 {
		mc.responses = mc.responses[1:]
	}
	return res
}

// MockMetadataResponse is a `MetadataResponse` builder.
type MockMetadataResponse struct {
	leaders map[string]map[int32]int32
	brokers map[string]int32
	t       TestReporter
}

func NewMockMetadataResponse(t TestReporter) *MockMetadataResponse {
	return &MockMetadataResponse{
		leaders: make(map[string]map[int32]int32),
		brokers: make(map[string]int32),
		t:       t,
	}
}

func (mmr *MockMetadataResponse) SetLeader(topic string, partition, brokerID int32) *MockMetadataResponse {
	partitions := mmr.leaders[topic]
	if partitions == nil {
		partitions = make(map[int32]int32)
		mmr.leaders[topic] = partitions
	}
	partitions[partition] = brokerID
	return mmr
}

func (mmr *MockMetadataResponse) SetBroker(addr string, brokerID int32) *MockMetadataResponse {
	mmr.brokers[addr] = brokerID
	return mmr
}

func (mmr *MockMetadataResponse) For(reqBody versionedDecoder) encoder {
	metadataRequest := reqBody.(*MetadataRequest)
	metadataResponse := &MetadataResponse{}
	for addr, brokerID := range mmr.brokers {
		metadataResponse.AddBroker(addr, brokerID)
	}
	if len(metadataRequest.Topics) == 0 {
		for topic, partitions := range mmr.leaders {
			for partition, brokerID := range partitions {
				metadataResponse.AddTopicPartition(topic, partition, brokerID, nil, nil, ErrNoError)
			}
		}
		return metadataResponse
	}
	for _, topic := range metadataRequest.Topics {
		for partition, brokerID := range mmr.leaders[topic] {
			metadataResponse.AddTopicPartition(topic, partition, brokerID, nil, nil, ErrNoError)
		}
	}
	return metadataResponse
}

// MockOffsetResponse is an `OffsetResponse` builder.
type MockOffsetResponse struct {
	offsets map[string]map[int32]map[int64]int64
	t       TestReporter
}

func NewMockOffsetResponse(t TestReporter) *MockOffsetResponse {
	return &MockOffsetResponse{
		offsets: make(map[string]map[int32]map[int64]int64),
		t:       t,
	}
}

func (mor *MockOffsetResponse) SetOffset(topic string, partition int32, time, offset int64) *MockOffsetResponse {
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

func (mor *MockOffsetResponse) For(reqBody versionedDecoder) encoder {
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

func (mor *MockOffsetResponse) getOffset(topic string, partition int32, time int64) int64 {
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

// MockFetchResponse is a `FetchResponse` builder.
type MockFetchResponse struct {
	messages       map[string]map[int32]map[int64]Encoder
	highWaterMarks map[string]map[int32]int64
	t              TestReporter
	batchSize      int
}

func NewMockFetchResponse(t TestReporter, batchSize int) *MockFetchResponse {
	return &MockFetchResponse{
		messages:       make(map[string]map[int32]map[int64]Encoder),
		highWaterMarks: make(map[string]map[int32]int64),
		t:              t,
		batchSize:      batchSize,
	}
}

func (mfr *MockFetchResponse) SetMessage(topic string, partition int32, offset int64, msg Encoder) *MockFetchResponse {
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

func (mfr *MockFetchResponse) SetHighWaterMark(topic string, partition int32, offset int64) *MockFetchResponse {
	partitions := mfr.highWaterMarks[topic]
	if partitions == nil {
		partitions = make(map[int32]int64)
		mfr.highWaterMarks[topic] = partitions
	}
	partitions[partition] = offset
	return mfr
}

func (mfr *MockFetchResponse) For(reqBody versionedDecoder) encoder {
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

func (mfr *MockFetchResponse) getMessage(topic string, partition int32, offset int64) Encoder {
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

func (mfr *MockFetchResponse) getMessageCount(topic string, partition int32) int {
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

func (mfr *MockFetchResponse) getHighWaterMark(topic string, partition int32) int64 {
	partitions := mfr.highWaterMarks[topic]
	if partitions == nil {
		return 0
	}
	return partitions[partition]
}

// MockConsumerMetadataResponse is a `ConsumerMetadataResponse` builder.
type MockConsumerMetadataResponse struct {
	coordinators map[string]interface{}
	t            TestReporter
}

func NewMockConsumerMetadataResponse(t TestReporter) *MockConsumerMetadataResponse {
	return &MockConsumerMetadataResponse{
		coordinators: make(map[string]interface{}),
		t:            t,
	}
}

func (mr *MockConsumerMetadataResponse) SetCoordinator(group string, broker *MockBroker) *MockConsumerMetadataResponse {
	mr.coordinators[group] = broker
	return mr
}

func (mr *MockConsumerMetadataResponse) SetError(group string, kerror KError) *MockConsumerMetadataResponse {
	mr.coordinators[group] = kerror
	return mr
}

func (mr *MockConsumerMetadataResponse) For(reqBody versionedDecoder) encoder {
	req := reqBody.(*ConsumerMetadataRequest)
	group := req.ConsumerGroup
	res := &ConsumerMetadataResponse{}
	v := mr.coordinators[group]
	switch v := v.(type) {
	case *MockBroker:
		res.Coordinator = &Broker{id: v.BrokerID(), addr: v.Addr()}
	case KError:
		res.Err = v
	}
	return res
}

// MockOffsetCommitResponse is a `OffsetCommitResponse` builder.
type MockOffsetCommitResponse struct {
	errors map[string]map[string]map[int32]KError
	t      TestReporter
}

func NewMockOffsetCommitResponse(t TestReporter) *MockOffsetCommitResponse {
	return &MockOffsetCommitResponse{t: t}
}

func (mr *MockOffsetCommitResponse) SetError(group, topic string, partition int32, kerror KError) *MockOffsetCommitResponse {
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

func (mr *MockOffsetCommitResponse) For(reqBody versionedDecoder) encoder {
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

func (mr *MockOffsetCommitResponse) getError(group, topic string, partition int32) KError {
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

// MockProduceResponse is a `ProduceResponse` builder.
type MockProduceResponse struct {
	errors map[string]map[int32]KError
	t      TestReporter
}

func NewMockProduceResponse(t TestReporter) *MockProduceResponse {
	return &MockProduceResponse{t: t}
}

func (mr *MockProduceResponse) SetError(topic string, partition int32, kerror KError) *MockProduceResponse {
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

func (mr *MockProduceResponse) For(reqBody versionedDecoder) encoder {
	req := reqBody.(*ProduceRequest)
	res := &ProduceResponse{}
	for topic, partitions := range req.msgSets {
		for partition := range partitions {
			res.AddTopicPartition(topic, partition, mr.getError(topic, partition))
		}
	}
	return res
}

func (mr *MockProduceResponse) getError(topic string, partition int32) KError {
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

// MockOffsetFetchResponse is a `OffsetFetchResponse` builder.
type MockOffsetFetchResponse struct {
	offsets map[string]map[string]map[int32]*OffsetFetchResponseBlock
	t       TestReporter
}

func NewMockOffsetFetchResponse(t TestReporter) *MockOffsetFetchResponse {
	return &MockOffsetFetchResponse{t: t}
}

func (mr *MockOffsetFetchResponse) SetOffset(group, topic string, partition int32, offset int64, metadata string, kerror KError) *MockOffsetFetchResponse {
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

func (mr *MockOffsetFetchResponse) For(reqBody versionedDecoder) encoder {
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
