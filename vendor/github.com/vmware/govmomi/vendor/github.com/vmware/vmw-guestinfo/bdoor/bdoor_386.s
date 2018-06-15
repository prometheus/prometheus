#include "textflag.h"

// Doc of the golang plan9 assembler
// http://p9.nyx.link/labs/sys/doc/asm.html
//
// A good primer of how to write golang with some plan9 flavored assembly
// http://www.doxsey.net/blog/go-and-assembly
//
// Some x86 references
// http://www.eecg.toronto.edu/~amza/www.mindsec.com/files/x86regs.html
// https://cseweb.ucsd.edu/classes/sp10/cse141/pdf/02/S01_x86_64.key.pdf
// https://en.wikibooks.org/wiki/X86_Assembly/Other_Instructions
//
// (This one is invaluable.  Has a working example of how a standard function
// call looks on the stack with the associated assembly.)
// https://www.recurse.com/blog/7-understanding-c-by-learning-assembly
//
// Reference with raw form of the Opcode
// http://x86.renejeschke.de/html/file_module_x86_id_139.html
//
// Massive x86_64 reference
// http://ref.x86asm.net/coder64.html#xED
//
// Adding instructions to the go assembler
// https://blog.klauspost.com/adding-unsupported-instructions-in-golang-assembler/
//
// Backdoor commands
// https://sites.google.com/site/chitchatvmback/backdoor

// func bdoor_inout(ax, bx, cx, dx, si, di, bp uint32) (retax, retbx, retcx, retdx, retsi, retdi, retbp uint32)
TEXT ·bdoor_inout(SB), NOSPLIT|WRAPPER, $0
	MOVL ax+0(FP), AX
	MOVL bx+4(FP), BX
	MOVL cx+8(FP), CX
	MOVL dx+12(FP), DX
	MOVL si+16(FP), SI
	MOVL di+20(FP), DI
	MOVL bp+24(FP), BP

	// IN to DX from EAX
	INL

	MOVL AX, retax+28(FP)
	MOVL BX, retbx+32(FP)
	MOVL CX, retcx+36(FP)
	MOVL DX, retdx+40(FP)
	MOVL SI, retsi+44(FP)
	MOVL DI, retdi+48(FP)
	MOVL BP, retbp+52(FP)
	RET

// func bdoor_hbout(ax, bx, cx, dx, si, di, bp uint32) (retax, retbx, retcx, retdx, retsi, retdi, retbp uint32)
TEXT ·bdoor_hbout(SB), NOSPLIT|WRAPPER, $0
	MOVL ax+0(FP), AX
	MOVL bx+4(FP), BX
	MOVL cx+8(FP), CX
	MOVL dx+12(FP), DX
	MOVL si+16(FP), SI
	MOVL di+20(FP), DI
	MOVL bp+24(FP), BP

	CLD; REP; OUTSB

	MOVL AX, retax+28(FP)
	MOVL BX, retbx+32(FP)
	MOVL CX, retcx+36(FP)
	MOVL DX, retdx+40(FP)
	MOVL SI, retsi+44(FP)
	MOVL DI, retdi+48(FP)
	MOVL BP, retbp+52(FP)
	RET

// func bdoor_hbin(ax, bx, cx, dx, si, di, bp uint32) (retax, retbx, retcx, retdx, retsi, retdi, retbp uint32)
TEXT ·bdoor_hbin(SB), NOSPLIT|WRAPPER, $0
	MOVL ax+0(FP), AX
	MOVL bx+4(FP), BX
	MOVL cx+8(FP), CX
	MOVL dx+12(FP), DX
	MOVL si+16(FP), SI
	MOVL di+20(FP), DI
	MOVL bp+24(FP), BP

	CLD; REP; INSB

	MOVL AX, retax+28(FP)
	MOVL BX, retbx+32(FP)
	MOVL CX, retcx+40(FP)
	MOVL DX, retdx+44(FP)
	MOVL SI, retsi+48(FP)
	MOVL DI, retdi+52(FP)
	MOVL BP, retbp+56(FP)
	RET

// func bdoor_inout_test(ax, bx, cx, dx, si, di, bp uint32) (retax, retbx, retcx, retdx, retsi, retdi, retbp uint32)
TEXT ·bdoor_inout_test(SB), NOSPLIT|WRAPPER, $0
	MOVL ax+0(FP), AX
	MOVL bx+4(FP), BX
	MOVL cx+8(FP), CX
	MOVL dx+12(FP), DX
	MOVL si+16(FP), SI
	MOVL di+20(FP), DI
	MOVL bp+24(FP), BP

	MOVL AX, retax+28(FP)
	MOVL BX, retbx+32(FP)
	MOVL CX, retcx+36(FP)
	MOVL DX, retdx+40(FP)
	MOVL SI, retsi+44(FP)
	MOVL DI, retdi+48(FP)
	MOVL BP, retbp+52(FP)
	RET

