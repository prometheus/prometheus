package nas

type DescribeMountTargetsRequest struct {
	FileSystemName string
	Version        string
	RegionId       string
}

type DescribeMountTargetsResponse struct {
	MountTargets []MountTarget
	Code         string
}

type MountTarget struct {
	AccessGroupName string
	MountTargetIp   string
	NetworkType     string
	Status          string
	MountTargetId   string
	VpcId           string
	VSwitchId       string
	DomainName      string
	CloudInstId     string
}

func (client *Client) DescribeMountTargets(args *DescribeMountTargetsRequest) (resp DescribeMountTargetsResponse, err error) {
	response := DescribeMountTargetsResponse{}
	args.Version = VERSION
	err = client.Invoke("DescribeMountTargets", args, &response)
	return response, err
}
