package ram

type RoleRequest struct {
	RoleName                 string
	AssumeRolePolicyDocument string
	Description              string
}

type RoleResponse struct {
	RamCommonResponse
	Role Role
}

type RoleQueryRequest struct {
	RoleName string
}

type UpdateRoleRequest struct {
	RoleName                    string
	NewAssumeRolePolicyDocument string
}

type ListRoleResponse struct {
	RamCommonResponse
	Roles struct {
		Role []Role
	}
}

func (client *RamClient) CreateRole(role RoleRequest) (RoleResponse, error) {
	var roleResponse RoleResponse
	err := client.Invoke("CreateRole", role, &roleResponse)
	if err != nil {
		return RoleResponse{}, err
	}
	return roleResponse, nil
}

func (client *RamClient) GetRole(roleQuery RoleQueryRequest) (RoleResponse, error) {
	var roleResponse RoleResponse
	err := client.Invoke("GetRole", roleQuery, &roleResponse)
	if err != nil {
		return RoleResponse{}, nil
	}
	return roleResponse, nil
}

func (client *RamClient) UpdateRole(newRole UpdateRoleRequest) (RoleResponse, error) {
	var roleResponse RoleResponse
	err := client.Invoke("UpdateRole", newRole, &roleResponse)
	if err != nil {
		return RoleResponse{}, err
	}
	return roleResponse, nil
}

func (client *RamClient) ListRoles() (ListRoleResponse, error) {
	var roleList ListRoleResponse
	err := client.Invoke("ListRoles", struct{}{}, &roleList)
	if err != nil {
		return ListRoleResponse{}, err
	}
	return roleList, nil
}

func (client *RamClient) DeleteRole(roleQuery RoleQueryRequest) (RamCommonResponse, error) {
	var commonResp RamCommonResponse
	err := client.Invoke("DeleteRole", roleQuery, &commonResp)
	if err != nil {
		return RamCommonResponse{}, err
	}
	return commonResp, nil
}
