// +build amd64,!appengine,!popcntgo

#define POPCNTQ_DX_DX BYTE $0xf3; BYTE $0x48; BYTE $0x0f; BYTE $0xb8; BYTE $0xd2

// func Popcnt(x uint64) uint64

TEXT ·Popcnt(SB),4,$0-16
        MOVQ  x+0(FP), DX
        POPCNTQ_DX_DX
        MOVQ DX, ret+8(FP)
        RET
