package credentials

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestRoleArnCredential(t *testing.T) {
	c := NewRamRoleArnCredential("accessKeyId", "accessKeySecret", "roleArn", "roleSessionName", 3600)
	assert.Equal(t, "accessKeyId", c.AccessKeyId)
	assert.Equal(t, "accessKeySecret", c.AccessKeySecret)
	assert.Equal(t, "roleArn", c.RoleArn)
	assert.Equal(t, "roleSessionName", c.RoleSessionName)
	assert.Equal(t, 3600, c.RoleSessionExpiration)
	s := NewStsRoleArnCredential("accessKeyId", "accessKeySecret", "roleArn", "roleSessionName", 3600)
	assert.Equal(t, "accessKeyId", s.AccessKeyId)
	assert.Equal(t, "accessKeySecret", s.AccessKeySecret)
	assert.Equal(t, "roleArn", s.RoleArn)
	assert.Equal(t, "roleSessionName", s.RoleSessionName)
	assert.Equal(t, 3600, s.RoleSessionExpiration)
	r := s.ToRamRoleArnCredential()
	assert.Equal(t, "accessKeyId", r.AccessKeyId)
	assert.Equal(t, "accessKeySecret", r.AccessKeySecret)
	assert.Equal(t, "roleArn", r.RoleArn)
	assert.Equal(t, "roleSessionName", r.RoleSessionName)
	assert.Equal(t, 3600, r.RoleSessionExpiration)
	p := NewRamRoleArnWithPolicyCredential("accessKeyId", "accessKeySecret", "roleArn", "roleSessionName", "test", 3600)
	assert.Equal(t, "accessKeyId", p.AccessKeyId)
	assert.Equal(t, "accessKeySecret", p.AccessKeySecret)
	assert.Equal(t, "roleArn", p.RoleArn)
	assert.Equal(t, "test", p.Policy)
	assert.Equal(t, "roleSessionName", p.RoleSessionName)
	assert.Equal(t, 3600, p.RoleSessionExpiration)
}
