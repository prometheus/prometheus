package credentials

// Deprecated: Use AccessKeyCredential in this package instead.
type BaseCredential struct {
	AccessKeyId     string
	AccessKeySecret string
}

type AccessKeyCredential struct {
	AccessKeyId     string
	AccessKeySecret string
}

// Deprecated: Use NewAccessKeyCredential in this package instead.
func NewBaseCredential(accessKeyId, accessKeySecret string) *BaseCredential {
	return &BaseCredential{
		AccessKeyId:     accessKeyId,
		AccessKeySecret: accessKeySecret,
	}
}

func (baseCred *BaseCredential) ToAccessKeyCredential() *AccessKeyCredential {
	return &AccessKeyCredential{
		AccessKeyId:     baseCred.AccessKeyId,
		AccessKeySecret: baseCred.AccessKeySecret,
	}
}

func NewAccessKeyCredential(accessKeyId, accessKeySecret string) *AccessKeyCredential {
	return &AccessKeyCredential{
		AccessKeyId:     accessKeyId,
		AccessKeySecret: accessKeySecret,
	}
}
