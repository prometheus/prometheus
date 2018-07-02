// Copyright (c) 2016, 2018, Oracle and/or its affiliates. All rights reserved.

package auth

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

type jwtToken struct {
	raw     string
	header  map[string]interface{}
	payload map[string]interface{}
}

func (t *jwtToken) expired() bool {
	exp := int64(t.payload["exp"].(float64))
	return exp <= time.Now().Unix()
}

func parseJwt(tokenString string) (*jwtToken, error) {
	parts := strings.Split(tokenString, ".")
	if len(parts) != 3 {
		return nil, fmt.Errorf("the given token string contains an invalid number of parts")
	}

	token := &jwtToken{raw: tokenString}
	var err error

	// Parse Header part
	var headerBytes []byte
	if headerBytes, err = decodePart(parts[0]); err != nil {
		return nil, fmt.Errorf("failed to decode the header bytes: %s", err.Error())
	}
	if err = json.Unmarshal(headerBytes, &token.header); err != nil {
		return nil, err
	}

	// Parse Payload part
	var payloadBytes []byte
	if payloadBytes, err = decodePart(parts[1]); err != nil {
		return nil, fmt.Errorf("failed to decode the payload bytes: %s", err.Error())
	}
	decoder := json.NewDecoder(bytes.NewBuffer(payloadBytes))
	if err = decoder.Decode(&token.payload); err != nil {
		return nil, fmt.Errorf("failed to decode the payload json: %s", err.Error())
	}

	return token, nil
}

func decodePart(partString string) ([]byte, error) {
	if l := len(partString) % 4; 0 < l {
		partString += strings.Repeat("=", 4-l)
	}
	return base64.URLEncoding.DecodeString(partString)
}
