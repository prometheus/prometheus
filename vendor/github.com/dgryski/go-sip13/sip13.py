import peachpy.x86_64

k0 = Argument(uint64_t)
k1 = Argument(uint64_t)
p_base = Argument(ptr())
p_len = Argument(int64_t)
p_cap = Argument(int64_t)

# siphash 1-3
cROUND = 1
dROUND = 3


def sipround(v0, v1, v2, v3):
    ADD(v0, v1)
    ADD(v2, v3)
    ROL(v1, 13)
    ROL(v3, 16)
    XOR(v1, v0)
    XOR(v3, v2)

    ROL(v0, 32)

    ADD(v2, v1)
    ADD(v0, v3)
    ROL(v1, 17)
    ROL(v3, 21)
    XOR(v1, v2)
    XOR(v3, v0)

    ROL(v2, 32)


def makeSip(name, args):

    with Function(name, args, uint64_t, target=uarch.default) as function:

        reg_v0 = GeneralPurposeRegister64()
        reg_v1 = GeneralPurposeRegister64()
        reg_v2 = GeneralPurposeRegister64()
        reg_v3 = GeneralPurposeRegister64()

        LOAD.ARGUMENT(reg_v0, k0)
        MOV(reg_v2, reg_v0)
        LOAD.ARGUMENT(reg_v1, k1)
        MOV(reg_v3, reg_v1)

        reg_magic = GeneralPurposeRegister64()
        MOV(reg_magic, 0x736f6d6570736575)
        XOR(reg_v0, reg_magic)
        MOV(reg_magic, 0x646f72616e646f6d)
        XOR(reg_v1, reg_magic)
        MOV(reg_magic, 0x6c7967656e657261)
        XOR(reg_v2, reg_magic)
        MOV(reg_magic, 0x7465646279746573)
        XOR(reg_v3, reg_magic)

        reg_p = GeneralPurposeRegister64()
        reg_p_len = GeneralPurposeRegister64()
        LOAD.ARGUMENT(reg_p, p_base)
        LOAD.ARGUMENT(reg_p_len, p_len)

        reg_b = GeneralPurposeRegister64()
        MOV(reg_b, reg_p_len)
        SHL(reg_b, 56)

        reg_m = GeneralPurposeRegister64()

        loop = Loop()

        CMP(reg_p_len, 8)
        JL(loop.end)
        with loop:
            MOV(reg_m, [reg_p])

            XOR(reg_v3, reg_m)
            for _ in range(0, cROUND):
                sipround(reg_v0, reg_v1, reg_v2, reg_v3)
            XOR(reg_v0, reg_m)

            ADD(reg_p, 8)
            SUB(reg_p_len, 8)
            CMP(reg_p_len, 8)
            JGE(loop.begin)

        # no support for jump tables
        labels = [Label("sw%d" % i) for i in range(0, 8)]

        for i in range(0, 7):
            CMP(reg_p_len, i)
            JE(labels[i])

        char = GeneralPurposeRegister64()
        for i in range(7, 0, -1):
            LABEL(labels[i])
            MOVZX(char, byte[reg_p + i - 1])
            SHL(char, (i - 1) * 8)
            OR(reg_b, char)

        LABEL(labels[0])

        XOR(reg_v3, reg_b)
        for _ in range(0, cROUND):
            sipround(reg_v0, reg_v1, reg_v2, reg_v3)
        XOR(reg_v0, reg_b)

        XOR(reg_v2, 0xff)
        for _ in range(0, dROUND):
            sipround(reg_v0, reg_v1, reg_v2, reg_v3)

        XOR(reg_v0, reg_v1)
        XOR(reg_v2, reg_v3)
        XOR(reg_v0, reg_v2)

        RETURN(reg_v0)


makeSip("Sum64", (k0, k1, p_base, p_len, p_cap))
makeSip("Sum64Str", (k0, k1, p_base, p_len))
