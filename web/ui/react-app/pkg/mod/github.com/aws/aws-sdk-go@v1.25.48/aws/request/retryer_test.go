package request

import (
	"errors"
	"fmt"
	"testing"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/awserr"
	"github.com/aws/aws-sdk-go/aws/client/metadata"
)

func TestRequestThrottling(t *testing.T) {
	req := Request{}

	req.Error = awserr.New("Throttling", "", nil)
	if e, a := true, req.IsErrorThrottle(); e != a {
		t.Errorf("expect %t to be throttled, was %t", e, a)
	}
}

type mockTempError bool

func (e mockTempError) Error() string {
	return fmt.Sprintf("mock temporary error: %t", e.Temporary())
}
func (e mockTempError) Temporary() bool {
	return bool(e)
}

func TestIsErrorRetryable(t *testing.T) {
	cases := []struct {
		Err       error
		Retryable bool
	}{
		{
			Err:       awserr.New(ErrCodeSerialization, "temporary error", mockTempError(true)),
			Retryable: true,
		},
		{
			Err:       awserr.New(ErrCodeSerialization, "temporary error", mockTempError(false)),
			Retryable: false,
		},
		{
			Err:       awserr.New(ErrCodeSerialization, "some error", errors.New("blah")),
			Retryable: false,
		},
		{
			Err:       awserr.New("SomeError", "some error", nil),
			Retryable: false,
		},
		{
			Err:       awserr.New("RequestError", "some error", nil),
			Retryable: true,
		},
		{
			Err:       nil,
			Retryable: false,
		},
	}

	for i, c := range cases {
		retryable := IsErrorRetryable(c.Err)
		if e, a := c.Retryable, retryable; e != a {
			t.Errorf("%d, expect %t temporary error, got %t", i, e, a)
		}
	}
}

func TestRequest_NilRetyer(t *testing.T) {
	clientInfo := metadata.ClientInfo{Endpoint: "https://mock.region.amazonaws.com"}
	req := New(aws.Config{}, clientInfo, Handlers{}, nil, &Operation{}, nil, nil)

	if req.Retryer == nil {
		t.Fatalf("expect retryer to be set")
	}

	if e, a := 0, req.MaxRetries(); e != a {
		t.Errorf("expect no retries, got %v", a)
	}
}
