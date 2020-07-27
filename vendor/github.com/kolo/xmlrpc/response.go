package xmlrpc

import (
	"fmt"
	"regexp"
)

var (
	faultRx = regexp.MustCompile(`<fault>(\s|\S)+</fault>`)
)

// FaultError is returned from the server when an invalid call is made
type FaultError struct {
	Code   int    `xmlrpc:"faultCode"`
	String string `xmlrpc:"faultString"`
}

// Error implements the error interface
func (e FaultError) Error() string {
	return fmt.Sprintf("Fault(%d): %s", e.Code, e.String)
}

type Response []byte

func (r Response) Err() error {
	if !faultRx.Match(r) {
		return nil
	}
	var fault FaultError
	if err := unmarshal(r, &fault); err != nil {
		return err
	}
	return fault
}

func (r Response) Unmarshal(v interface{}) error {
	if err := unmarshal(r, v); err != nil {
		return err
	}

	return nil
}
