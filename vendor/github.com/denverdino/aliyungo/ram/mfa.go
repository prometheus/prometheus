package ram

type MFARequest struct {
	VirtualMFADeviceName string
}

type MFADeleteRequest struct {
	MFADevice
}

type MFABindRequest struct {
	SerialNumber        string
	UserName            string
	AuthenticationCode1 string
	AuthenticationCode2 string
}

type MFAResponse struct {
	RamCommonResponse
	VirtualMFADevice VirtualMFADevice
}

type MFAListResponse struct {
	RamCommonResponse
	VirtualMFADevices struct {
		VirtualMFADevice []VirtualMFADevice
	}
}

type MFAUserResponse struct {
	RamCommonResponse
	MFADevice MFADevice
}

func (client *RamClient) CreateVirtualMFADevice(req MFARequest) (MFAResponse, error) {
	var resp MFAResponse
	err := client.Invoke("CreateVirtualMFADevice", req, &resp)
	if err != nil {
		return MFAResponse{}, err
	}
	return resp, nil
}

func (client *RamClient) ListVirtualMFADevices() (MFAListResponse, error) {
	var resp MFAListResponse
	err := client.Invoke("ListVirtualMFADevices", struct{}{}, &resp)
	if err != nil {
		return MFAListResponse{}, err
	}
	return resp, nil
}

func (client *RamClient) DeleteVirtualMFADevice(req MFADeleteRequest) (RamCommonResponse, error) {
	var resp RamCommonResponse
	err := client.Invoke("DeleteVirtualMFADevice", req, &resp)
	if err != nil {
		return RamCommonResponse{}, err
	}
	return resp, nil
}

func (client *RamClient) BindMFADevice(req MFABindRequest) (RamCommonResponse, error) {
	var resp RamCommonResponse
	err := client.Invoke("BindMFADevice", req, &resp)
	if err != nil {
		return RamCommonResponse{}, err
	}
	return resp, nil
}

func (client *RamClient) UnbindMFADevice(req UserQueryRequest) (MFAUserResponse, error) {
	var resp MFAUserResponse
	err := client.Invoke("UnbindMFADevice", req, &resp)
	if err != nil {
		return MFAUserResponse{}, err
	}
	return resp, nil
}

func (client *RamClient) GetUserMFAInfo(req UserQueryRequest) (MFAUserResponse, error) {
	var resp MFAUserResponse
	err := client.Invoke("GetUserMFAInfo", req, &resp)
	if err != nil {
		return MFAUserResponse{}, err
	}
	return resp, nil
}
