package ecs

import "github.com/denverdino/aliyungo/common"

type CreateForwardEntryArgs struct {
	RegionId       common.Region
	ForwardTableId string
	ExternalIp     string
	ExternalPort   string
	IpProtocol     string
	InternalIp     string
	InternalPort   string
}

type CreateForwardEntryResponse struct {
	common.Response
	ForwardEntryId string
}

type DescribeForwardTableEntriesArgs struct {
	RegionId       common.Region
	ForwardTableId string
	common.Pagination
}

type ForwardTableEntrySetType struct {
	RegionId       common.Region
	ExternalIp     string
	ExternalPort   string
	ForwardEntryId string
	ForwardTableId string
	InternalIp     string
	InternalPort   string
	IpProtocol     string
	Status         string
}

type DescribeForwardTableEntriesResponse struct {
	common.Response
	common.PaginationResult
	ForwardTableEntries struct {
		ForwardTableEntry []ForwardTableEntrySetType
	}
}

type ModifyForwardEntryArgs struct {
	RegionId       common.Region
	ForwardTableId string
	ForwardEntryId string
	ExternalIp     string
	IpProtocol     string
	ExternalPort   string
	InternalIp     string
	InternalPort   string
}

type ModifyForwardEntryResponse struct {
	common.Response
}

type DeleteForwardEntryArgs struct {
	RegionId       common.Region
	ForwardTableId string
	ForwardEntryId string
}

type DeleteForwardEntryResponse struct {
	common.Response
}

func (client *Client) CreateForwardEntry(args *CreateForwardEntryArgs) (resp *CreateForwardEntryResponse, err error) {
	response := CreateForwardEntryResponse{}
	err = client.Invoke("CreateForwardEntry", args, &response)
	if err != nil {
		return nil, err
	}
	return &response, err
}

func (client *Client) DescribeForwardTableEntries(args *DescribeForwardTableEntriesArgs) (forwardTableEntries []ForwardTableEntrySetType,
	pagination *common.PaginationResult, err error) {
	response, err := client.DescribeForwardTableEntriesWithRaw(args)
	if err != nil {
		return nil, nil, err
	}

	return response.ForwardTableEntries.ForwardTableEntry, &response.PaginationResult, nil
}

func (client *Client) DescribeForwardTableEntriesWithRaw(args *DescribeForwardTableEntriesArgs) (response *DescribeForwardTableEntriesResponse, err error) {
	args.Validate()
	response = &DescribeForwardTableEntriesResponse{}

	err = client.Invoke("DescribeForwardTableEntries", args, response)

	if err != nil {
		return nil, err
	}

	return response, nil
}

func (client *Client) ModifyForwardEntry(args *ModifyForwardEntryArgs) error {
	response := ModifyForwardEntryResponse{}
	return client.Invoke("ModifyForwardEntry", args, &response)
}

func (client *Client) DeleteForwardEntry(args *DeleteForwardEntryArgs) error {
	response := DeleteForwardEntryResponse{}
	err := client.Invoke("DeleteForwardEntry", args, &response)
	return err
}
