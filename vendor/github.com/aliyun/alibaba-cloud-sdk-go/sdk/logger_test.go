package sdk

import (
	"github.com/stretchr/testify/assert"
	"testing"
)

func Test_OpenLogger(t *testing.T) {
	client, err := NewClientWithAccessKey("regionid", "acesskeyid", "accesskeysecret")
	assert.Nil(t, err)
	assert.NotNil(t, client)
	assert.Equal(t, true, client.isRunning)

	client.OpenLogger()
	assert.Equal(t, true, client.logger.isOpen)
}

func Test_SetTemplate(t *testing.T) {
	client, err := NewClientWithAccessKey("regionid", "acesskeyid", "accesskeysecret")
	assert.Nil(t, err)
	assert.NotNil(t, client)
	assert.Equal(t, true, client.isRunning)

	template := "{time}"
	client.SetTemplate(template)
	assert.Equal(t, "{time}", client.logger.formatTemplate)
}

func Test_GetTemplate(t *testing.T) {
	client, err := NewClientWithAccessKey("regionid", "acesskeyid", "accesskeysecret")
	assert.Nil(t, err)
	assert.NotNil(t, client)
	assert.Equal(t, true, client.isRunning)

	assert.Equal(t, defaultLoggerTemplate, client.GetTemplate())
}

func Test_GetLoggerMsg(t *testing.T) {
	client, err := NewClientWithAccessKey("regionid", "acesskeyid", "accesskeysecret")
	assert.Nil(t, err)
	assert.NotNil(t, client)
	assert.Equal(t, true, client.isRunning)

	assert.Equal(t, "", client.GetLoggerMsg())
}
