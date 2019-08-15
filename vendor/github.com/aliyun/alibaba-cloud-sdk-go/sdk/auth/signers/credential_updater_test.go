package signers

import (
	"fmt"
	"testing"
	"time"

	"github.com/aliyun/alibaba-cloud-sdk-go/sdk/requests"
	"github.com/aliyun/alibaba-cloud-sdk-go/sdk/responses"
	"github.com/stretchr/testify/assert"
)

func TestCredentialUpdater_NeedUpdateCredential(t *testing.T) {
	// default
	updater := &credentialUpdater{}
	assert.NotNil(t, updater)
	assert.True(t, updater.needUpdateCredential())

	// no need update
	updater = &credentialUpdater{
		inAdvanceScale:       1.0,
		lastUpdateTimestamp:  time.Now().Unix() - 4000,
		credentialExpiration: 5000,
	}
	assert.NotNil(t, updater)
	assert.False(t, updater.needUpdateCredential())

	// need update
	updater = &credentialUpdater{
		inAdvanceScale:       1.0,
		lastUpdateTimestamp:  time.Now().Unix() - 10000,
		credentialExpiration: 5000,
	}
	assert.NotNil(t, updater)
	assert.True(t, updater.needUpdateCredential())
}

func TestCredentialUpdater_UpdateCredential(t *testing.T) {
	updater := &credentialUpdater{}
	assert.NotNil(t, updater)
	updater.buildRequestMethod = func() (*requests.CommonRequest, error) {
		return nil, fmt.Errorf("build request method failed")
	}

	err := updater.updateCredential()
	assert.NotNil(t, err)
	assert.Equal(t, "build request method failed", err.Error())

	updater.buildRequestMethod = func() (*requests.CommonRequest, error) {
		return requests.NewCommonRequest(), nil
	}
	updater.refreshApi = func(request *requests.CommonRequest) (response *responses.CommonResponse, err error) {
		return nil, fmt.Errorf("refresh api executed fail")
	}

	err = updater.updateCredential()
	assert.NotNil(t, err)
	assert.Equal(t, "refresh api executed fail", err.Error())

	updater.refreshApi = func(request *requests.CommonRequest) (response *responses.CommonResponse, err error) {
		return responses.NewCommonResponse(), nil
	}

	updater.responseCallBack = func(response *responses.CommonResponse) error {
		return fmt.Errorf("response callback fail")
	}

	err = updater.updateCredential()
	assert.NotNil(t, err)
	// update timestamp
	assert.True(t, time.Now().Unix()-updater.lastUpdateTimestamp < 10)
	assert.Equal(t, "response callback fail", err.Error())

	updater.responseCallBack = func(response *responses.CommonResponse) error {
		return nil
	}

	err = updater.updateCredential()
	assert.Nil(t, err)
}
