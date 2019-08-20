package kms

import "github.com/denverdino/aliyungo/common"

//https://help.aliyun.com/document_detail/28950.html?spm=5176.doc28947.6.564.JrFZRr
type DecryptArgs struct {
	CiphertextBlob    string
	EncryptionContext map[string]string
}

type DecryptResponse struct {
	common.Response
	Plaintext string
	KeyId     string
}

func (client *Client) Decrypt(args *DecryptArgs) (*DecryptResponse, error) {
	response := &DecryptResponse{}
	err := client.Invoke("Decrypt", args, response)
	return response, err
}
