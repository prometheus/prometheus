package sarama

type HeartbeatResponse struct {
	Err KError
}

func (r *HeartbeatResponse) encode(pe packetEncoder) error {
	pe.putInt16(int16(r.Err))
	return nil
}

func (r *HeartbeatResponse) decode(pd packetDecoder, version int16) error {
	if kerr, err := pd.getInt16(); err != nil {
		return err
	} else {
		r.Err = KError(kerr)
	}

	return nil
}

func (r *HeartbeatResponse) key() int16 {
	return 12
}

func (r *HeartbeatResponse) version() int16 {
	return 0
}

func (r *HeartbeatResponse) requiredVersion() KafkaVersion {
	return V0_9_0_0
}
