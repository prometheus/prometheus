package credentials

type BearerTokenCredential struct {
	BearerToken string
}

// NewBearerTokenCredential return a BearerTokenCredential object
func NewBearerTokenCredential(token string) *BearerTokenCredential {
	return &BearerTokenCredential{
		BearerToken: token,
	}
}
