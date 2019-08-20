package kms

import "github.com/denverdino/aliyungo/common"

//https://help.aliyun.com/document_detail/44197.html?spm=5176.doc54560.6.562.p7NkZB
type CancelKeyDeletionArgs struct {
	KeyId string
}

type CancelKeyDeletionResponse struct {
	common.Response
}

func (client *Client) CancelKeyDeletion(keyId string) (*CancelKeyDeletionResponse, error) {
	args := &CancelKeyDeletionArgs{
		KeyId: keyId,
	}
	response := &CancelKeyDeletionResponse{}
	err := client.Invoke("CancelKeyDeletion", args, response)
	return response, err
}

//https://help.aliyun.com/document_detail/28947.html?spm=5176.doc44197.6.563.CRY6Nq
type KeyUsage string

const (
	KEY_USAGE_ENCRYPT_DECRYPT = KeyUsage("ENCRYPT/DECRYPT")
)

type CreateKeyArgs struct {
	KeyUsage    KeyUsage
	Description string
}

type KeyMetadata struct {
	CreationDate string
	Description  string
	KeyId        string
	KeyState     string
	KeyUsage     string
	DeleteDate   string
	Creator      string
	Arn          string
}

type CreateKeyResponse struct {
	common.Response
	KeyMetadata KeyMetadata
}

func (client *Client) CreateKey(args *CreateKeyArgs) (*CreateKeyResponse, error) {
	response := &CreateKeyResponse{}
	err := client.Invoke("CreateKey", args, response)
	return response, err
}

//https://help.aliyun.com/document_detail/28952.html?spm=5176.doc28950.6.565.66jjUg
type DescribeKeyArgs struct {
	KeyId string
}

type DescribeKeyResponse struct {
	common.Response
	KeyMetadata KeyMetadata
}

func (client *Client) DescribeKey(keyId string) (*DescribeKeyResponse, error) {
	response := &DescribeKeyResponse{}
	err := client.Invoke("DescribeKey", &DescribeKeyArgs{KeyId: keyId}, response)
	return response, err
}

//https://help.aliyun.com/document_detail/35151.html?spm=5176.doc28952.6.567.v0ZOgK
type DisableKeyArgs struct {
	KeyId string
}

type DisableKeyResponse struct {
	common.Response
}

func (client *Client) DisableKey(keyId string) (*DisableKeyResponse, error) {
	response := &DisableKeyResponse{}
	err := client.Invoke("DisableKey", &DisableKeyArgs{KeyId: keyId}, response)
	return response, err
}

//https://help.aliyun.com/document_detail/35150.html?spm=5176.doc35151.6.568.angV44
type EnableKeyArgs struct {
	KeyId string
}

type EnableKeyResponse struct {
	common.Response
}

func (client *Client) EnableKey(keyId string) (*EnableKeyResponse, error) {
	response := &EnableKeyResponse{}
	err := client.Invoke("EnableKey", &EnableKeyArgs{KeyId: keyId}, response)
	return response, err
}

//https://help.aliyun.com/document_detail/28948.html?spm=5176.doc28949.6.570.bFGCyz
type KeySpec string

const (
	KeySpec_AES_256 = KeySpec("AES_256")
	KeySpec_AES_128 = KeySpec("AES_128")
)

type GenerateDataKeyArgs struct {
	KeyId             string
	KeySpec           KeySpec
	NumberOfBytes     int
	EncryptionContext map[string]string
}

type GenerateDataKeyResponse struct {
	common.Response
	CiphertextBlob string
	KeyId          string
	RequestId      string
}

func (client *Client) GenerateDataKey(args *GenerateDataKeyArgs) (*GenerateDataKeyResponse, error) {
	response := &GenerateDataKeyResponse{}
	err := client.Invoke("GenerateDataKey", args, response)
	return response, err
}

//https://help.aliyun.com/document_detail/28951.html?spm=5176.doc28948.6.571.PIpkFD
type ListKeysArgs struct {
	common.Pagination
}

type Keys struct {
	Key []Key
}

type Key struct {
	KeyId  string
	KeyArn string
}

type ListKeysResponse struct {
	common.Response
	common.PaginationResult
	Keys Keys
}

func (client *Client) ListKeys(args *ListKeysArgs) (*ListKeysResponse, error) {
	response := &ListKeysResponse{}
	err := client.Invoke("ListKeys", args, response)
	return response, err
}

//https://help.aliyun.com/document_detail/44196.html?spm=5176.doc28951.6.572.9Oqd9S
type ScheduleKeyDeletionArgs struct {
	KeyId               string
	PendingWindowInDays int
}

type ScheduleKeyDeletionResponse struct {
	common.Response
}

func (client *Client) ScheduleKeyDeletion(args *ScheduleKeyDeletionArgs) (*ScheduleKeyDeletionResponse, error) {
	response := &ScheduleKeyDeletionResponse{}
	err := client.Invoke("ScheduleKeyDeletion", args, response)
	return response, err
}
