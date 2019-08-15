package credentials

func (oldCred *StsRoleNameOnEcsCredential) ToEcsRamRoleCredential() *EcsRamRoleCredential {
	return &EcsRamRoleCredential{
		RoleName: oldCred.RoleName,
	}
}

type EcsRamRoleCredential struct {
	RoleName string
}

func NewEcsRamRoleCredential(roleName string) *EcsRamRoleCredential {
	return &EcsRamRoleCredential{
		RoleName: roleName,
	}
}

// Deprecated: Use EcsRamRoleCredential in this package instead.
type StsRoleNameOnEcsCredential struct {
	RoleName string
}

// Deprecated: Use NewEcsRamRoleCredential in this package instead.
func NewStsRoleNameOnEcsCredential(roleName string) *StsRoleNameOnEcsCredential {
	return &StsRoleNameOnEcsCredential{
		RoleName: roleName,
	}
}
