/*
Copyright 2015 Google LLC

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package main

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"strconv"
	"strings"
	"unicode"

	"cloud.google.com/go/bigtable"
)

// Parse a GC policy. Valid policies include
//     never
//     maxage = 5d
//     maxversions = 3
//     maxage = 5d || maxversions = 3
//     maxage=30d || (maxage=3d && maxversions=100)
func parseGCPolicy(s string) (bigtable.GCPolicy, error) {
	if strings.TrimSpace(s) == "never" {
		return bigtable.NoGcPolicy(), nil
	}
	r := strings.NewReader(s)
	p, err := parsePolicyExpr(r)
	if err != nil {
		return nil, fmt.Errorf("invalid GC policy: %v", err)
	}
	tok, err := getToken(r)
	if err != nil {
		return nil, err
	}
	if tok != "" {
		return nil, fmt.Errorf("invalid GC policy: want end of input, got %q", tok)
	}
	return p, nil
}

// expr ::= term (op term)*
// op ::= "and" | "or" | "&&" | "||"
func parsePolicyExpr(r io.RuneScanner) (bigtable.GCPolicy, error) {
	policy, err := parsePolicyTerm(r)
	if err != nil {
		return nil, err
	}
	for {
		tok, err := getToken(r)
		if err != nil {
			return nil, err
		}
		var f func(...bigtable.GCPolicy) bigtable.GCPolicy
		switch tok {
		case "and", "&&":
			f = bigtable.IntersectionPolicy
		case "or", "||":
			f = bigtable.UnionPolicy
		default:
			ungetToken(tok)
			return policy, nil
		}
		p2, err := parsePolicyTerm(r)
		if err != nil {
			return nil, err
		}
		policy = f(policy, p2)
	}
}

// term ::= "maxage" "=" duration | "maxversions" "=" int | "(" policy ")"
func parsePolicyTerm(r io.RuneScanner) (bigtable.GCPolicy, error) {
	tok, err := getToken(r)
	if err != nil {
		return nil, err
	}
	switch tok {
	case "":
		return nil, errors.New("empty GC policy term")

	case "maxage", "maxversions":
		if err := expectToken(r, "="); err != nil {
			return nil, err
		}
		tok2, err := getToken(r)
		if err != nil {
			return nil, err
		}
		if tok2 == "" {
			return nil, errors.New("expected a token after '='")
		}
		if tok == "maxage" {
			dur, err := parseDuration(tok2)
			if err != nil {
				return nil, err
			}
			return bigtable.MaxAgePolicy(dur), nil
		}
		n, err := strconv.ParseUint(tok2, 10, 16)
		if err != nil {
			return nil, err
		}
		return bigtable.MaxVersionsPolicy(int(n)), nil

	case "(":
		p, err := parsePolicyExpr(r)
		if err != nil {
			return nil, err
		}
		if err := expectToken(r, ")"); err != nil {
			return nil, err
		}
		return p, nil

	default:
		return nil, fmt.Errorf("unexpected token: %q", tok)
	}

}

func expectToken(r io.RuneScanner, want string) error {
	got, err := getToken(r)
	if err != nil {
		return err
	}
	if got != want {
		return fmt.Errorf("expected %q, saw %q", want, got)
	}
	return nil
}

const noToken = "_" // empty token is valid, so use "_" instead

// If not noToken, getToken will return this instead of reading a new token
// from the input.
var ungotToken = noToken

// getToken extracts the first token from the input. Valid tokens include
// any sequence of letters and digits, and these symbols: &&, ||, =, ( and ).
// getToken returns ("", nil) at end of input.
func getToken(r io.RuneScanner) (string, error) {
	if ungotToken != noToken {
		t := ungotToken
		ungotToken = noToken
		return t, nil
	}
	var err error
	// Skip leading whitespace.
	c := ' '
	for unicode.IsSpace(c) {
		c, _, err = r.ReadRune()
		if err == io.EOF {
			return "", nil
		}
		if err != nil {
			return "", err
		}
	}
	switch {
	case c == '=' || c == '(' || c == ')':
		return string(c), nil

	case c == '&' || c == '|':
		c2, _, err := r.ReadRune()
		if err != nil && err != io.EOF {
			return "", err
		}
		if c != c2 {
			return "", fmt.Errorf("expected %c%c", c, c)
		}
		return string([]rune{c, c}), nil

	case unicode.IsLetter(c) || unicode.IsDigit(c):
		// Collect an alphanumeric token.
		var b bytes.Buffer
		for unicode.IsLetter(c) || unicode.IsDigit(c) {
			b.WriteRune(c)
			c, _, err = r.ReadRune()
			if err == io.EOF {
				break
			}
			if err != nil {
				return "", err
			}
		}
		r.UnreadRune()
		return b.String(), nil

	default:
		return "", fmt.Errorf("bad rune %q", c)
	}
}

// "unget" a token so the next call to getToken will return it.
func ungetToken(tok string) {
	if ungotToken != noToken {
		panic("ungetToken called twice")
	}
	ungotToken = tok
}
