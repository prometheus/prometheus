package sarama

type SyncGroupResponse struct {
	Err              KError
	MemberAssignment []byte
}

func (r *SyncGroupResponse) GetMemberAssignment() (*ConsumerGroupMemberAssignment, error) {
	assignment := new(ConsumerGroupMemberAssignment)
	err := decode(r.MemberAssignment, assignment)
	return assignment, err
}

func (r *SyncGroupResponse) encode(pe packetEncoder) error {
	pe.putInt16(int16(r.Err))
	return pe.putBytes(r.MemberAssignment)
}

func (r *SyncGroupResponse) decode(pd packetDecoder, version int16) (err error) {
	if kerr, err := pd.getInt16(); err != nil {
		return err
	} else {
		r.Err = KError(kerr)
	}

	r.MemberAssignment, err = pd.getBytes()
	return
}

func (r *SyncGroupResponse) key() int16 {
	return 14
}

func (r *SyncGroupResponse) version() int16 {
	return 0
}

func (r *SyncGroupResponse) requiredVersion() KafkaVersion {
	return V0_9_0_0
}
