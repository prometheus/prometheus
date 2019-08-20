package ram

type GroupRequest struct {
	Group
}

type GroupQueryRequest struct {
	GroupName string
}

type GroupUpdateRequest struct {
	GroupName    string
	NewGroupName string
	NewComments  string
}

type GroupListRequest struct {
	Marker   string
	MaxItems int8
}

type UserRelateGroupRequest struct {
	UserName  string
	GroupName string
}

type GroupResponse struct {
	RamCommonResponse
	Group Group
}

type GroupListResponse struct {
	RamCommonResponse
	IsTruncated bool
	Marker      string
	Groups      struct {
		Group []Group
	}
}

func (client *RamClient) CreateGroup(req GroupRequest) (GroupResponse, error) {
	var resp GroupResponse
	err := client.Invoke("CreateGroup", req, &resp)
	if err != nil {
		return GroupResponse{}, err
	}
	return resp, nil
}

func (client *RamClient) GetGroup(req GroupQueryRequest) (GroupResponse, error) {
	var resp GroupResponse
	err := client.Invoke("GetGroup", req, &resp)
	if err != nil {
		return GroupResponse{}, err
	}
	return resp, nil
}

func (client *RamClient) UpdateGroup(req GroupUpdateRequest) (GroupResponse, error) {
	var resp GroupResponse
	err := client.Invoke("UpdateGroup", req, &resp)
	if err != nil {
		return GroupResponse{}, err
	}
	return resp, nil
}

func (client *RamClient) ListGroup(req GroupListRequest) (GroupListResponse, error) {
	var resp GroupListResponse
	err := client.Invoke("ListGroups", req, &resp)
	if err != nil {
		return GroupListResponse{}, err
	}
	return resp, nil
}

func (client *RamClient) DeleteGroup(req GroupQueryRequest) (RamCommonResponse, error) {
	var resp RamCommonResponse
	err := client.Invoke("DeleteGroup", req, &resp)
	if err != nil {
		return RamCommonResponse{}, err
	}
	return resp, nil
}

func (client *RamClient) AddUserToGroup(req UserRelateGroupRequest) (RamCommonResponse, error) {
	var resp RamCommonResponse
	err := client.Invoke("AddUserToGroup", req, &resp)
	if err != nil {
		return RamCommonResponse{}, err
	}
	return resp, nil
}

func (client *RamClient) RemoveUserFromGroup(req UserRelateGroupRequest) (RamCommonResponse, error) {
	var resp RamCommonResponse
	err := client.Invoke("RemoveUserFromGroup", req, &resp)
	if err != nil {
		return RamCommonResponse{}, err
	}
	return resp, nil
}

func (client *RamClient) ListGroupsForUser(req UserQueryRequest) (GroupListResponse, error) {
	var resp GroupListResponse
	err := client.Invoke("ListGroupsForUser", req, &resp)
	if err != nil {
		return GroupListResponse{}, err
	}
	return resp, nil
}

func (client *RamClient) ListUsersForGroup(req GroupQueryRequest) (ListUserResponse, error) {
	var resp ListUserResponse
	err := client.Invoke("ListUsersForGroup", req, &resp)
	if err != nil {
		return ListUserResponse{}, err
	}
	return resp, nil
}
