package ecs

import "github.com/denverdino/aliyungo/common"

type CreateSnatEntryArgs struct {
	RegionId        common.Region
	SnatTableId     string
	SourceVSwitchId string
	SnatIp          string
}

type CreateSnatEntryResponse struct {
	common.Response
	SnatEntryId string
}

type SnatEntrySetType struct {
	RegionId        common.Region
	SnatEntryId     string
	SnatIp          string
	SnatTableId     string
	SourceCIDR      string
	SourceVSwitchId string
	Status          string
}

type DescribeSnatTableEntriesArgs struct {
	RegionId    common.Region
	SnatTableId string
	common.Pagination
}

type DescribeSnatTableEntriesResponse struct {
	common.Response
	common.PaginationResult
	SnatTableEntries struct {
		SnatTableEntry []SnatEntrySetType
	}
}

type ModifySnatEntryArgs struct {
	RegionId    common.Region
	SnatTableId string
	SnatEntryId string
	SnatIp      string
}

type ModifySnatEntryResponse struct {
	common.Response
}

type DeleteSnatEntryArgs struct {
	RegionId    common.Region
	SnatTableId string
	SnatEntryId string
}

type DeleteSnatEntryResponse struct {
	common.Response
}

func (client *Client) CreateSnatEntry(args *CreateSnatEntryArgs) (resp *CreateSnatEntryResponse, err error) {
	response := CreateSnatEntryResponse{}
	err = client.Invoke("CreateSnatEntry", args, &response)
	if err != nil {
		return nil, err
	}
	return &response, err
}

func (client *Client) DescribeSnatTableEntries(args *DescribeSnatTableEntriesArgs) (snatTableEntries []SnatEntrySetType,
	pagination *common.PaginationResult, err error) {
	response, err := client.DescribeSnatTableEntriesWithRaw(args)
	if err != nil {
		return nil, nil, err
	}

	return response.SnatTableEntries.SnatTableEntry, &response.PaginationResult, nil
}

func (client *Client) DescribeSnatTableEntriesWithRaw(args *DescribeSnatTableEntriesArgs) (response *DescribeSnatTableEntriesResponse, err error) {
	args.Validate()
	response = &DescribeSnatTableEntriesResponse{}

	err = client.Invoke("DescribeSnatTableEntries", args, response)

	if err != nil {
		return nil, err
	}

	return response, nil
}

func (client *Client) ModifySnatEntry(args *ModifySnatEntryArgs) error {
	response := ModifySnatEntryResponse{}
	return client.Invoke("ModifySnatEntry", args, &response)
}

func (client *Client) DeleteSnatEntry(args *DeleteSnatEntryArgs) error {
	response := DeleteSnatEntryResponse{}
	err := client.Invoke("DeleteSnatEntry", args, &response)
	return err
}
