/*
 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use this file except in compliance with the License.
 * You may obtain a copy of the License at
 *
 * http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
 */

package errors

import "fmt"

const (
	DefaultClientErrorStatus = 400
	DefaultClientErrorCode   = "SDK.ClientError"

	UnsupportedCredentialErrorCode    = "SDK.UnsupportedCredential"
	UnsupportedCredentialErrorMessage = "Specified credential (type = %s) is not supported, please check"

	CanNotResolveEndpointErrorCode    = "SDK.CanNotResolveEndpoint"
	CanNotResolveEndpointErrorMessage = "Can not resolve endpoint(param = %s), please check your accessKey with secret, and read the user guide\n %s"

	UnsupportedParamPositionErrorCode    = "SDK.UnsupportedParamPosition"
	UnsupportedParamPositionErrorMessage = "Specified param position (%s) is not supported, please upgrade sdk and retry"

	AsyncFunctionNotEnabledCode    = "SDK.AsyncFunctionNotEnabled"
	AsyncFunctionNotEnabledMessage = "Async function is not enabled in client, please invoke 'client.EnableAsync' function"

	UnknownRequestTypeErrorCode    = "SDK.UnknownRequestType"
	UnknownRequestTypeErrorMessage = "Unknown Request Type: %s"

	MissingParamErrorCode = "SDK.MissingParam"
	InvalidParamErrorCode = "SDK.InvalidParam"

	JsonUnmarshalErrorCode    = "SDK.JsonUnmarshalError"
	JsonUnmarshalErrorMessage = "Failed to unmarshal response, but you can get the data via response.GetHttpStatusCode() and response.GetHttpContentString()"

	TimeoutErrorCode    = "SDK.TimeoutError"
	TimeoutErrorMessage = "The request timed out %s times(%s for retry), perhaps we should have the threshold raised a little?"
)

type ClientError struct {
	errorCode   string
	message     string
	originError error
}

func NewClientError(errorCode, message string, originErr error) Error {
	return &ClientError{
		errorCode:   errorCode,
		message:     message,
		originError: originErr,
	}
}

func (err *ClientError) Error() string {
	clientErrMsg := fmt.Sprintf("[%s] %s", err.ErrorCode(), err.message)
	if err.originError != nil {
		return clientErrMsg + "\ncaused by:\n" + err.originError.Error()
	}
	return clientErrMsg
}

func (err *ClientError) OriginError() error {
	return err.originError
}

func (*ClientError) HttpStatus() int {
	return DefaultClientErrorStatus
}

func (err *ClientError) ErrorCode() string {
	if err.errorCode == "" {
		return DefaultClientErrorCode
	} else {
		return err.errorCode
	}
}

func (err *ClientError) Message() string {
	return err.message
}

func (err *ClientError) String() string {
	return err.Error()
}
