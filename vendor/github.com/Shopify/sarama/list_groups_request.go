package sarama

type ListGroupsRequest struct {
}

func (r *ListGroupsRequest) encode(pe packetEncoder) error {
	return nil
}

func (r *ListGroupsRequest) decode(pd packetDecoder, version int16) (err error) {
	return nil
}

func (r *ListGroupsRequest) key() int16 {
	return 16
}

func (r *ListGroupsRequest) version() int16 {
	return 0
}

func (r *ListGroupsRequest) requiredVersion() KafkaVersion {
	return V0_9_0_0
}
