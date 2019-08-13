// +build amd64,!noasm

package sip13

//go:generate go run asm.go -out sip13_amd64.s
//go:noescape

func Sum64(k0, k1 uint64, p []byte) uint64

//go:noescape
func Sum64Str(k0, k1 uint64, p string) uint64
