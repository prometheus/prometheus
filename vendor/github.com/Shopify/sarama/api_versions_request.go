package sarama

type ApiVersionsRequest struct {
}

func (r *ApiVersionsRequest) encode(pe packetEncoder) error {
	return nil
}

func (r *ApiVersionsRequest) decode(pd packetDecoder, version int16) (err error) {
	return nil
}

func (r *ApiVersionsRequest) key() int16 {
	return 18
}

func (r *ApiVersionsRequest) version() int16 {
	return 0
}

func (r *ApiVersionsRequest) requiredVersion() KafkaVersion {
	return V0_10_0_0
}
