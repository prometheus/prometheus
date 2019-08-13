// +build amd64,!noasm

package sip13

//go:generate python -m peachpy.x86_64 sip13.py -S -o sip13_amd64.s -mabi=goasm
//go:noescape

func Sum64(k0, k1 uint64, p []byte) uint64

//go:noescape
func Sum64Str(k0, k1 uint64, p string) uint64
