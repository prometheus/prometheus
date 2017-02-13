package sarama

type ListGroupsResponse struct {
	Err    KError
	Groups map[string]string
}

func (r *ListGroupsResponse) encode(pe packetEncoder) error {
	pe.putInt16(int16(r.Err))

	if err := pe.putArrayLength(len(r.Groups)); err != nil {
		return err
	}
	for groupId, protocolType := range r.Groups {
		if err := pe.putString(groupId); err != nil {
			return err
		}
		if err := pe.putString(protocolType); err != nil {
			return err
		}
	}

	return nil
}

func (r *ListGroupsResponse) decode(pd packetDecoder, version int16) error {
	if kerr, err := pd.getInt16(); err != nil {
		return err
	} else {
		r.Err = KError(kerr)
	}

	n, err := pd.getArrayLength()
	if err != nil {
		return err
	}
	if n == 0 {
		return nil
	}

	r.Groups = make(map[string]string)
	for i := 0; i < n; i++ {
		groupId, err := pd.getString()
		if err != nil {
			return err
		}
		protocolType, err := pd.getString()
		if err != nil {
			return err
		}

		r.Groups[groupId] = protocolType
	}

	return nil
}

func (r *ListGroupsResponse) key() int16 {
	return 16
}

func (r *ListGroupsResponse) version() int16 {
	return 0
}

func (r *ListGroupsResponse) requiredVersion() KafkaVersion {
	return V0_9_0_0
}
