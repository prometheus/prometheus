package ram

/*
	CreateAccessKey()
	UpdateAccessKey()
	DeleteAccessKey()
	ListAccessKeys()
*/
type State string

type AccessKeyResponse struct {
	RamCommonResponse
	AccessKey AccessKey
}

type UpdateAccessKeyRequest struct {
	UserAccessKeyId string
	Status          State
	UserName        string
}

type AccessKeyListResponse struct {
	RamCommonResponse
	AccessKeys struct {
		AccessKey []AccessKey
	}
}

func (client *RamClient) CreateAccessKey(userQuery UserQueryRequest) (AccessKeyResponse, error) {
	var accesskeyResp AccessKeyResponse
	err := client.Invoke("CreateAccessKey", userQuery, &accesskeyResp)
	if err != nil {
		return AccessKeyResponse{}, err
	}
	return accesskeyResp, nil
}

func (client *RamClient) UpdateAccessKey(accessKeyRequest UpdateAccessKeyRequest) (RamCommonResponse, error) {
	var commonResp RamCommonResponse
	err := client.Invoke("UpdateAccessKey", accessKeyRequest, &commonResp)
	if err != nil {
		return RamCommonResponse{}, err
	}
	return commonResp, nil
}

func (client *RamClient) DeleteAccessKey(accessKeyRequest UpdateAccessKeyRequest) (RamCommonResponse, error) {
	var commonResp RamCommonResponse
	err := client.Invoke("DeleteAccessKey", accessKeyRequest, &commonResp)
	if err != nil {
		return RamCommonResponse{}, err
	}
	return commonResp, nil
}

func (client *RamClient) ListAccessKeys(userQuery UserQueryRequest) (AccessKeyListResponse, error) {
	var accessKeyListResp AccessKeyListResponse
	err := client.Invoke("ListAccessKeys", userQuery, &accessKeyListResp)
	if err != nil {
		return AccessKeyListResponse{}, err
	}
	return accessKeyListResp, nil
}
