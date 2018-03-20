package api

import (
	"fmt"
	"github.com/sacloud/libsacloud/sacloud"
)

// Error APIコール時のエラー情報
type Error interface {
	// errorインターフェースを内包
	error

	// エラー発生時のレスポンスコード
	ResponseCode() int

	// エラーコード
	Code() string

	// エラー発生時のメッセージ
	Message() string

	// エラー追跡用シリアルコード
	Serial() string

	// エラー(オリジナル)
	OrigErr() *sacloud.ResultErrorValue
}

// NewError APIコール時のエラー情報
func NewError(responseCode int, err *sacloud.ResultErrorValue) Error {
	return &apiError{
		responseCode: responseCode,
		origErr:      err,
	}
}

type apiError struct {
	responseCode int
	origErr      *sacloud.ResultErrorValue
}

// Error errorインターフェース
func (e *apiError) Error() string {
	return fmt.Sprintf("Error in response: %#v", e.origErr)
}

// ResponseCode エラー発生時のレスポンスコード
func (e *apiError) ResponseCode() int {
	return e.responseCode
}

// Code エラーコード
func (e *apiError) Code() string {
	if e.origErr != nil {
		return e.origErr.ErrorCode
	}
	return ""
}

// Message エラー発生時のメッセージ(
func (e *apiError) Message() string {
	if e.origErr != nil {
		return e.origErr.ErrorMessage
	}
	return ""
}

// Serial エラー追跡用シリアルコード
func (e *apiError) Serial() string {
	if e.origErr != nil {
		return e.origErr.Serial
	}
	return ""
}

// OrigErr エラー(オリジナル)
func (e *apiError) OrigErr() *sacloud.ResultErrorValue {
	return e.origErr
}
