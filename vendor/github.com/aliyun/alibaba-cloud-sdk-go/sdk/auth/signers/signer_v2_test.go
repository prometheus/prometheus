package signers

import (
	"testing"

	"github.com/aliyun/alibaba-cloud-sdk-go/sdk/auth/credentials"
	"github.com/stretchr/testify/assert"
)

func TestSignerV2(t *testing.T) {
	privateKey := `
MIICeQIBADANBgkqhkiG9w0BAQEFAASCAmMwggJfAgEAAoGBAOJC+2WXtkXZ+6sa
3+qJp4mDOsiZb3BghHT9nVbjTeaw4hsZWHYxQ6l6XDmTg4twPB59LOGAlAjYrT31
3pdwEawnmdf6zyF93Zvxxpy7lO2HoxYKSjbtXO4I0pcq3WTnw2xlbhqHvrcuWwt+
FqH9akzcnwHjc03siZBzt/dwDL3vAgMBAAECgYEAzwgZPqFuUEYgaTVDFDl2ynYA
kNMMzBgUu3Pgx0Nf4amSitdLQYLcdbQXtTtMT4eYCxHgwkpDqkCRbLOQRKNwFo0I
oaCuhjZlxWcKil4z4Zb/zB7gkeuXPOVUjFSS3FogsRWMtnNAMgR/yJRlbcg/Puqk
Magt/yDk+7cJCe6H96ECQQDxMT4S+tVP9nOw//QT39Dk+kWe/YVEhnWnCMZmGlEq
1gnN6qpUi68ts6b3BVgrDPrPN6wm/Z9vpcKNeWpIvxXRAkEA8CcT2UEUwDGRKAUu
WVPJqdAJjpjc072eRF5g792NyO+TAF6thBlDKNslRvFQDB6ymLsjfy8JYCnGbbSb
WqbHvwJBAIs7KeI6+jiWxGJA3t06LpSABQCqyOut0u0Bm8YFGyXnOPGtrXXwzMdN
Fe0zIJp5e69zK+W2Mvt4bL7OgBROeoECQQDsE+4uLw0gFln0tosmovhmp60NcfX7
bLbtzL2MbwbXlbOztF7ssgzUWAHgKI6hK3g0LhsqBuo3jzmSVO43giZvAkEA08Nm
2TI9EvX6DfCVfPOiKZM+Pijh0xLN4Dn8qUgt3Tcew/vfj4WA2ZV6qiJqL01vMsHc
vftlY0Hs1vNXcaBgEA==`
	c := credentials.NewRsaKeyPairCredential(privateKey, "publicKeyId", 3600)
	s := NewSignerV2(c)
	assert.Equal(t, "SHA256withRSA", s.GetName())
	assert.Equal(t, "PRIVATEKEY", s.GetType())
	assert.Equal(t, "1.0", s.GetVersion())
	assert.Nil(t, s.GetExtraParam())
	accesskeyId, err := s.GetAccessKeyId()
	assert.Nil(t, err)
	assert.Equal(t, "publicKeyId", accesskeyId)
	assert.Equal(t, "KoQz1EdAD5jsmYuvaDZTQLQo4bP2ex6zR0dJcsNTjVE/MgGP8emz0rhiwSDmffEsbGPrRN8qPWGltEleH7xbLuBtviBbW5M7Ga7cuYQaxATDbwsPVNGgr3QPWY+nEjX3lBwGAeebf5H9WidI1cbTB+uYh0XB4o/sL34npE6qOxk=", s.Sign("string to sign", "suffix"))
}
