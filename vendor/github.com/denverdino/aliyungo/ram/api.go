package ram

/*
	ringtail 2016/1/19
	All RAM apis provided
*/

type RamClientInterface interface {
	//ram user
	CreateUser(user UserRequest) (UserResponse, error)
	GetUser(userQuery UserQueryRequest) (UserResponse, error)
	UpdateUser(newUser UpdateUserRequest) (UserResponse, error)
	DeleteUser(userQuery UserQueryRequest) (RamCommonResponse, error)
	ListUsers(listParams ListUserRequest) (ListUserResponse, error)

	//ram login profile
	CreateLoginProfile(req ProfileRequest) (ProfileResponse, error)
	GetLoginProfile(req UserQueryRequest) (ProfileResponse, error)
	DeleteLoginProfile(req UserQueryRequest) (RamCommonResponse, error)
	UpdateLoginProfile(req ProfileRequest) (ProfileResponse, error)

	//ram ak
	CreateAccessKey(userQuery UserQueryRequest) (AccessKeyResponse, error)
	UpdateAccessKey(accessKeyRequest UpdateAccessKeyRequest) (RamCommonResponse, error)
	DeleteAccessKey(accessKeyRequest UpdateAccessKeyRequest) (RamCommonResponse, error)
	ListAccessKeys(userQuery UserQueryRequest) (AccessKeyListResponse, error)

	//ram mfa
	CreateVirtualMFADevice(req MFARequest) (MFAResponse, error)
	ListVirtualMFADevices() (MFAListResponse, error)
	DeleteVirtualMFADevice(req MFADeleteRequest) (RamCommonResponse, error)
	BindMFADevice(req MFABindRequest) (RamCommonResponse, error)
	UnbindMFADevice(req UserQueryRequest) (MFAUserResponse, error)
	GetUserMFAInfo(req UserQueryRequest) (MFAUserResponse, error)

	//ram group
	CreateGroup(req GroupRequest) (GroupResponse, error)
	GetGroup(req GroupQueryRequest) (GroupResponse, error)
	UpdateGroup(req GroupUpdateRequest) (GroupResponse, error)
	ListGroup(req GroupListRequest) (GroupListResponse, error)
	DeleteGroup(req GroupQueryRequest) (RamCommonResponse, error)
	AddUserToGroup(req UserRelateGroupRequest) (RamCommonResponse, error)
	RemoveUserFromGroup(req UserRelateGroupRequest) (RamCommonResponse, error)
	ListGroupsForUser(req UserQueryRequest) (GroupListResponse, error)
	ListUsersForGroup(req GroupQueryRequest) (ListUserResponse, error)

	CreateRole(role RoleRequest) (RoleResponse, error)
	GetRole(roleQuery RoleQueryRequest) (RoleResponse, error)
	UpdateRole(newRole UpdateRoleRequest) (RoleResponse, error)
	ListRoles() (ListRoleResponse, error)
	DeleteRole(roleQuery RoleQueryRequest) (RamCommonResponse, error)

	//DONE policy
	CreatePolicy(policyReq PolicyRequest) (PolicyResponse, error)
	GetPolicy(policyReq PolicyRequest) (PolicyResponse, error)
	DeletePolicy(policyReq PolicyRequest) (RamCommonResponse, error)
	ListPolicies(policyQuery PolicyQueryRequest) (PolicyQueryResponse, error)
	ListPoliciesForUser(userQuery UserQueryRequest) (PolicyListResponse, error)

	//ram policy version
	CreatePolicyVersion(policyReq PolicyRequest) (PolicyVersionResponse, error)
	GetPolicyVersion(policyReq PolicyRequest) (PolicyVersionResponse, error)
	GetPolicyVersionNew(policyReq PolicyRequest) (PolicyVersionResponseNew, error)
	DeletePolicyVersion(policyReq PolicyRequest) (RamCommonResponse, error)
	ListPolicyVersions(policyReq PolicyRequest) (PolicyVersionResponse, error)
	ListPolicyVersionsNew(policyReq PolicyRequest) (PolicyVersionsResponse, error)
	AttachPolicyToUser(attachPolicyRequest AttachPolicyRequest) (RamCommonResponse, error)
	DetachPolicyFromUser(attachPolicyRequest AttachPolicyRequest) (RamCommonResponse, error)
	ListEntitiesForPolicy(policyReq PolicyRequest) (PolicyListEntitiesResponse, error)
	SetDefaultPolicyVersion(policyReq PolicyRequest) (RamCommonResponse, error)
	ListPoliciesForGroup(groupQuery GroupQueryRequest) (PolicyListResponse, error)
	AttachPolicyToGroup(attachPolicyRequest AttachPolicyToGroupRequest) (RamCommonResponse, error)
	DetachPolicyFromGroup(attachPolicyRequest AttachPolicyToGroupRequest) (RamCommonResponse, error)
	AttachPolicyToRole(attachPolicyRequest AttachPolicyToRoleRequest) (RamCommonResponse, error)
	DetachPolicyFromRole(attachPolicyRequest AttachPolicyToRoleRequest) (RamCommonResponse, error)
	ListPoliciesForRole(roleQuery RoleQueryRequest) (PolicyListResponse, error)

	//ram security
	SetAccountAlias(accountAlias AccountAliasRequest) (RamCommonResponse, error)
	GetAccountAlias() (AccountAliasResponse, error)
	ClearAccountAlias() (RamCommonResponse, error)
	SetPasswordPolicy(passwordPolicy PasswordPolicyRequest) (PasswordPolicyResponse, error)
	GetPasswordPolicy() (PasswordPolicyResponse, error)

	// Common Client Methods
	SetUserAgent(userAgent string)
}
