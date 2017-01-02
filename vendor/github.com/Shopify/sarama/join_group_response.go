package sarama

type JoinGroupResponse struct {
	Err           KError
	GenerationId  int32
	GroupProtocol string
	LeaderId      string
	MemberId      string
	Members       map[string][]byte
}

func (r *JoinGroupResponse) GetMembers() (map[string]ConsumerGroupMemberMetadata, error) {
	members := make(map[string]ConsumerGroupMemberMetadata, len(r.Members))
	for id, bin := range r.Members {
		meta := new(ConsumerGroupMemberMetadata)
		if err := decode(bin, meta); err != nil {
			return nil, err
		}
		members[id] = *meta
	}
	return members, nil
}

func (r *JoinGroupResponse) encode(pe packetEncoder) error {
	pe.putInt16(int16(r.Err))
	pe.putInt32(r.GenerationId)

	if err := pe.putString(r.GroupProtocol); err != nil {
		return err
	}
	if err := pe.putString(r.LeaderId); err != nil {
		return err
	}
	if err := pe.putString(r.MemberId); err != nil {
		return err
	}

	if err := pe.putArrayLength(len(r.Members)); err != nil {
		return err
	}

	for memberId, memberMetadata := range r.Members {
		if err := pe.putString(memberId); err != nil {
			return err
		}

		if err := pe.putBytes(memberMetadata); err != nil {
			return err
		}
	}

	return nil
}

func (r *JoinGroupResponse) decode(pd packetDecoder, version int16) (err error) {
	if kerr, err := pd.getInt16(); err != nil {
		return err
	} else {
		r.Err = KError(kerr)
	}

	if r.GenerationId, err = pd.getInt32(); err != nil {
		return
	}

	if r.GroupProtocol, err = pd.getString(); err != nil {
		return
	}

	if r.LeaderId, err = pd.getString(); err != nil {
		return
	}

	if r.MemberId, err = pd.getString(); err != nil {
		return
	}

	n, err := pd.getArrayLength()
	if err != nil {
		return err
	}
	if n == 0 {
		return nil
	}

	r.Members = make(map[string][]byte)
	for i := 0; i < n; i++ {
		memberId, err := pd.getString()
		if err != nil {
			return err
		}

		memberMetadata, err := pd.getBytes()
		if err != nil {
			return err
		}

		r.Members[memberId] = memberMetadata
	}

	return nil
}

func (r *JoinGroupResponse) key() int16 {
	return 11
}

func (r *JoinGroupResponse) version() int16 {
	return 0
}

func (r *JoinGroupResponse) requiredVersion() KafkaVersion {
	return V0_9_0_0
}
