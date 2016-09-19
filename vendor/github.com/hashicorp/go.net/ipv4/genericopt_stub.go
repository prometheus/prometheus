// Copyright 2012 The Go Authors.  All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// +build nacl plan9 solaris

package ipv4

func (c *genericOpt) TOS() (int, error) {
	// TODO(mikio): Implement this
	return 0, errOpNoSupport
}

func (c *genericOpt) SetTOS(tos int) error {
	// TODO(mikio): Implement this
	return errOpNoSupport
}

func (c *genericOpt) TTL() (int, error) {
	// TODO(mikio): Implement this
	return 0, errOpNoSupport
}

func (c *genericOpt) SetTTL(ttl int) error {
	// TODO(mikio): Implement this
	return errOpNoSupport
}
