// Copyright (c) 2016, 2018, Oracle and/or its affiliates. All rights reserved.

package common

import (
	"fmt"
	"io/ioutil"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"path"
	"strings"
)

var (
	tuser              = "someuser"
	tfingerprint       = "somefingerprint"
	tkeyfile           = "somelocation"
	ttenancy           = "sometenancy"
	tregion            = "someregion"
	testPrivateKeyConf = `-----BEGIN RSA PRIVATE KEY-----
MIICXgIBAAKBgQDCFENGw33yGihy92pDjZQhl0C36rPJj+CvfSC8+q28hxA161QF
NUd13wuCTUcq0Qd2qsBe/2hFyc2DCJJg0h1L78+6Z4UMR7EOcpfdUE9Hf3m/hs+F
UR45uBJeDK1HSFHD8bHKD6kv8FPGfJTotc+2xjJwoYi+1hqp1fIekaxsyQIDAQAB
AoGBAJR8ZkCUvx5kzv+utdl7T5MnordT1TvoXXJGXK7ZZ+UuvMNUCdN2QPc4sBiA
QWvLw1cSKt5DsKZ8UETpYPy8pPYnnDEz2dDYiaew9+xEpubyeW2oH4Zx71wqBtOK
kqwrXa/pzdpiucRRjk6vE6YY7EBBs/g7uanVpGibOVAEsqH1AkEA7DkjVH28WDUg
f1nqvfn2Kj6CT7nIcE3jGJsZZ7zlZmBmHFDONMLUrXR/Zm3pR5m0tCmBqa5RK95u
412jt1dPIwJBANJT3v8pnkth48bQo/fKel6uEYyboRtA5/uHuHkZ6FQF7OUkGogc
mSJluOdc5t6hI1VsLn0QZEjQZMEOWr+wKSMCQQCC4kXJEsHAve77oP6HtG/IiEn7
kpyUXRNvFsDE0czpJJBvL/aRFUJxuRK91jhjC68sA7NsKMGg5OXb5I5Jj36xAkEA
gIT7aFOYBFwGgQAQkWNKLvySgKbAZRTeLBacpHMuQdl1DfdntvAyqpAZ0lY0RKmW
G6aFKaqQfOXKCyWoUiVknQJAXrlgySFci/2ueKlIE1QqIiLSZ8V8OlpFLRnb1pzI
7U1yQXnTAEFYM560yJlzUpOb1V4cScGd365tiSMvxLOvTA==
-----END RSA PRIVATE KEY-----`
	testEncryptedPrivateKeyConf = `-----BEGIN RSA PRIVATE KEY-----
Proc-Type: 4,ENCRYPTED
DEK-Info: DES-EDE3-CBC,05B7ACED45203763

bKbv8X2oyfxwp55w3MVKj1bfWnhvQgyqJ/1dER53STao3qRS26epRoBc0BoLtrNj
L+Wfa3NeuEinetDYKRwWGHZqvbs/3PD5OKIXW1y/EAlg1vr6JWX8KxhQ0PzGJOdQ
KPcB2duDtlNJ4awoGEsSp/qYyJLKOKpcz893OWTe3Oi9aQpzuL+kgH6VboCUdwdl
Ub7YyTMFBkGzzjOXV/iSJDaxvVUIZt7CQS/DkBq4IHXX8iFUDzh6L297/BuRp3Q8
hDL4yQacl2F2yCWpUoNNkbPpe6oOmL8JHrxXxo+u0pSJELXx0sjWMn7bSRfgFFIE
k08y4wXZeoxHiQDhHmQI+YTikgqnxEWtDYhHYvWudVQY6Wcf1Fdypa1v4I3gv4S9
QwjDRbRcrnPxMkxWmQEM6xGCwWBj8wmFyIQoEA5MJuQZxWdyptEKVtwwI1TB9etn
SlXPUl125dYYBu2ynmR96nBVEZd6BWl+iFeeZnqxDHABOB0AvpI61vt/6c7tIimC
YciZs74XZH/ERs55p0Ng/G23XNu+UGQQptrr2kyRR5JrS0UGKVjivydIK5Lus4c4
NTaKyEJNMbvSUGY5SLfxyp6HZnlbr4aCDAk62+2ZUotr+sVXplCpuxoSc2Qlw0en
y+plCvd2RdQ/EzIFkpi9V/snIvbMvH3Sp/HqFDG8GehFTRvwpCIVqWC+BZYeaERX
n2P4jODz2M8Ns7txv1nB4CyxWgu19398Zit0K0QmG24kCJtLg9spEOmKtoIuVTnU
9ydxmHQjNNtyH+RceZFn07IkWvPveo2BXpK4K9DXE39Z/g1nQzwTqgN8diXxwRuN
Ge97lBWup4vP1TV8nyHW2AppgFVuPynO+XWfZUuCUzxNseB+XOyeqitoM4uvSNax
DQmokjIf4qXC/46EnJ/fd9Ydz4GVQ4TYyxwNCBJK39RdUOcUtyI+A3IbZ+vt2HIV
eiIN2BhdnwbvNTbPs9nc9McM2NtACqDGQsIzRdXcQ8SFDP2DnTVjGu5E8H9dnVrd
FcuUnA9TIbfBkRHOS7yoDHOo4j28g6xePDV5tK0L5C2yyDh+bwWnO5AIg/gdpnuH
wxIZUxFwkD4GvOVtj5Y4W5L+Uy3c94stMPbHE+zGN75DdQRy5aVbDjWqXRB9AEQN
+NSb526oqhv0JyYlZmCqz2ydBxkT4FsShZv/34pkRr3qL5FSTAQTXQAZdiQQbMTe
H3zKyu4GbEUV9WsyriqSq27ptMwFfIqN1NdsWeVWN1mXf2KZDn61EgleeQXmdSZu
XM4Z1n98xjYDwdCkF738j+oRAlSUThBeU/hYbH6Ysff6ON9MPBAAKy3ZxM5tF86e
l0x20lpND2QLLDZbsg/LrCrE6ZzpWkXn4w4PG4lWMAqph0BebSkFqXvUvuds3c39
yptNH3FsyqeyM9kDwbDpBQAvpsDIQJfwAbQPLAiQJhpbixZyG9lqhkKOhYTZhU3l
ufFtnLEj/5G9a8A//MFrXsXePUeBDEzjtEcjPGNxe0ZkuOgYx11Zc0R4oLI7LoHO
07vtw4qCH4hztCJ5+JOUac6sGcILFRc4vSQQ15Cg5QEdBiSbQ/yo1P0hbNtSvnwO
-----END RSA PRIVATE KEY-----`
	testKeyPassphrase = "goisfun"
)

func removeFileFn(filename string) {
	os.Remove(filename)
}

func writeTempFile(data string) (filename string) {
	f, _ := ioutil.TempFile("", "gosdkTest")
	f.WriteString(data)
	filename = f.Name()
	return
}

func TestRawConfigurationProvider(t *testing.T) {
	var (
		testTenancy     = "ocid1.tenancy.oc1..aaaaaaaaxf3fuazos"
		testUser        = "ocid1.user.oc1..aaaaaaaa3p67n2kmpxnbcnff"
		testRegion      = "us-ashburn-1"
		testFingerprint = "af:81:71:8e:d2"
	)

	c := NewRawConfigurationProvider(testTenancy, testUser, testRegion, testFingerprint, testPrivateKeyConf, nil)

	user, err := c.UserOCID()
	assert.NoError(t, err)
	assert.Equal(t, user, testUser)

	fingerprint, err := c.KeyFingerprint()
	assert.NoError(t, err)
	assert.Equal(t, fingerprint, testFingerprint)

	region, err := c.Region()
	assert.NoError(t, err)
	assert.Equal(t, region, testRegion)

	rsaKey, err := c.PrivateRSAKey()
	assert.NoError(t, err)
	assert.NotEmpty(t, rsaKey)

	keyID, err := c.KeyID()
	assert.NoError(t, err)
	assert.NotEmpty(t, keyID)

	assert.Equal(t, keyID, "ocid1.tenancy.oc1..aaaaaaaaxf3fuazos/ocid1.user.oc1..aaaaaaaa3p67n2kmpxnbcnff/af:81:71:8e:d2")

}

func TestFileConfigurationProvider_parseConfigFileData(t *testing.T) {
	data := `[DEFAULT]
user=someuser
fingerprint=somefingerprint
key_file=somelocation
tenancy=sometenancy
compartment = somecompartment
region=someregion
`
	c, e := parseConfigFile([]byte(data), "DEFAULT")

	assert.NoError(t, e)
	assert.Equal(t, c.UserOcid, tuser)
	assert.Equal(t, c.Fingerprint, tfingerprint)
	assert.Equal(t, c.KeyFilePath, tkeyfile)
	assert.Equal(t, c.TenancyOcid, ttenancy)
	assert.Equal(t, c.Region, tregion)
}

func TestFileConfigurationProvider_ParseEmptyFile(t *testing.T) {
	data := ``
	_, e := parseConfigFile([]byte(data), "DEFAULT")
	assert.Error(t, e)
}

func TestFileConfigurationProvider_FromFile(t *testing.T) {
	expected := []string{ttenancy, tuser, tfingerprint, tkeyfile}
	data := `[DEFAULT]
user=someuser
fingerprint=somefingerprint
key_file=somelocation
tenancy=sometenancy
compartment = somecompartment
region=someregion
`
	filename := writeTempFile(data)
	defer removeFileFn(filename)

	c := fileConfigurationProvider{ConfigPath: filename, Profile: "DEFAULT"}
	fns := []func() (string, error){c.TenancyOCID, c.UserOCID, c.KeyFingerprint}

	for i, fn := range fns {
		val, e := fn()
		assert.NoError(t, e)
		assert.Equal(t, expected[i], val)
	}
}

func TestFileConfigurationProvider_FromFileEmptyProfile(t *testing.T) {
	expected := []string{ttenancy, tuser, tfingerprint, tkeyfile}
	data := `
[DEFAULT]
user=a
fingerprint=a
key_file=a
tenancy=a
compartment = b
region=b

[]
user=someuser
fingerprint=somefingerprint
key_file=somelocation
tenancy=sometenancy
compartment = somecompartment
region=someregion
`
	filename := writeTempFile(data)
	defer removeFileFn(filename)

	c := fileConfigurationProvider{ConfigPath: filename, Profile: ""}
	fns := []func() (string, error){c.TenancyOCID, c.UserOCID, c.KeyFingerprint}

	for i, fn := range fns {
		val, e := fn()
		assert.NoError(t, e)
		assert.Equal(t, expected[i], val)
	}
}

func TestFileConfigurationProvider_FromFileBadConfig(t *testing.T) {
	data := `
user=someuser
fingerprint=somefingerprint
key_file=somelocation
tenancy=sometenancy
compartment = somecompartment
region=someregion
`
	filename := writeTempFile(data)
	defer removeFileFn(filename)

	c := fileConfigurationProvider{ConfigPath: filename, Profile: "PROFILE"}
	fns := []func() (string, error){c.TenancyOCID, c.UserOCID, c.KeyFingerprint}

	for _, fn := range fns {
		_, e := fn()
		assert.Error(t, e)
	}
}

func TestFileConfigurationProvider_FromFileMultipleProfiles(t *testing.T) {
	expected := []string{ttenancy, tuser, tfingerprint, tkeyfile}
	data := `
[DEFAULT]
user=a
fingerprint=a
key_file=a
tenancy=a
compartment = b
region=b

[PROFILE]
user=someuser
fingerprint=somefingerprint
key_file=somelocation
tenancy=sometenancy
compartment = somecompartment
region=someregion

[PROFILE2]
user=someuser
fingerprint=somefingerprint
key_file=somelocation
tenancy=sometenancy
compartment = somecompartment
region=someregion
`
	filename := writeTempFile(data)
	defer removeFileFn(filename)

	c := fileConfigurationProvider{ConfigPath: filename, Profile: "PROFILE"}
	fns := []func() (string, error){c.TenancyOCID, c.UserOCID, c.KeyFingerprint}

	for i, fn := range fns {
		val, e := fn()
		assert.NoError(t, e)
		assert.Equal(t, expected[i], val)
	}
}

func TestFileConfigurationProvider_NoFile(t *testing.T) {
	c := fileConfigurationProvider{ConfigPath: "/no/file"}
	fns := []func() (string, error){c.TenancyOCID, c.UserOCID, c.KeyFingerprint}

	for _, fn := range fns {
		_, e := fn()
		assert.Error(t, e)
	}
}

func TestFileConfigurationProvider_KeyProvider(t *testing.T) {
	dataTpl := `[DEFAULT]
user=someuser
fingerprint=somefingerprint
key_file=%s
tenancy=sometenancy
compartment = somecompartment
region=someregion
`

	keyFile := writeTempFile(testPrivateKeyConf)
	data := fmt.Sprintf(dataTpl, keyFile)
	tmpConfFile := writeTempFile(data)

	defer removeFileFn(tmpConfFile)
	defer removeFileFn(keyFile)

	c := fileConfigurationProvider{ConfigPath: tmpConfFile, Profile: "DEFAULT"}
	rskey, e := c.PrivateRSAKey()
	keyID, e1 := c.KeyID()
	assert.NoError(t, e)
	assert.NotEmpty(t, rskey)
	assert.NoError(t, e1)
	assert.NotEmpty(t, keyID)
}

func TestFileConfigurationProvider_FromFileFn(t *testing.T) {
	dataTpl := `[DEFAULT]
user=someuser
fingerprint=somefingerprint
key_file=%s
tenancy=sometenancy
compartment = somecompartment
region=someregion
`

	keyFile := writeTempFile(testPrivateKeyConf)
	data := fmt.Sprintf(dataTpl, keyFile)
	tmpConfFile := writeTempFile(data)

	defer removeFileFn(tmpConfFile)
	defer removeFileFn(keyFile)

	c, e0 := ConfigurationProviderFromFile(tmpConfFile, "")
	assert.NoError(t, e0)
	rskey, e := c.PrivateRSAKey()
	keyID, e1 := c.KeyID()
	assert.NoError(t, e)
	assert.NotEmpty(t, rskey)
	assert.NoError(t, e1)
	assert.NotEmpty(t, keyID)
}

func TestFileConfigurationProvider_FromFileAndProfile(t *testing.T) {
	dataTpl := `[DEFAULT]
user=someuser
fingerprint=somefingerprint
key_file=%s
tenancy=sometenancy
compartment = somecompartment
region=someregion

[PROFILE2]
user=user2
fingerprint=f2
key_file=%s
tenancy=tenancy2
compartment = compartment2
region=region2

`

	keyFile := writeTempFile(testPrivateKeyConf)
	data := fmt.Sprintf(dataTpl, keyFile, keyFile)
	tmpConfFile := writeTempFile(data)

	defer removeFileFn(tmpConfFile)
	defer removeFileFn(keyFile)

	c, e0 := ConfigurationProviderFromFile(tmpConfFile, "")
	assert.NoError(t, e0)
	rskey, e := c.PrivateRSAKey()
	keyID, e1 := c.KeyID()
	assert.NoError(t, e)
	assert.NotEmpty(t, rskey)
	assert.NoError(t, e1)
	assert.NotEmpty(t, keyID)

	c, e0 = ConfigurationProviderFromFileWithProfile(tmpConfFile, "PROFILE2", "")
	assert.NoError(t, e0)
	rskey, e = c.PrivateRSAKey()
	keyID, e1 = c.KeyID()
	assert.NoError(t, e)
	assert.NotEmpty(t, rskey)
	assert.NoError(t, e1)
	assert.NotEmpty(t, keyID)

}

func TestFileConfigurationProvider_FromFileIncomplete(t *testing.T) {
	dataTpl := `[DEFAULT]
user=someuser
fingerprint=somefingerprint
key_file=%s
compartment = somecompartment
region=someregion
`

	keyFile := writeTempFile(testPrivateKeyConf)
	data := fmt.Sprintf(dataTpl, keyFile)
	tmpConfFile := writeTempFile(data)

	defer removeFileFn(tmpConfFile)
	defer removeFileFn(keyFile)

	c, e0 := ConfigurationProviderFromFile(tmpConfFile, "")
	assert.NoError(t, e0)
	_, e1 := c.KeyID()
	assert.NoError(t, e1)
	_, e1 = c.TenancyOCID()
	assert.Error(t, e1)

}

func TestFileConfigurationProvider_FromFileIncomplete2(t *testing.T) {
	dataTpl := `[DEFAULT]
user=someuser
fingerprint=somefingerprint
key_file=%s
compartment = somecompartment
`

	keyFile := writeTempFile(testPrivateKeyConf)
	data := fmt.Sprintf(dataTpl, keyFile)
	tmpConfFile := writeTempFile(data)

	defer removeFileFn(tmpConfFile)
	defer removeFileFn(keyFile)

	c, e0 := ConfigurationProviderFromFile(tmpConfFile, "")
	assert.NoError(t, e0)
	_, e1 := c.KeyID()
	assert.NoError(t, e1)
	_, e1 = c.TenancyOCID()
	assert.Error(t, e1)
	_, e1 = c.Region()
	assert.Error(t, e1)

}

func TestComposingConfigurationProvider_MultipleFiles(t *testing.T) {
	dataTpl0 := ``
	dataTpl := `[DEFAULT]
user=someuser
fingerprint=somefingerprint
key_file=%s
tenancy=sometenancy
compartment = somecompartment
region=someregion
`

	keyFile := writeTempFile(testPrivateKeyConf)
	data := fmt.Sprintf(dataTpl, keyFile)
	tmpConfFile0 := writeTempFile(dataTpl0)
	tmpConfFile := writeTempFile(data)

	defer removeFileFn(tmpConfFile)
	defer removeFileFn(tmpConfFile0)
	defer removeFileFn(keyFile)

	c0, _ := ConfigurationProviderFromFile(tmpConfFile, "")
	c, _ := ConfigurationProviderFromFile(tmpConfFile, "")

	provider, ec := ComposingConfigurationProvider([]ConfigurationProvider{c0, c})
	assert.NoError(t, ec)
	fns := []func() (string, error){provider.TenancyOCID, provider.UserOCID, provider.KeyFingerprint}

	for _, fn := range fns {
		val, e := fn()
		assert.NoError(t, e)
		assert.NotEmpty(t, val)
	}
	key, _ := provider.PrivateRSAKey()
	assert.NotNil(t, key)
}

func TestComposingConfigurationProvider_MultipleFilesNoConf(t *testing.T) {
	dataTpl0 := ``
	dataTpl := ` `

	keyFile := writeTempFile(testPrivateKeyConf)
	data := fmt.Sprintf(dataTpl, keyFile)
	tmpConfFile0 := writeTempFile(dataTpl0)
	tmpConfFile := writeTempFile(data)

	defer removeFileFn(tmpConfFile)
	defer removeFileFn(tmpConfFile0)
	defer removeFileFn(keyFile)

	c0, _ := ConfigurationProviderFromFile(tmpConfFile, "")
	c, _ := ConfigurationProviderFromFile(tmpConfFile, "")

	provider, ec := ComposingConfigurationProvider([]ConfigurationProvider{c0, c})
	assert.NoError(t, ec)
	fns := []func() (string, error){provider.TenancyOCID, provider.UserOCID, provider.KeyFingerprint}

	for _, fn := range fns {
		_, e := fn()
		assert.Error(t, e)
	}
}

func TestComposingConfigurationProvider_FirstConfigWrong(t *testing.T) {
	dataTpl0 := ``
	dataTpl := `[DEFAULT]
user=someuser
fingerprint=somefingerprint
key_file=%s
tenancy=sometenancy
compartment = somecompartment
region=someregion
`

	keyFile := writeTempFile(testPrivateKeyConf)
	data := fmt.Sprintf(dataTpl, keyFile)
	tmpConfFile0 := writeTempFile(dataTpl0)
	tmpConfFile := writeTempFile(data)

	defer removeFileFn(tmpConfFile)
	defer removeFileFn(tmpConfFile0)
	defer removeFileFn(keyFile)

	c0, _ := ConfigurationProviderFromFile(tmpConfFile0, "")
	c1, _ := ConfigurationProviderFromFile("/dev/nowhere", "")
	p0 := ConfigurationProviderEnvironmentVariables("OCI", os.Getenv("BLAH"))
	c, _ := ConfigurationProviderFromFile(tmpConfFile, "")

	provider, ec := ComposingConfigurationProvider([]ConfigurationProvider{p0, c0, c1, c})
	assert.NoError(t, ec)
	ok, err := IsConfigurationProviderValid(provider)
	assert.NoError(t, err)
	assert.True(t, ok)

	fns := []func() (string, error){provider.TenancyOCID, provider.UserOCID, provider.KeyFingerprint}

	for _, fn := range fns {
		val, e := fn()
		assert.NoError(t, e)
		assert.NotEmpty(t, val)
	}
	key, _ := provider.PrivateRSAKey()
	assert.NotNil(t, key)
}

func TestComposingConfigurationProvider_NilConfiguration(t *testing.T) {
	dataTpl := `[DEFAULT]
user=someuser
fingerprint=somefingerprint
key_file=%s
tenancy=sometenancy
compartment = somecompartment
region=someregion
`

	keyFile := writeTempFile(testPrivateKeyConf)
	data := fmt.Sprintf(dataTpl, keyFile)
	tmpConfFile := writeTempFile(data)

	defer removeFileFn(tmpConfFile)
	defer removeFileFn(keyFile)

	c1, _ := ConfigurationProviderFromFile("/dev/nowhere", "")
	p0 := ConfigurationProviderEnvironmentVariables("OCI", os.Getenv("BLAH"))
	c, _ := ConfigurationProviderFromFile(tmpConfFile, "")

	_, ec := ComposingConfigurationProvider([]ConfigurationProvider{p0, nil, c1, c})
	assert.Error(t, ec)
}

func TestComposingConfigurationProvider_WithEncryptedKeyPassphraseInConfig(t *testing.T) {
	dataTpl := `[DEFAULT]
user=someuser
fingerprint=somefingerprint
key_file=%s
tenancy=sometenancy
compartment = somecompartment
region=someregion
pass_phrase=%s
`

	keyFile := writeTempFile(testEncryptedPrivateKeyConf)
	data := fmt.Sprintf(dataTpl, keyFile, testKeyPassphrase)
	tmpConfFile := writeTempFile(data)

	defer removeFileFn(tmpConfFile)
	defer removeFileFn(keyFile)

	provider, err := ConfigurationProviderFromFile(tmpConfFile, "")
	assert.NoError(t, err)
	ok, err := IsConfigurationProviderValid(provider)
	assert.NoError(t, err)
	assert.True(t, ok)

	fns := []func() (string, error){provider.TenancyOCID, provider.UserOCID, provider.KeyFingerprint}

	for _, fn := range fns {
		val, e := fn()
		assert.NoError(t, e)
		assert.NotEmpty(t, val)
	}

	key, err := provider.PrivateRSAKey()
	assert.NoError(t, err)
	assert.NotNil(t, key)
}

func TestComposingConfigurationProvider_WithEncryptedKeyOverridePassphrase(t *testing.T) {
	dataTpl := `[DEFAULT]
user=someuser
fingerprint=somefingerprint
key_file=%s
tenancy=sometenancy
compartment = somecompartment
region=someregion
passphrase=%s
`

	keyFile := writeTempFile(testEncryptedPrivateKeyConf)
	data := fmt.Sprintf(dataTpl, keyFile, "thewrongpassphrase")
	tmpConfFile := writeTempFile(data)

	defer removeFileFn(tmpConfFile)
	defer removeFileFn(keyFile)

	provider, err := ConfigurationProviderFromFile(tmpConfFile, testKeyPassphrase)
	assert.NoError(t, err)
	ok, err := IsConfigurationProviderValid(provider)
	assert.NoError(t, err)
	assert.True(t, ok)

	fns := []func() (string, error){provider.TenancyOCID, provider.UserOCID, provider.KeyFingerprint}

	for _, fn := range fns {
		val, e := fn()
		assert.NoError(t, e)
		assert.NotEmpty(t, val)
	}

	key, err := provider.PrivateRSAKey()
	assert.NoError(t, err)
	assert.NotNil(t, key)
}

func TestComposingConfigurationProvider_WithEncryptedKeyNoConfig(t *testing.T) {
	dataTpl := `[DEFAULT]
user=someuser
fingerprint=somefingerprint
key_file=%s
tenancy=sometenancy
compartment = somecompartment
region=someregion
`

	keyFile := writeTempFile(testEncryptedPrivateKeyConf)
	data := fmt.Sprintf(dataTpl, keyFile)
	tmpConfFile := writeTempFile(data)

	defer removeFileFn(tmpConfFile)
	defer removeFileFn(keyFile)

	provider, err := ConfigurationProviderFromFile(tmpConfFile, testKeyPassphrase)
	assert.NoError(t, err)
	ok, err := IsConfigurationProviderValid(provider)
	assert.NoError(t, err)
	assert.True(t, ok)

	fns := []func() (string, error){provider.TenancyOCID, provider.UserOCID, provider.KeyFingerprint}

	for _, fn := range fns {
		val, e := fn()
		assert.NoError(t, e)
		assert.NotEmpty(t, val)
	}

	key, err := provider.PrivateRSAKey()
	assert.NoError(t, err)
	assert.NotNil(t, key)
}

func TestConfigurationWithTilde(t *testing.T) {
	dataTpl := `[DEFAULT]
user=someuser
fingerprint=somefingerprint
key_file=%s
tenancy=sometenancy
compartment = somecompartment
region=someregion
`

	tmpKeyLocation := path.Join(getHomeFolder(), "testKey")
	e := ioutil.WriteFile(tmpKeyLocation, []byte(testEncryptedPrivateKeyConf), 777)
	if e != nil {
		assert.FailNow(t, e.Error())
	}

	newlocation := strings.Replace(tmpKeyLocation, getHomeFolder(), "~/", 1)
	data := fmt.Sprintf(dataTpl, newlocation)
	tmpConfFile := writeTempFile(data)

	defer removeFileFn(tmpConfFile)
	defer removeFileFn(tmpKeyLocation)

	provider, err := ConfigurationProviderFromFile(tmpConfFile, testKeyPassphrase)
	assert.NoError(t, err)
	ok, err := IsConfigurationProviderValid(provider)
	assert.NoError(t, err)
	assert.True(t, ok)

	fns := []func() (string, error){provider.TenancyOCID, provider.UserOCID, provider.KeyFingerprint}

	for _, fn := range fns {
		val, e := fn()
		assert.NoError(t, e)
		assert.NotEmpty(t, val)
	}

	key, err := provider.PrivateRSAKey()
	assert.NoError(t, err)
	assert.NotNil(t, key)
}

func TestExpandPath(t *testing.T) {
	home := getHomeFolder()
	testIO := []struct {
		name, inPath, expectedPath string
	}{
		{
			name:         "should expand tilde and return appended home dir",
			inPath:       "~/somepath",
			expectedPath: path.Join(home, "somepath"),
		},
		{
			name:         "should not do anything",
			inPath:       "/somepath/some/dir/~/file",
			expectedPath: "/somepath/some/dir/~/file",
		},
		{
			name:         "should replace one tilde only",
			inPath:       "~/~/some/path",
			expectedPath: path.Join(home, "~/some/path"),
		},
	}
	for _, tio := range testIO {
		t.Run(tio.name, func(t *testing.T) {
			p := expandPath(tio.inPath)
			assert.Equal(t, tio.expectedPath, p)
		})
	}
}
