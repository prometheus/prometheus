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

import (
	"encoding/json"
	"fmt"

	"github.com/jmespath/go-jmespath"
)

var wrapperList = []ServerErrorWrapper{
	&SignatureDostNotMatchWrapper{},
}

type ServerError struct {
	httpStatus int
	requestId  string
	hostId     string
	errorCode  string
	recommend  string
	message    string
	comment    string
}

type ServerErrorWrapper interface {
	tryWrap(error *ServerError, wrapInfo map[string]string) bool
}

func (err *ServerError) Error() string {
	return fmt.Sprintf("SDK.ServerError\nErrorCode: %s\nRecommend: %s\nRequestId: %s\nMessage: %s",
		err.errorCode, err.comment+err.recommend, err.requestId, err.message)
}

func NewServerError(httpStatus int, responseContent, comment string) Error {
	result := &ServerError{
		httpStatus: httpStatus,
		message:    responseContent,
		comment:    comment,
	}

	var data interface{}
	err := json.Unmarshal([]byte(responseContent), &data)
	if err == nil {
		requestId, _ := jmespath.Search("RequestId", data)
		hostId, _ := jmespath.Search("HostId", data)
		errorCode, _ := jmespath.Search("Code", data)
		recommend, _ := jmespath.Search("Recommend", data)
		message, _ := jmespath.Search("Message", data)

		if requestId != nil {
			result.requestId = requestId.(string)
		}
		if hostId != nil {
			result.hostId = hostId.(string)
		}
		if errorCode != nil {
			result.errorCode = errorCode.(string)
		}
		if recommend != nil {
			result.recommend = recommend.(string)
		}
		if message != nil {
			result.message = message.(string)
		}
	}

	return result
}

func WrapServerError(originError *ServerError, wrapInfo map[string]string) *ServerError {
	for _, wrapper := range wrapperList {
		ok := wrapper.tryWrap(originError, wrapInfo)
		if ok {
			return originError
		}
	}
	return originError
}

func (err *ServerError) HttpStatus() int {
	return err.httpStatus
}

func (err *ServerError) ErrorCode() string {
	return err.errorCode
}

func (err *ServerError) Message() string {
	return err.message
}

func (err *ServerError) OriginError() error {
	return nil
}

func (err *ServerError) HostId() string {
	return err.hostId
}

func (err *ServerError) RequestId() string {
	return err.requestId
}

func (err *ServerError) Recommend() string {
	return err.recommend
}

func (err *ServerError) Comment() string {
	return err.comment
}
