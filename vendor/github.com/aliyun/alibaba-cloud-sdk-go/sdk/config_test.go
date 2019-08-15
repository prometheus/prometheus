package sdk

import (
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func Test_Config(t *testing.T) {
	config := NewConfig()
	assert.NotNil(t, config, "NewConfig failed")
	assert.Equal(t, true, config.AutoRetry, "Default AutoRetry should be true")
	assert.Equal(t, 3, config.MaxRetryTime, "Default MaxRetryTime should be 3")
	assert.Equal(t, "", config.UserAgent, "Default UserAgent should be empty")
	assert.Equal(t, false, config.Debug, "Default AutoRetry should be false")
	assert.Equal(t, time.Duration(0), config.Timeout, "Default Timeout should be 10000000000")
	assert.Equal(t, (*http.Transport)(nil), config.HttpTransport, "Default HttpTransport should be nil")
	assert.Equal(t, false, config.EnableAsync, "Default EnableAsync should be false")
	assert.Equal(t, 1000, config.MaxTaskQueueSize, "Default MaxTaskQueueSize should be 1000")
	assert.Equal(t, 5, config.GoRoutinePoolSize, "Default GoRoutinePoolSize should be 5")
	assert.Equal(t, "HTTP", config.Scheme, "Default Scheme should be HTTP")

	transport := &http.Transport{
		MaxIdleConns:       10,
		IdleConnTimeout:    30 * time.Second,
		DisableCompression: true,
	}
	config.
		WithAutoRetry(false).
		WithMaxRetryTime(0).
		WithUserAgent("new user agent").
		WithDebug(true).
		WithTimeout(time.Duration(500000)).
		WithHttpTransport(transport).
		WithEnableAsync(true).
		WithMaxTaskQueueSize(1).
		WithGoRoutinePoolSize(10).
		WithScheme("HTTPS")

	assert.Equal(t, 0, config.MaxRetryTime)
	assert.Equal(t, false, config.AutoRetry)
	assert.Equal(t, "new user agent", config.UserAgent)
	assert.Equal(t, true, config.Debug)
	assert.Equal(t, time.Duration(500000), config.Timeout)
	assert.Equal(t, transport, config.HttpTransport)
	assert.Equal(t, true, config.EnableAsync)
	assert.Equal(t, 1, config.MaxTaskQueueSize)
	assert.Equal(t, 10, config.GoRoutinePoolSize)
	assert.Equal(t, "HTTPS", config.Scheme)
}
