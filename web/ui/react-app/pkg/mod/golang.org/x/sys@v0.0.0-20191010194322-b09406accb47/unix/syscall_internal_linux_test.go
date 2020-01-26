// Copyright 2019 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// +build linux

package unix

import (
	"reflect"
	"testing"
	"unsafe"
)

func Test_anyToSockaddr(t *testing.T) {
	tests := []struct {
		name string
		rsa  *RawSockaddrAny
		sa   Sockaddr
		err  error
	}{
		{
			name: "AF_TIPC bad addrtype",
			rsa: &RawSockaddrAny{
				Addr: RawSockaddr{
					Family: AF_TIPC,
				},
			},
			err: EINVAL,
		},
		{
			name: "AF_TIPC NameSeq",
			rsa: (*RawSockaddrAny)(unsafe.Pointer(&RawSockaddrTIPC{
				Family:   AF_TIPC,
				Addrtype: TIPC_SERVICE_RANGE,
				Scope:    1,
				Addr: (&TIPCServiceRange{
					Type:  1,
					Lower: 2,
					Upper: 3,
				}).tipcAddr(),
			})),
			sa: &SockaddrTIPC{
				Scope: 1,
				Addr: &TIPCServiceRange{
					Type:  1,
					Lower: 2,
					Upper: 3,
				},
			},
		},
		{
			name: "AF_TIPC Name",
			rsa: (*RawSockaddrAny)(unsafe.Pointer(&RawSockaddrTIPC{
				Family:   AF_TIPC,
				Addrtype: TIPC_SERVICE_ADDR,
				Scope:    2,
				Addr: (&TIPCServiceName{
					Type:     1,
					Instance: 2,
					Domain:   3,
				}).tipcAddr(),
			})),
			sa: &SockaddrTIPC{
				Scope: 2,
				Addr: &TIPCServiceName{
					Type:     1,
					Instance: 2,
					Domain:   3,
				},
			},
		},
		{
			name: "AF_TIPC ID",
			rsa: (*RawSockaddrAny)(unsafe.Pointer(&RawSockaddrTIPC{
				Family:   AF_TIPC,
				Addrtype: TIPC_SOCKET_ADDR,
				Scope:    3,
				Addr: (&TIPCSocketAddr{
					Ref:  1,
					Node: 2,
				}).tipcAddr(),
			})),
			sa: &SockaddrTIPC{
				Scope: 3,
				Addr: &TIPCSocketAddr{
					Ref:  1,
					Node: 2,
				},
			},
		},
		{
			name: "AF_MAX EAFNOSUPPORT",
			rsa: &RawSockaddrAny{
				Addr: RawSockaddr{
					Family: AF_MAX,
				},
			},
			err: EAFNOSUPPORT,
		},
		// TODO: expand to support other families.
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// TODO: parameterize fd (and its setup) when needed.
			sa, err := anyToSockaddr(0, tt.rsa)
			if err != tt.err {
				t.Fatalf("unexpected error: %v, want: %v", err, tt.err)
			}

			if !reflect.DeepEqual(sa, tt.sa) {
				t.Fatalf("unexpected Sockaddr:\n got: %#v\nwant: %#v", sa, tt.sa)
			}
		})
	}
}

func TestSockaddrTIPC_sockaddr(t *testing.T) {
	tests := []struct {
		name string
		sa   *SockaddrTIPC
		raw  *RawSockaddrTIPC
		err  error
	}{
		{
			name: "no fields set",
			sa:   &SockaddrTIPC{},
			err:  EINVAL,
		},
		{
			name: "ID",
			sa: &SockaddrTIPC{
				Scope: 1,
				Addr: &TIPCSocketAddr{
					Ref:  1,
					Node: 2,
				},
			},
			raw: &RawSockaddrTIPC{
				Family:   AF_TIPC,
				Addrtype: TIPC_SOCKET_ADDR,
				Scope:    1,
				Addr: (&TIPCSocketAddr{
					Ref:  1,
					Node: 2,
				}).tipcAddr(),
			},
		},
		{
			name: "NameSeq",
			sa: &SockaddrTIPC{
				Scope: 2,
				Addr: &TIPCServiceRange{
					Type:  1,
					Lower: 2,
					Upper: 3,
				},
			},
			raw: &RawSockaddrTIPC{
				Family:   AF_TIPC,
				Addrtype: TIPC_SERVICE_RANGE,
				Scope:    2,
				Addr: (&TIPCServiceRange{
					Type:  1,
					Lower: 2,
					Upper: 3,
				}).tipcAddr(),
			},
		},
		{
			name: "Name",
			sa: &SockaddrTIPC{
				Scope: 3,
				Addr: &TIPCServiceName{
					Type:     1,
					Instance: 2,
					Domain:   3,
				},
			},
			raw: &RawSockaddrTIPC{
				Family:   AF_TIPC,
				Addrtype: TIPC_SERVICE_ADDR,
				Scope:    3,
				Addr: (&TIPCServiceName{
					Type:     1,
					Instance: 2,
					Domain:   3,
				}).tipcAddr(),
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			out, l, err := tt.sa.sockaddr()
			if err != tt.err {
				t.Fatalf("unexpected error: %v, want: %v", err, tt.err)
			}

			// Must be 0 on error or a fixed size otherwise.
			if (tt.err != nil && l != 0) || (tt.raw != nil && l != SizeofSockaddrTIPC) {
				t.Fatalf("unexpected Socklen: %d", l)
			}
			if out == nil {
				// No pointer to cast, return early.
				return
			}

			raw := (*RawSockaddrTIPC)(out)
			if !reflect.DeepEqual(raw, tt.raw) {
				t.Fatalf("unexpected RawSockaddrTIPC:\n got: %#v\nwant: %#v", raw, tt.raw)
			}
		})
	}
}
