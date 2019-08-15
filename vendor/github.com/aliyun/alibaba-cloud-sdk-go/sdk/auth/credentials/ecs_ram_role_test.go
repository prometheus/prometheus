package credentials

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestECSRamRole(t *testing.T) {
	c := NewEcsRamRoleCredential("rolename")
	assert.Equal(t, "rolename", c.RoleName)
	s := NewStsRoleNameOnEcsCredential("rolename")
	assert.Equal(t, "rolename", s.RoleName)
	r := s.ToEcsRamRoleCredential()
	assert.Equal(t, "rolename", r.RoleName)
}
