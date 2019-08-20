package ram

import (
	"github.com/denverdino/aliyungo/common"
)

/*
	All common struct
*/

const (
	Active   State = "Active"
	Inactive State = "Inactive"
)

/*
	AccountAlias
	类型：String
	必须：是
	描述：指定云账号的别名, 长度限制为3-63个字符
	限制：^[a-z0-9](([a-z0-9]|-(?!-))*[a-z0-9])?$
*/
type AccountAlias string

type UserQueryRequest struct {
	UserName string
}

type User struct {
	UserId        string
	UserName      string
	DisplayName   string
	MobilePhone   string
	Email         string
	Comments      string
	CreateDate    string
	UpdateDate    string
	LastLoginDate string
}

type LoginProfile struct {
	UserName              string
	PasswordResetRequired bool
	MFABindRequired       bool
}

type MFADevice struct {
	SerialNumber string
}

type VirtualMFADevice struct {
	SerialNumber     string
	Base32StringSeed string
	QRCodePNG        string
	ActivateDate     string
	User             User
}

type AccessKey struct {
	AccessKeyId     string
	AccessKeySecret string
	Status          State
	CreateDate      string
}

type Group struct {
	GroupName string
	Comments  string
}

type Role struct {
	RoleId                   string
	RoleName                 string
	Arn                      string
	Description              string
	AssumeRolePolicyDocument string
	CreateDate               string
	UpdateDate               string
}

type Policy struct {
	PolicyName      string
	PolicyType      string
	Description     string
	DefaultVersion  string
	CreateDate      string
	UpdateDate      string
	AttachmentCount int64
}

type PolicyVersion struct {
	VersionId        string
	IsDefaultVersion bool
	CreateDate       string
	PolicyDocument   string
}

type PolicyDocument struct {
	Statement []PolicyItem
	Version   string
}

type PolicyItem struct {
	Action   string
	Effect   string
	Resource string
}

type AssumeRolePolicyDocument struct {
	Statement []AssumeRolePolicyItem
	Version   string
}

type AssumeRolePolicyItem struct {
	Action    string
	Effect    string
	Principal AssumeRolePolicyPrincpal
}

type AssumeRolePolicyPrincpal struct {
	RAM []string
}

/*
	"PasswordPolicy": {
        "MinimumPasswordLength": 12,
        "RequireLowercaseCharacters": true,
        "RequireUppercaseCharacters": true,
        "RequireNumbers": true,
        "RequireSymbols": true
    }
*/

type PasswordPolicy struct {
	MinimumPasswordLength      int8
	RequireLowercaseCharacters bool
	RequireUppercaseCharacters bool
	RequireNumbers             bool
	RequireSymbols             bool
}

type RamCommonResponse struct {
	common.Response
}
