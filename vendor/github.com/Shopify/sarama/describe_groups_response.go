package sarama

type DescribeGroupsResponse struct {
	Groups []*GroupDescription
}

func (r *DescribeGroupsResponse) encode(pe packetEncoder) error {
	if err := pe.putArrayLength(len(r.Groups)); err != nil {
		return err
	}

	for _, groupDescription := range r.Groups {
		if err := groupDescription.encode(pe); err != nil {
			return err
		}
	}

	return nil
}

func (r *DescribeGroupsResponse) decode(pd packetDecoder, version int16) (err error) {
	n, err := pd.getArrayLength()
	if err != nil {
		return err
	}

	r.Groups = make([]*GroupDescription, n)
	for i := 0; i < n; i++ {
		r.Groups[i] = new(GroupDescription)
		if err := r.Groups[i].decode(pd); err != nil {
			return err
		}
	}

	return nil
}

func (r *DescribeGroupsResponse) key() int16 {
	return 15
}

func (r *DescribeGroupsResponse) version() int16 {
	return 0
}

func (r *DescribeGroupsResponse) requiredVersion() KafkaVersion {
	return V0_9_0_0
}

type GroupDescription struct {
	Err          KError
	GroupId      string
	State        string
	ProtocolType string
	Protocol     string
	Members      map[string]*GroupMemberDescription
}

func (gd *GroupDescription) encode(pe packetEncoder) error {
	pe.putInt16(int16(gd.Err))

	if err := pe.putString(gd.GroupId); err != nil {
		return err
	}
	if err := pe.putString(gd.State); err != nil {
		return err
	}
	if err := pe.putString(gd.ProtocolType); err != nil {
		return err
	}
	if err := pe.putString(gd.Protocol); err != nil {
		return err
	}

	if err := pe.putArrayLength(len(gd.Members)); err != nil {
		return err
	}

	for memberId, groupMemberDescription := range gd.Members {
		if err := pe.putString(memberId); err != nil {
			return err
		}
		if err := groupMemberDescription.encode(pe); err != nil {
			return err
		}
	}

	return nil
}

func (gd *GroupDescription) decode(pd packetDecoder) (err error) {
	if kerr, err := pd.getInt16(); err != nil {
		return err
	} else {
		gd.Err = KError(kerr)
	}

	if gd.GroupId, err = pd.getString(); err != nil {
		return
	}
	if gd.State, err = pd.getString(); err != nil {
		return
	}
	if gd.ProtocolType, err = pd.getString(); err != nil {
		return
	}
	if gd.Protocol, err = pd.getString(); err != nil {
		return
	}

	n, err := pd.getArrayLength()
	if err != nil {
		return err
	}
	if n == 0 {
		return nil
	}

	gd.Members = make(map[string]*GroupMemberDescription)
	for i := 0; i < n; i++ {
		memberId, err := pd.getString()
		if err != nil {
			return err
		}

		gd.Members[memberId] = new(GroupMemberDescription)
		if err := gd.Members[memberId].decode(pd); err != nil {
			return err
		}
	}

	return nil
}

type GroupMemberDescription struct {
	ClientId         string
	ClientHost       string
	MemberMetadata   []byte
	MemberAssignment []byte
}

func (gmd *GroupMemberDescription) encode(pe packetEncoder) error {
	if err := pe.putString(gmd.ClientId); err != nil {
		return err
	}
	if err := pe.putString(gmd.ClientHost); err != nil {
		return err
	}
	if err := pe.putBytes(gmd.MemberMetadata); err != nil {
		return err
	}
	if err := pe.putBytes(gmd.MemberAssignment); err != nil {
		return err
	}

	return nil
}

func (gmd *GroupMemberDescription) decode(pd packetDecoder) (err error) {
	if gmd.ClientId, err = pd.getString(); err != nil {
		return
	}
	if gmd.ClientHost, err = pd.getString(); err != nil {
		return
	}
	if gmd.MemberMetadata, err = pd.getBytes(); err != nil {
		return
	}
	if gmd.MemberAssignment, err = pd.getBytes(); err != nil {
		return
	}

	return nil
}

func (gmd *GroupMemberDescription) GetMemberAssignment() (*ConsumerGroupMemberAssignment, error) {
	assignment := new(ConsumerGroupMemberAssignment)
	err := decode(gmd.MemberAssignment, assignment)
	return assignment, err
}

func (gmd *GroupMemberDescription) GetMemberMetadata() (*ConsumerGroupMemberMetadata, error) {
	metadata := new(ConsumerGroupMemberMetadata)
	err := decode(gmd.MemberMetadata, metadata)
	return metadata, err
}
