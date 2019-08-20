package ram

type UserRequest struct {
	User
}

type UserResponse struct {
	RamCommonResponse
	User User
}

type UpdateUserRequest struct {
	UserName       string
	NewUserName    string
	NewDisplayName string
	NewMobilePhone string
	NewEmail       string
	NewComments    string
}

type ListUserRequest struct {
	Marker   string
	MaxItems int8
}

type ListUserResponse struct {
	RamCommonResponse
	IsTruncated bool
	Marker      string
	Users       struct {
		User []User
	}
}

func (client *RamClient) CreateUser(user UserRequest) (UserResponse, error) {
	var userResponse UserResponse
	err := client.Invoke("CreateUser", user, &userResponse)
	if err != nil {
		return UserResponse{}, err
	}
	return userResponse, nil
}

func (client *RamClient) GetUser(userQuery UserQueryRequest) (UserResponse, error) {
	var userResponse UserResponse
	err := client.Invoke("GetUser", userQuery, &userResponse)
	if err != nil {
		return UserResponse{}, nil
	}
	return userResponse, nil
}

func (client *RamClient) UpdateUser(newUser UpdateUserRequest) (UserResponse, error) {
	var userResponse UserResponse
	err := client.Invoke("UpdateUser", newUser, &userResponse)
	if err != nil {
		return UserResponse{}, err
	}
	return userResponse, nil
}

func (client *RamClient) DeleteUser(userQuery UserQueryRequest) (RamCommonResponse, error) {
	var commonResp RamCommonResponse
	err := client.Invoke("DeleteUser", userQuery, &commonResp)
	if err != nil {
		return RamCommonResponse{}, err
	}
	return commonResp, nil
}

func (client *RamClient) ListUsers(listParams ListUserRequest) (ListUserResponse, error) {
	var userList ListUserResponse
	err := client.Invoke("ListUsers", listParams, &userList)
	if err != nil {
		return ListUserResponse{}, err
	}
	return userList, nil
}
