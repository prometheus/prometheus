package jwt

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strings"
)

type Parser struct {
	ValidMethods  []string // If populated, only these methods will be considered valid
	UseJSONNumber bool     // Use JSON Number format in JSON decoder
}

// Parse, validate, and return a token.
// keyFunc will receive the parsed token and should return the key for validating.
// If everything is kosher, err will be nil
func (p *Parser) Parse(tokenString string, keyFunc Keyfunc) (*Token, error) {
	parts := strings.Split(tokenString, ".")
	if len(parts) != 3 {
		return nil, &ValidationError{err: "token contains an invalid number of segments", Errors: ValidationErrorMalformed}
	}

	var err error
	token := &Token{Raw: tokenString}
	// parse Header
	var headerBytes []byte
	if headerBytes, err = DecodeSegment(parts[0]); err != nil {
		if strings.HasPrefix(strings.ToLower(tokenString), "bearer ") {
			return token, &ValidationError{err: "tokenstring should not contain 'bearer '", Errors: ValidationErrorMalformed}
		}
		return token, &ValidationError{err: err.Error(), Errors: ValidationErrorMalformed}
	}
	if err = json.Unmarshal(headerBytes, &token.Header); err != nil {
		return token, &ValidationError{err: err.Error(), Errors: ValidationErrorMalformed}
	}

	// parse Claims
	var claimBytes []byte
	if claimBytes, err = DecodeSegment(parts[1]); err != nil {
		return token, &ValidationError{err: err.Error(), Errors: ValidationErrorMalformed}
	}
	dec := json.NewDecoder(bytes.NewBuffer(claimBytes))
	if p.UseJSONNumber {
		dec.UseNumber()
	}
	if err = dec.Decode(&token.Claims); err != nil {
		return token, &ValidationError{err: err.Error(), Errors: ValidationErrorMalformed}
	}

	// Lookup signature method
	if method, ok := token.Header["alg"].(string); ok {
		if token.Method = GetSigningMethod(method); token.Method == nil {
			return token, &ValidationError{err: "signing method (alg) is unavailable.", Errors: ValidationErrorUnverifiable}
		}
	} else {
		return token, &ValidationError{err: "signing method (alg) is unspecified.", Errors: ValidationErrorUnverifiable}
	}

	// Verify signing method is in the required set
	if p.ValidMethods != nil {
		var signingMethodValid = false
		var alg = token.Method.Alg()
		for _, m := range p.ValidMethods {
			if m == alg {
				signingMethodValid = true
				break
			}
		}
		if !signingMethodValid {
			// signing method is not in the listed set
			return token, &ValidationError{err: fmt.Sprintf("signing method %v is invalid", alg), Errors: ValidationErrorSignatureInvalid}
		}
	}

	// Lookup key
	var key interface{}
	if keyFunc == nil {
		// keyFunc was not provided.  short circuiting validation
		return token, &ValidationError{err: "no Keyfunc was provided.", Errors: ValidationErrorUnverifiable}
	}
	if key, err = keyFunc(token); err != nil {
		// keyFunc returned an error
		return token, &ValidationError{err: err.Error(), Errors: ValidationErrorUnverifiable}
	}

	// Check expiration times
	vErr := &ValidationError{}
	now := TimeFunc().Unix()
	if exp, ok := token.Claims["exp"].(float64); ok {
		if now > int64(exp) {
			vErr.err = "token is expired"
			vErr.Errors |= ValidationErrorExpired
		}
	}
	if nbf, ok := token.Claims["nbf"].(float64); ok {
		if now < int64(nbf) {
			vErr.err = "token is not valid yet"
			vErr.Errors |= ValidationErrorNotValidYet
		}
	}

	// Perform validation
	token.Signature = parts[2]
	if err = token.Method.Verify(strings.Join(parts[0:2], "."), token.Signature, key); err != nil {
		vErr.err = err.Error()
		vErr.Errors |= ValidationErrorSignatureInvalid
	}

	if vErr.valid() {
		token.Valid = true
		return token, nil
	}

	return token, vErr
}
