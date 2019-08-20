package ecs

import (
	"github.com/denverdino/aliyungo/common"
)

type CreateKeyPairArgs struct {
	RegionId    common.Region
	KeyPairName string
}

type CreateKeyPairResponse struct {
	common.Response
	KeyPairName        string
	KeyPairFingerPrint string
	PrivateKeyBody     string
}

// CreateKeyPair creates keypair
//
// You can read doc at https://help.aliyun.com/document_detail/51771.html?spm=5176.doc51775.6.910.cedjfr
func (client *Client) CreateKeyPair(args *CreateKeyPairArgs) (resp *CreateKeyPairResponse, err error) {
	response := CreateKeyPairResponse{}
	err = client.Invoke("CreateKeyPair", args, &response)
	if err != nil {
		return nil, err
	}
	return &response, err
}

type ImportKeyPairArgs struct {
	RegionId      common.Region
	PublicKeyBody string
	KeyPairName   string
}

type ImportKeyPairResponse struct {
	common.Response
	KeyPairName        string
	KeyPairFingerPrint string
}

// ImportKeyPair import keypair
//
// You can read doc at https://help.aliyun.com/document_detail/51774.html?spm=5176.doc51771.6.911.BicQq2
func (client *Client) ImportKeyPair(args *ImportKeyPairArgs) (resp *ImportKeyPairResponse, err error) {
	response := ImportKeyPairResponse{}
	err = client.Invoke("ImportKeyPair", args, &response)
	if err != nil {
		return nil, err
	}
	return &response, err
}

type DescribeKeyPairsArgs struct {
	RegionId           common.Region
	KeyPairFingerPrint string
	KeyPairName        string
	common.Pagination
}

type KeyPairItemType struct {
	KeyPairName        string
	KeyPairFingerPrint string
}

type DescribeKeyPairsResponse struct {
	common.Response
	common.PaginationResult
	RegionId common.Region
	KeyPairs struct {
		KeyPair []KeyPairItemType
	}
}

// DescribeKeyPairs describe keypairs
//
// You can read doc at https://help.aliyun.com/document_detail/51773.html?spm=5176.doc51774.6.912.lyE0iX
func (client *Client) DescribeKeyPairs(args *DescribeKeyPairsArgs) (KeyPairs []KeyPairItemType, pagination *common.PaginationResult, err error) {
	response, err := client.DescribeKeyPairsWithRaw(args)
	if err != nil {
		return nil, nil, err
	}

	return response.KeyPairs.KeyPair, &response.PaginationResult, err
}

func (client *Client) DescribeKeyPairsWithRaw(args *DescribeKeyPairsArgs) (response *DescribeKeyPairsResponse, err error) {
	response = &DescribeKeyPairsResponse{}

	err = client.Invoke("DescribeKeyPairs", args, response)

	if err != nil {
		return nil, err
	}

	return response, err
}

type AttachKeyPairArgs struct {
	RegionId    common.Region
	KeyPairName string
	InstanceIds string
}

// AttachKeyPair keypars to instances
//
// You can read doc at https://help.aliyun.com/document_detail/51775.html?spm=5176.doc51773.6.913.igEem4
func (client *Client) AttachKeyPair(args *AttachKeyPairArgs) (err error) {
	response := common.Response{}
	err = client.Invoke("AttachKeyPair", args, &response)
	if err != nil {
		return err
	}
	return nil
}

type DetachKeyPairArgs struct {
	RegionId    common.Region
	KeyPairName string
	InstanceIds string
}

// DetachKeyPair keyparis from instances
//
// You can read doc at https://help.aliyun.com/document_detail/51776.html?spm=5176.doc51775.6.914.DJ7Gmq
func (client *Client) DetachKeyPair(args *DetachKeyPairArgs) (err error) {
	response := common.Response{}
	err = client.Invoke("DetachKeyPair", args, &response)
	if err != nil {
		return err
	}
	return nil
}

type DeleteKeyPairsArgs struct {
	RegionId     common.Region
	KeyPairNames string
}

// DeleteKeyPairs delete keypairs
//
// You can read doc at https://help.aliyun.com/document_detail/51772.html?spm=5176.doc51776.6.915.Qqcv2Q
func (client *Client) DeleteKeyPairs(args *DeleteKeyPairsArgs) (err error) {
	response := common.Response{}
	err = client.Invoke("DeleteKeyPairs", args, &response)
	if err != nil {
		return err
	}
	return nil
}
