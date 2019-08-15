package errors

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestClientError(t *testing.T) {
	err := NewClientError("error code", "error message", nil)
	assert.Equal(t, "[error code] error message", err.Error())
	assert.Equal(t, 400, err.HttpStatus())
	assert.Equal(t, "error code", err.ErrorCode())
	assert.Equal(t, "error message", err.Message())
	clientError, ok := err.(*ClientError)
	assert.True(t, ok)
	assert.Equal(t, "[error code] error message", clientError.String())
	assert.Nil(t, err.OriginError())
}

type OriginError struct {
}

func (err *OriginError) Error() string {
	return "The origin error"
}

func TestClientErrorWithOriginError(t *testing.T) {
	err := NewClientError("", "error message", &OriginError{})
	assert.Equal(t, "[SDK.ClientError] error message\ncaused by:\nThe origin error", err.Error())
	assert.Equal(t, 400, err.HttpStatus())
	assert.Equal(t, "SDK.ClientError", err.ErrorCode())
	assert.Equal(t, "error message", err.Message())
	clientError, ok := err.(*ClientError)
	assert.True(t, ok)
	assert.Equal(t, "[SDK.ClientError] error message\ncaused by:\nThe origin error", clientError.String())
	assert.NotNil(t, err.OriginError())
}
