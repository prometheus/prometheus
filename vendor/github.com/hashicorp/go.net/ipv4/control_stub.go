// Copyright 2012 The Go Authors.  All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// +build nacl plan9 solaris

package ipv4

func setControlMessage(fd int, opt *rawOpt, cf ControlFlags, on bool) error {
	// TODO(mikio): Implement this
	return errOpNoSupport
}

func newControlMessage(opt *rawOpt) []byte {
	// TODO(mikio): Implement this
	return nil
}

func parseControlMessage(b []byte) (*ControlMessage, error) {
	// TODO(mikio): Implement this
	return nil, errOpNoSupport
}

func marshalControlMessage(cm *ControlMessage) []byte {
	// TODO(mikio): Implement this
	return nil
}
