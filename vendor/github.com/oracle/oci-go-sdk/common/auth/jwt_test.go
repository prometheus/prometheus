// Copyright (c) 2016, 2018, Oracle and/or its affiliates. All rights reserved.

package auth

import (
	"github.com/stretchr/testify/assert"
	"testing"
)

const validJwtTokenString = `eyJhbGciOiJSUzI1NiIsImtpZCI6ImFzdyIsInR5cCI6IkpXVCJ9.eyJhdWQiOiJvcGMub3JhY2xlLmNvbSIsImV4cCI6MTUxMTgzODc5MywiaWF0IjoxNTExODE3MTkzLCJpc3MiOiJhdXRoU2VydmljZS5vcmFjbGUuY29tIiwib3BjLWNlcnR0eXBlIjoiaW5zdGFuY2UiLCJvcGMtY29tcGFydG1lbnQiOiJvY2lkMS5jb21wYXJ0bWVudC5vYzEuLmJsdWhibHVoYmx1aCIsIm9wYy1pbnN0YW5jZSI6Im9jaWQxLmluc3RhbmNlLm9jMS5waHguYmx1aGJsdWhibHVoIiwib3BjLXRlbmFudCI6Im9jaWR2MTp0ZW5hbmN5Om9jMTpwaHg6MTIzNDU2Nzg5MDpibHVoYmx1aGJsdWgiLCJwdHlwZSI6Imluc3RhbmNlIiwic3ViIjoib2NpZDEuaW5zdGFuY2Uub2MxLnBoeC5ibHVoYmx1aGJsdWgiLCJ0ZW5hbnQiOiJvY2lkdjE6dGVuYW5jeTpvYzE6cGh4OjEyMzQ1Njc4OTA6Ymx1aGJsdWhibHVoIiwidHR5cGUiOiJ4NTA5In0.zen7q2yJSpMjzH4ym_H7VEwZA0-vTT4Wcild-HRfLxX6A1ej4tlpACa7A24j5JoZYI4mHooZVJ8e7ZezFenK0zZx5j8RbIjsqJKwroYXExOiBXLCUwMWOLXIndEsUzzGLqnPfKHXd80vrhMLmtkVTCJqBMzvPUSYkH_ciWgmjP9m0YETdQ9ifghkADhZGt9IlnOswg0s3Bx9ASwxFZEtom0BmU9GwEuITTTZfKvndk785BlNeZMOjhovaD97-LYpv5B_PiWEz8zialK5zxjijLCw06zyA8CQRQqmVCagNUPilfz_BcPyImzvFDuzQcPyDkTcsB7weX35tafHmA_Ulg`

func TestJwtToken_ParseJwt(t *testing.T) {
	token, err := parseJwt(validJwtTokenString)

	assert.NoError(t, err)

	expectedHeader := map[string]interface{}{
		"alg": "RS256",
		"kid": "asw",
		"typ": "JWT",
	}
	assert.Equal(t, expectedHeader, token.header)

	expectedPayload := map[string]interface{}{
		"aud":             "opc.oracle.com",
		"exp":             float64(1511838793),
		"iat":             float64(1511817193),
		"iss":             "authService.oracle.com",
		"opc-certtype":    "instance",
		"opc-compartment": "ocid1.compartment.oc1..bluhbluhbluh",
		"opc-instance":    "ocid1.instance.oc1.phx.bluhbluhbluh",
		"opc-tenant":      "ocidv1:tenancy:oc1:phx:1234567890:bluhbluhbluh",
		"ptype":           "instance",
		"sub":             "ocid1.instance.oc1.phx.bluhbluhbluh",
		"tenant":          "ocidv1:tenancy:oc1:phx:1234567890:bluhbluhbluh",
		"ttype":           "x509",
	}
	assert.Equal(t, expectedPayload, token.payload)

	assert.Equal(t, true, token.expired())
}

const headerMissingJwtTokenString = `eyJhdWQiOiJvcGMub3JhY2xlLmNvbSIsImV4cCI6MTUxMTgzODc5MywiaWF0IjoxNTExODE3MTkzLCJpc3MiOiJhdXRoU2VydmljZS5vcmFjbGUuY29tIiwib3BjLWNlcnR0eXBlIjoiaW5zdGFuY2UiLCJvcGMtY29tcGFydG1lbnQiOiJvY2lkMS5jb21wYXJ0bWVudC5vYzEuLmJsdWhibHVoYmx1aCIsIm9wYy1pbnN0YW5jZSI6Im9jaWQxLmluc3RhbmNlLm9jMS5waHguYmx1aGJsdWhibHVoIiwib3BjLXRlbmFudCI6Im9jaWR2MTp0ZW5hbmN5Om9jMTpwaHg6MTIzNDU2Nzg5MDpibHVoYmx1aGJsdWgiLCJwdHlwZSI6Imluc3RhbmNlIiwic3ViIjoib2NpZDEuaW5zdGFuY2Uub2MxLnBoeC5ibHVoYmx1aGJsdWgiLCJ0ZW5hbnQiOiJvY2lkdjE6dGVuYW5jeTpvYzE6cGh4OjEyMzQ1Njc4OTA6Ymx1aGJsdWhibHVoIiwidHR5cGUiOiJ4NTA5In0.zen7q2yJSpMjzH4ym_H7VEwZA0-vTT4Wcild-HRfLxX6A1ej4tlpACa7A24j5JoZYI4mHooZVJ8e7ZezFenK0zZx5j8RbIjsqJKwroYXExOiBXLCUwMWOLXIndEsUzzGLqnPfKHXd80vrhMLmtkVTCJqBMzvPUSYkH_ciWgmjP9m0YETdQ9ifghkADhZGt9IlnOswg0s3Bx9ASwxFZEtom0BmU9GwEuITTTZfKvndk785BlNeZMOjhovaD97-LYpv5B_PiWEz8zialK5zxjijLCw06zyA8CQRQqmVCagNUPilfz_BcPyImzvFDuzQcPyDkTcsB7weX35tafHmA_Ulg`

func TestJwtToken_ParseJwtInvalidNumberOfParts(t *testing.T) {
	token, err := parseJwt(headerMissingJwtTokenString)

	assert.Nil(t, token)
	assert.EqualError(t, err, "the given token string contains an invalid number of parts")
}

const invalidHeaderJwtTokenString = `INVALIDHEADER.eyJhdWQiOiJvcGMub3JhY2xlLmNvbSIsImV4cCI6MTUxMTgzODc5MywiaWF0IjoxNTExODE3MTkzLCJpc3MiOiJhdXRoU2VydmljZS5vcmFjbGUuY29tIiwib3BjLWNlcnR0eXBlIjoiaW5zdGFuY2UiLCJvcGMtY29tcGFydG1lbnQiOiJvY2lkMS5jb21wYXJ0bWVudC5vYzEuLmJsdWhibHVoYmx1aCIsIm9wYy1pbnN0YW5jZSI6Im9jaWQxLmluc3RhbmNlLm9jMS5waHguYmx1aGJsdWhibHVoIiwib3BjLXRlbmFudCI6Im9jaWR2MTp0ZW5hbmN5Om9jMTpwaHg6MTIzNDU2Nzg5MDpibHVoYmx1aGJsdWgiLCJwdHlwZSI6Imluc3RhbmNlIiwic3ViIjoib2NpZDEuaW5zdGFuY2Uub2MxLnBoeC5ibHVoYmx1aGJsdWgiLCJ0ZW5hbnQiOiJvY2lkdjE6dGVuYW5jeTpvYzE6cGh4OjEyMzQ1Njc4OTA6Ymx1aGJsdWhibHVoIiwidHR5cGUiOiJ4NTA5In0.zen7q2yJSpMjzH4ym_H7VEwZA0-vTT4Wcild-HRfLxX6A1ej4tlpACa7A24j5JoZYI4mHooZVJ8e7ZezFenK0zZx5j8RbIjsqJKwroYXExOiBXLCUwMWOLXIndEsUzzGLqnPfKHXd80vrhMLmtkVTCJqBMzvPUSYkH_ciWgmjP9m0YETdQ9ifghkADhZGt9IlnOswg0s3Bx9ASwxFZEtom0BmU9GwEuITTTZfKvndk785BlNeZMOjhovaD97-LYpv5B_PiWEz8zialK5zxjijLCw06zyA8CQRQqmVCagNUPilfz_BcPyImzvFDuzQcPyDkTcsB7weX35tafHmA_Ulg`

func TestJwtToken_ParseJwtInvalidHeader(t *testing.T) {
	token, err := parseJwt(invalidHeaderJwtTokenString)

	assert.Nil(t, token)
	assert.Error(t, err)
}

const invalidPayloadJwtTokenString = `eyJhbGciOiJSUzI1NiIsImtpZCI6ImFzdyIsInR5cCI6IkpXVCJ9.INVALIDPAYLOAD.zen7q2yJSpMjzH4ym_H7VEwZA0-vTT4Wcild-HRfLxX6A1ej4tlpACa7A24j5JoZYI4mHooZVJ8e7ZezFenK0zZx5j8RbIjsqJKwroYXExOiBXLCUwMWOLXIndEsUzzGLqnPfKHXd80vrhMLmtkVTCJqBMzvPUSYkH_ciWgmjP9m0YETdQ9ifghkADhZGt9IlnOswg0s3Bx9ASwxFZEtom0BmU9GwEuITTTZfKvndk785BlNeZMOjhovaD97-LYpv5B_PiWEz8zialK5zxjijLCw06zyA8CQRQqmVCagNUPilfz_BcPyImzvFDuzQcPyDkTcsB7weX35tafHmA_Ulg`

func TestJwtToken_ParseJwtInvalidPayload(t *testing.T) {
	token, err := parseJwt(invalidPayloadJwtTokenString)

	assert.Nil(t, token)
	assert.Error(t, err)
}
