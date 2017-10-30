package eureka

import (
	"encoding/json"
	"fmt"
)

const (
	ErrCodeEurekaNotReachable = 501
)

var (
	errorMap = map[int]string{
		ErrCodeEurekaNotReachable: "All the given peers are not reachable",
	}
)

type EurekaError struct {
	ErrorCode int    `json:"errorCode"`
	Message   string `json:"message"`
	Cause     string `json:"cause,omitempty"`
	Index     uint64 `json:"index"`
}

func (e EurekaError) Error() string {
	return fmt.Sprintf("%v: %v (%v) [%v]", e.ErrorCode, e.Message, e.Cause, e.Index)
}

func newError(errorCode int, cause string, index uint64) *EurekaError {
	return &EurekaError{
		ErrorCode: errorCode,
		Message:   errorMap[errorCode],
		Cause:     cause,
		Index:     index,
	}
}

func handleError(b []byte) error {
	eurekaErr := new(EurekaError)

	err := json.Unmarshal(b, eurekaErr)
	if err != nil {
		logger.Warning("cannot unmarshal eureka error: %v", err)
		return err
	}

	return eurekaErr
}
