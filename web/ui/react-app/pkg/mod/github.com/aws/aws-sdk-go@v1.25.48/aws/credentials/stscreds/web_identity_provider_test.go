// +build go1.7

package stscreds_test

import (
	"net/http"
	"reflect"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/awserr"
	"github.com/aws/aws-sdk-go/aws/corehandlers"
	"github.com/aws/aws-sdk-go/aws/credentials"
	"github.com/aws/aws-sdk-go/aws/credentials/stscreds"
	"github.com/aws/aws-sdk-go/aws/request"
	"github.com/aws/aws-sdk-go/awstesting/unit"
	"github.com/aws/aws-sdk-go/service/sts"
)

func TestWebIdentityProviderRetrieve(t *testing.T) {
	var reqCount int
	cases := []struct {
		name              string
		onSendReq         func(*testing.T, *request.Request)
		roleARN           string
		tokenFilepath     string
		sessionName       string
		expectedError     error
		expectedCredValue credentials.Value
	}{
		{
			name:          "session name case",
			roleARN:       "arn01234567890123456789",
			tokenFilepath: "testdata/token.jwt",
			sessionName:   "foo",
			onSendReq: func(t *testing.T, r *request.Request) {
				input := r.Params.(*sts.AssumeRoleWithWebIdentityInput)
				if e, a := "foo", *input.RoleSessionName; !reflect.DeepEqual(e, a) {
					t.Errorf("expected %v, but received %v", e, a)
				}

				data := r.Data.(*sts.AssumeRoleWithWebIdentityOutput)
				*data = sts.AssumeRoleWithWebIdentityOutput{
					Credentials: &sts.Credentials{
						Expiration:      aws.Time(time.Now()),
						AccessKeyId:     aws.String("access-key-id"),
						SecretAccessKey: aws.String("secret-access-key"),
						SessionToken:    aws.String("session-token"),
					},
				}
			},
			expectedCredValue: credentials.Value{
				AccessKeyID:     "access-key-id",
				SecretAccessKey: "secret-access-key",
				SessionToken:    "session-token",
				ProviderName:    stscreds.WebIdentityProviderName,
			},
		},
		{
			name:          "invalid token retry",
			roleARN:       "arn01234567890123456789",
			tokenFilepath: "testdata/token.jwt",
			sessionName:   "foo",
			onSendReq: func(t *testing.T, r *request.Request) {
				input := r.Params.(*sts.AssumeRoleWithWebIdentityInput)
				if e, a := "foo", *input.RoleSessionName; !reflect.DeepEqual(e, a) {
					t.Errorf("expected %v, but received %v", e, a)
				}

				if reqCount == 0 {
					r.HTTPResponse.StatusCode = 400
					r.Error = awserr.New(sts.ErrCodeInvalidIdentityTokenException,
						"some error message", nil)
					return
				}

				data := r.Data.(*sts.AssumeRoleWithWebIdentityOutput)
				*data = sts.AssumeRoleWithWebIdentityOutput{
					Credentials: &sts.Credentials{
						Expiration:      aws.Time(time.Now()),
						AccessKeyId:     aws.String("access-key-id"),
						SecretAccessKey: aws.String("secret-access-key"),
						SessionToken:    aws.String("session-token"),
					},
				}
			},
			expectedCredValue: credentials.Value{
				AccessKeyID:     "access-key-id",
				SecretAccessKey: "secret-access-key",
				SessionToken:    "session-token",
				ProviderName:    stscreds.WebIdentityProviderName,
			},
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			reqCount = 0

			svc := sts.New(unit.Session, &aws.Config{
				Logger: t,
			})
			svc.Handlers.Send.Swap(corehandlers.SendHandler.Name, request.NamedHandler{
				Name: "custom send stub handler",
				Fn: func(r *request.Request) {
					r.HTTPResponse = &http.Response{
						StatusCode: 200, Header: http.Header{},
					}
					c.onSendReq(t, r)
					reqCount++
				},
			})
			svc.Handlers.UnmarshalMeta.Clear()
			svc.Handlers.Unmarshal.Clear()
			svc.Handlers.UnmarshalError.Clear()

			p := stscreds.NewWebIdentityRoleProvider(svc, c.roleARN, c.sessionName, c.tokenFilepath)
			credValue, err := p.Retrieve()
			if e, a := c.expectedError, err; !reflect.DeepEqual(e, a) {
				t.Errorf("expected %v, but received %v", e, a)
			}

			if e, a := c.expectedCredValue, credValue; !reflect.DeepEqual(e, a) {
				t.Errorf("expected %v, but received %v", e, a)
			}
		})
	}
}
