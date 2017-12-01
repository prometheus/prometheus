// +build amd64,!appengine

// func Clz(x uint64) uint64
TEXT ·Clz(SB),4,$0-16
        BSRQ  x+0(FP), AX
        JZ zero
        SUBQ  $63, AX
        NEGQ AX
        MOVQ AX, ret+8(FP)
        RET
zero:
        MOVQ $64, ret+8(FP)
        RET
