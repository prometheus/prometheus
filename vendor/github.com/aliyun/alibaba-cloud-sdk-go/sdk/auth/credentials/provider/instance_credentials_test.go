package provider_test

import (
	"os"
	"testing"

	"github.com/aliyun/alibaba-cloud-sdk-go/sdk/auth/credentials"

	"github.com/stretchr/testify/assert"

	"github.com/aliyun/alibaba-cloud-sdk-go/sdk/auth/credentials/provider"
)

func TestInstanceCredentialsProvider(t *testing.T) {
	//testcase 1
	//return nil, nil
	p := provider.NewInstanceCredentialsProvider()
	c, err := p.Resolve()
	assert.Nil(t, c)
	assert.Nil(t, err)

	//testcase 2
	//return nil, errors.New("Environmental variable 'ALIBABA_CLOUD_ECS_METADATA' are empty")
	os.Setenv(provider.ENVEcsMetadata, "")
	c, err = p.Resolve()
	assert.Nil(t, c)
	assert.EqualError(t, err, "Environmental variable 'ALIBABA_CLOUD_ECS_METADATA' are empty")

	//testcase 3
	//return nil, err
	os.Setenv(provider.ENVEcsMetadata, "test")
	c, err = p.Resolve()
	assert.Nil(t, c)
	assert.NotNil(t, err)

	//testcase 4
	//return nil, fmt.Errorf("The role was not found in the instance")
	provider.HookGet = func(fn func(string) (int, []byte, error)) func(string) (int, []byte, error) {
		return func(string) (int, []byte, error) {
			return 404, []byte(""), nil
		}
	}
	c, err = p.Resolve()
	assert.Nil(t, c)
	assert.EqualError(t, err, "The role was not found in the instance")

	//testcase 5
	//return nil, fmt.Errorf("Received %d when getting security credentials for %s", status, roleName)
	provider.HookGet = func(fn func(string) (int, []byte, error)) func(string) (int, []byte, error) {
		return func(string) (int, []byte, error) {
			return 400, []byte(""), nil
		}
	}
	c, err = p.Resolve()
	assert.Nil(t, c)
	assert.EqualError(t, err, "Received 400 when getting security credentials for test")

	//testcase 6
	//json unmarshal error
	provider.HookGet = func(fn func(string) (int, []byte, error)) func(string) (int, []byte, error) {
		return func(string) (int, []byte, error) {
			return 200, []byte(`{
				AccessKSTS.*******
			  }`), nil
		}
	}
	c, err = p.Resolve()
	assert.Nil(t, c)
	assert.NotNil(t, err)

	//testcase 7, AccessKeyId does not receive
	provider.HookGet = func(fn func(string) (int, []byte, error)) func(string) (int, []byte, error) {
		return func(string) (int, []byte, error) {
			return 200, []byte(`{
				"AccessKeySecret" : "*******",
				"Expiration" : "2019-01-28T15:15:56Z",
				"SecurityToken" : "SecurityToken",
				"LastUpdated" : "2019-01-28T09:15:55Z",
				"Code" : "Success"
			  }`), nil
		}
	}
	c, err = p.Resolve()
	assert.Nil(t, c)
	assert.EqualError(t, err, "AccessKeyId not in map")

	//testcase 8,AccessKeySecret does not receive
	provider.HookGet = func(fn func(string) (int, []byte, error)) func(string) (int, []byte, error) {
		return func(string) (int, []byte, error) {
			return 200, []byte(`{
				"AccessKeyId" : "STS.*******",
				"Expiration" : "2019-01-28T15:15:56Z",
				"SecurityToken" : "SecurityToken",
				"LastUpdated" : "2019-01-28T09:15:55Z",
				"Code" : "Success"
			  }`), nil
		}
	}
	c, err = p.Resolve()
	assert.Nil(t, c)
	assert.EqualError(t, err, "AccessKeySecret not in map")

	//testcase 9, SecurityToken does not receive
	provider.HookGet = func(fn func(string) (int, []byte, error)) func(string) (int, []byte, error) {
		return func(string) (int, []byte, error) {
			return 200, []byte(`{
				"AccessKeyId" : "STS.*******",
				"AccessKeySecret" : "*******",
				"Expiration" : "2019-01-28T15:15:56Z",
				"LastUpdated" : "2019-01-28T09:15:55Z",
				"Code" : "Success"
			  }`), nil
		}
	}
	c, err = p.Resolve()
	assert.Nil(t, c)
	assert.EqualError(t, err, "SecurityToken not in map")

	//testcase, normal receive
	provider.HookGet = func(fn func(string) (int, []byte, error)) func(string) (int, []byte, error) {
		return func(string) (int, []byte, error) {
			return 200, []byte(`{
				"AccessKeyId" : "STS.*******",
				"AccessKeySecret" : "*******",
				"Expiration" : "2019-01-28T15:15:56Z",
				"SecurityToken" : "SecurityToken",
				"LastUpdated" : "2019-01-28T09:15:55Z",
				"Code" : "Success"
			  }`), nil
		}
	}
	c, err = p.Resolve()
	assert.Nil(t, err)
	assert.Equal(t, credentials.NewStsTokenCredential("STS.*******", "*******", "SecurityToken"), c)

}
