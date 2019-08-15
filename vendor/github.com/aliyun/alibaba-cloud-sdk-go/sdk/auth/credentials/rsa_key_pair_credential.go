package credentials

type RsaKeyPairCredential struct {
	PrivateKey        string
	PublicKeyId       string
	SessionExpiration int
}

func NewRsaKeyPairCredential(privateKey, publicKeyId string, sessionExpiration int) *RsaKeyPairCredential {
	return &RsaKeyPairCredential{
		PrivateKey:        privateKey,
		PublicKeyId:       publicKeyId,
		SessionExpiration: sessionExpiration,
	}
}
