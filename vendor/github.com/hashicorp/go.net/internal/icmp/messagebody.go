// Copyright 2012 The Go Authors.  All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package icmp

// A MessageBody represents an ICMP message body.
type MessageBody interface {
	// Len returns the length of ICMP message body.
	Len() int

	// Marshal returns the binary enconding of ICMP message body.
	Marshal() ([]byte, error)
}

// A DefaultMessageBody represents the default message body.
type DefaultMessageBody struct {
	Data []byte // data
}

// Len implements the Len method of MessageBody interface.
func (p *DefaultMessageBody) Len() int {
	if p == nil {
		return 0
	}
	return len(p.Data)
}

// Marshal implements the Marshal method of MessageBody interface.
func (p *DefaultMessageBody) Marshal() ([]byte, error) {
	return p.Data, nil
}
