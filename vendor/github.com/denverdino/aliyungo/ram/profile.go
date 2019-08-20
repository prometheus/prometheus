package ram

/*
	CreateLoginProfile()
	GetLoginProfile()
	DeleteLoginProfile()
	UpdateLoginProfile()
*/

type ProfileRequest struct {
	UserName              string
	Password              string
	PasswordResetRequired bool
	MFABindRequired       bool
}

type ProfileResponse struct {
	RamCommonResponse
	LoginProfile LoginProfile
}

func (client *RamClient) CreateLoginProfile(req ProfileRequest) (ProfileResponse, error) {
	var resp ProfileResponse
	err := client.Invoke("CreateLoginProfile", req, &resp)
	if err != nil {
		return ProfileResponse{}, err
	}
	return resp, nil
}

func (client *RamClient) GetLoginProfile(req UserQueryRequest) (ProfileResponse, error) {
	var resp ProfileResponse
	err := client.Invoke("GetLoginProfile", req, &resp)
	if err != nil {
		return ProfileResponse{}, err
	}
	return resp, nil
}

func (client *RamClient) DeleteLoginProfile(req UserQueryRequest) (RamCommonResponse, error) {
	var resp RamCommonResponse
	err := client.Invoke("DeleteLoginProfile", req, &resp)
	if err != nil {
		return RamCommonResponse{}, err
	}
	return resp, nil
}

func (client *RamClient) UpdateLoginProfile(req ProfileRequest) (ProfileResponse, error) {
	var resp ProfileResponse
	err := client.Invoke("UpdateLoginProfile", req, &resp)
	if err != nil {
		return ProfileResponse{}, err
	}
	return resp, nil
}
