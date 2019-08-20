package nas

type CreateFileSystemRequest struct {
	ZoneId   string
	Version  string
	RegionId string
}

type CreateFileSystemResponse struct {
	FileSystemName string
	RequestId      string
	Code           string
}

func (client *Client) CreateFileSystem(args *CreateFileSystemRequest) (resp CreateFileSystemResponse, err error) {
	response := CreateFileSystemResponse{}
	args.Version = VERSION

	err = client.Invoke("CreateFileSystem", args, &response)
	return response, err
}
