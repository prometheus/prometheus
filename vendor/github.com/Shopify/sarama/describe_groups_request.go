package sarama

type DescribeGroupsRequest struct {
	Groups []string
}

func (r *DescribeGroupsRequest) encode(pe packetEncoder) error {
	return pe.putStringArray(r.Groups)
}

func (r *DescribeGroupsRequest) decode(pd packetDecoder, version int16) (err error) {
	r.Groups, err = pd.getStringArray()
	return
}

func (r *DescribeGroupsRequest) key() int16 {
	return 15
}

func (r *DescribeGroupsRequest) version() int16 {
	return 0
}

func (r *DescribeGroupsRequest) requiredVersion() KafkaVersion {
	return V0_9_0_0
}

func (r *DescribeGroupsRequest) AddGroup(group string) {
	r.Groups = append(r.Groups, group)
}
