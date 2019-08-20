package kms

import "github.com/denverdino/aliyungo/common"

//https://help.aliyun.com/document_detail/28949.html?spm=5176.doc28950.6.569.kI1mTw
type EncryptAgrs struct {
	KeyId             string
	Plaintext         string
	EncryptionContext map[string]string
}

type EncryptResponse struct {
	common.Response
	CiphertextBlob string
	KeyId          string
}

func (client *Client) Encrypt(args *EncryptAgrs) (*EncryptResponse, error) {
	response := &EncryptResponse{}
	err := client.Invoke("Encrypt", args, response)
	return response, err
}
