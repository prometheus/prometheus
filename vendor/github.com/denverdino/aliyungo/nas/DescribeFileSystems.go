package nas

type DescribeFileSystemsRequest struct {
	FileSystemName string
	Version        string
	RegionId       string
}

type DescribeFileSystemsResponse struct {
	FileSystems []FileSystem
	Code        string
}

type FileSystem struct {
	CreateTime       uint64
	MountTargetCount uint64
	PackageId        string
	FileSystemName   string
	FileSystemType   string
	MeteredSize      uint64
	FileSystemDesc   string
	QuotaSize        uint64
	ZoneId           string
}

func (client *Client) DescribeFileSystems(args *DescribeFileSystemsRequest) (resp DescribeFileSystemsResponse, err error) {
	response := DescribeFileSystemsResponse{}
	args.Version = VERSION
	err = client.Invoke("DescribeFileSystems", args, &response)
	return response, err
}
