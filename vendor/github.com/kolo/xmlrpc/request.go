package xmlrpc

import (
	"bytes"
	"fmt"
	"net/http"
)

func NewRequest(url string, method string, args interface{}) (*http.Request, error) {
	var t []interface{}
	var ok bool
	if t, ok = args.([]interface{}); !ok {
		if args != nil {
			t = []interface{}{args}
		}
	}

	body, err := EncodeMethodCall(method, t...)
	if err != nil {
		return nil, err
	}

	request, err := http.NewRequest("POST", url, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}

	request.Header.Set("Content-Type", "text/xml")
	request.Header.Set("Content-Length", fmt.Sprintf("%d", len(body)))

	return request, nil
}

func EncodeMethodCall(method string, args ...interface{}) ([]byte, error) {
	var b bytes.Buffer
	b.WriteString(`<?xml version="1.0" encoding="UTF-8"?>`)
	b.WriteString(fmt.Sprintf("<methodCall><methodName>%s</methodName>", method))

	if args != nil {
		b.WriteString("<params>")

		for _, arg := range args {
			p, err := marshal(arg)
			if err != nil {
				return nil, err
			}

			b.WriteString(fmt.Sprintf("<param>%s</param>", string(p)))
		}

		b.WriteString("</params>")
	}

	b.WriteString("</methodCall>")

	return b.Bytes(), nil
}
