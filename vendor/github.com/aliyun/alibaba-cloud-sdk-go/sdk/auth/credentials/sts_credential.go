package credentials

type StsTokenCredential struct {
	AccessKeyId       string
	AccessKeySecret   string
	AccessKeyStsToken string
}

func NewStsTokenCredential(accessKeyId, accessKeySecret, accessKeyStsToken string) *StsTokenCredential {
	return &StsTokenCredential{
		AccessKeyId:       accessKeyId,
		AccessKeySecret:   accessKeySecret,
		AccessKeyStsToken: accessKeyStsToken,
	}
}
