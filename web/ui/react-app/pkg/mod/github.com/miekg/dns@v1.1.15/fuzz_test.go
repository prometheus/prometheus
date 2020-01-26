package dns

import (
	"net"
	"testing"
)

// TestPackDataOpt tests generated using fuzz.go and with a message pack
// containing the following bytes:
// "0000\x00\x00000000\x00\x00/00000" +
// "0\x00\v\x00#\b00000000\x00\x00)000" +
// "000\x00\x1c00\x00\x0000\x00\x01000\x00\x00\x00\b" +
// "\x00\v\x00\x02\x0000000000"
// That bytes sequence created the overflow error.
func TestPackDataOpt(t *testing.T) {
	type args struct {
		option []EDNS0
		msg    []byte
		off    int
	}
	tests := []struct {
		name       string
		args       args
		want       int
		wantErr    bool
		wantErrMsg string
	}{
		{
			name: "overflow",
			args: args{
				option: []EDNS0{
					&EDNS0_LOCAL{Code: 0x3030, Data: []uint8{}},
					&EDNS0_LOCAL{Code: 0x3030, Data: []uint8{0x30}},
					&EDNS0_LOCAL{Code: 0x3030, Data: []uint8{}},
					&EDNS0_SUBNET{
						Code: 0x0, Family: 0x2,
						SourceNetmask: 0x0, SourceScope: 0x30,
						Address: net.IP{0x30, 0x30, 0x30, 0x30, 0x30, 0x30, 0x30, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0}},
				},
				msg: []byte{
					0x30, 0x30, 0x30, 0x30, 0x00, 0x00, 0x00, 0x2,
					0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x2f, 0x30,
					0x30, 0x30, 0x30, 0x30, 0x30, 0x00, 0x0b, 0x00,
					0x23, 0x08, 0x30, 0x30, 0x30, 0x30, 0x30, 0x30,
					0x30, 0x30, 0x00, 0x00, 0x29, 0x30, 0x30, 0x30,
					0x30, 0x30, 0x30, 0x00, 0x00, 0x30, 0x30, 0x00,
					0x00, 0x30, 0x30, 0x00, 0x01, 0x30, 0x00, 0x00,
					0x00,
				},
				off: 54,
			},
			wantErr:    true,
			wantErrMsg: "dns: overflow packing opt",
			want:       57,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := packDataOpt(tt.args.option, tt.args.msg, tt.args.off)
			if (err != nil) != tt.wantErr {
				t.Errorf("packDataOpt() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if err != nil && tt.wantErrMsg != err.Error() {
				t.Errorf("packDataOpt() error msg = %v, wantErrMsg %v", err.Error(), tt.wantErrMsg)
				return
			}
			if got != tt.want {
				t.Errorf("packDataOpt() = %v, want %v", got, tt.want)
			}
		})
	}
}

// TestCrashNSEC tests generated using fuzz.go and with a message pack
// containing the following bytes:
// "0000\x00\x00000000\x00\x00/00000" +
// "0\x00\v\x00#\b00000\x00\x00\x00\x00\x00\x1a000" +
// "000\x00\x00\x00\x00\x1a000000\x00\x00\x00\x00\x1a0" +
// "00000\x00\v00\a0000000\x00"
// That byte sequence, when Unpack() and subsequential Pack() created a
// panic: runtime error: slice bounds out of range
// which was attributed to the fact that NSEC RR length computation was different (and smaller)
// then when within packDataNsec.
func TestCrashNSEC(t *testing.T) {
	compression := make(map[string]struct{})
	nsec := &NSEC{
		Hdr: RR_Header{
			Name:     ".",
			Rrtype:   0x2f,
			Class:    0x3030,
			Ttl:      0x30303030,
			Rdlength: 0xb,
		},
		NextDomain: ".",
		TypeBitMap: []uint16{
			0x2302, 0x2303, 0x230a, 0x230b,
			0x2312, 0x2313, 0x231a, 0x231b,
			0x2322, 0x2323,
		},
	}
	expectedLength := 19
	l := nsec.len(0, compression)
	if l != expectedLength {
		t.Fatalf("expected length of %d, got %d", expectedLength, l)
	}
}

// TestCrashNSEC3 tests generated using fuzz.go and with a message pack
// containing the following bytes:
// "0000\x00\x00000000\x00\x00200000" +
// "0\x00\v0000\x00\x00#\x0300\x00\x00\x00\x1a000" +
// "000\x00\v00\x0200\x00\x03000\x00"
// That byte sequence, when Unpack() and subsequential Pack() created a
// panic: runtime error: slice bounds out of range
// which was attributed to the fact that NSEC3 RR length computation was
// different (and smaller) then within NSEC3.pack (which relies on
// packDataNsec).
func TestCrashNSEC3(t *testing.T) {
	compression := make(map[string]struct{})
	nsec3 := &NSEC3{
		Hdr: RR_Header{
			Name:     ".",
			Rrtype:   0x32,
			Class:    0x3030,
			Ttl:      0x30303030,
			Rdlength: 0xb,
		},
		Hash:       0x30,
		Flags:      0x30,
		Iterations: 0x3030,
		SaltLength: 0x0,
		Salt:       "",
		HashLength: 0x0,
		NextDomain: ".",
		TypeBitMap: []uint16{
			0x2302, 0x2303, 0x230a, 0x230b,
		},
	}
	expectedLength := 24
	l := nsec3.len(0, compression)
	if l != expectedLength {
		t.Fatalf("expected length of %d, got %d", expectedLength, l)
	}
}
