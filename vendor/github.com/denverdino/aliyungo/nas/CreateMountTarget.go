package nas

type CreateMountTargetRequest struct {
	FileSystemName  string
	AccessGroupName string
	NetworkType     string
	VpcId           string
	VSwitchId       string
	Version         string
	RegionId        string
}

type CreateMountTargetResponse struct {
	Code string
}

func (client *Client) CreateMountTarget(args *CreateMountTargetRequest) (resp CreateMountTargetResponse, err error) {
	response := CreateMountTargetResponse{}
	args.Version = VERSION

	err = client.Invoke("CreateMountTarget", args, &response)
	return response, err
}
