package dns

import "testing"

// TestPacketDataNsec tests generated using fuzz.go and with a message pack
// containing the following bytes: 0000\x00\x00000000\x00\x002000000\x0060000\x00\x130000000000000000000"
// That bytes sequence created the overflow error and further permutations of that sequence were able to trigger
// the other code paths.
func TestPackDataNsec(t *testing.T) {
	type args struct {
		bitmap []uint16
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
				bitmap: []uint16{
					8962, 8963, 8970, 8971, 8978, 8979,
					8986, 8987, 8994, 8995, 9002, 9003,
					9010, 9011, 9018, 9019, 9026, 9027,
					9034, 9035, 9042, 9043, 9050, 9051,
					9058, 9059, 9066,
				},
				msg: []byte{
					48, 48, 48, 48, 0, 0, 0,
					1, 0, 0, 0, 0, 0, 0, 50,
					48, 48, 48, 48, 48, 48,
					0, 54, 48, 48, 48, 48,
					0, 19, 48, 48,
				},
				off: 48,
			},
			wantErr:    true,
			wantErrMsg: "dns: overflow packing nsec",
			want:       31,
		},
		{
			name: "disordered nsec bits",
			args: args{
				bitmap: []uint16{
					8962,
					0,
				},
				msg: []byte{
					48, 48, 48, 48, 0, 0, 0, 1, 0, 0, 0, 0,
					0, 0, 50, 48, 48, 48, 48, 48, 48, 0, 54, 48,
					48, 48, 48, 0, 19, 48, 48, 48, 48, 48, 48, 0,
					0, 0, 1, 0, 0, 0, 0, 0, 0, 50, 48, 48,
					48, 48, 48, 48, 0, 54, 48, 48, 48, 48, 0, 19,
					48, 48, 48, 48, 48, 48, 0, 0, 0, 1, 0, 0,
					0, 0, 0, 0, 50, 48, 48, 48, 48, 48, 48, 0,
					54, 48, 48, 48, 48, 0, 19, 48, 48, 48, 48, 48,
					48, 0, 0, 0, 1, 0, 0, 0, 0, 0, 0, 50,
					48, 48, 48, 48, 48, 48, 0, 54, 48, 48, 48, 48,
					0, 19, 48, 48, 48, 48, 48, 48, 0, 0, 0, 1,
					0, 0, 0, 0, 0, 0, 50, 48, 48, 48, 48, 48,
					48, 0, 54, 48, 48, 48, 48, 0, 19, 48, 48,
				},
				off: 0,
			},
			wantErr:    true,
			wantErrMsg: "dns: nsec bits out of order",
			want:       155,
		},
		{
			name: "simple message with only one window",
			args: args{
				bitmap: []uint16{
					0,
				},
				msg: []byte{
					48, 48, 48, 48, 0, 0,
					0, 1, 0, 0, 0, 0,
					0, 0, 50, 48, 48, 48,
					48, 48, 48, 0, 54, 48,
					48, 48, 48, 0, 19, 48, 48,
				},
				off: 0,
			},
			wantErr: false,
			want:    3,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := packDataNsec(tt.args.bitmap, tt.args.msg, tt.args.off)
			if (err != nil) != tt.wantErr {
				t.Errorf("packDataNsec() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if err != nil && tt.wantErrMsg != err.Error() {
				t.Errorf("packDataNsec() error msg = %v, wantErrMsg %v", err.Error(), tt.wantErrMsg)
				return
			}
			if got != tt.want {
				t.Errorf("packDataNsec() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestUnpackString(t *testing.T) {
	msg := []byte("\x00abcdef\x0f\\\"ghi\x04mmm\x7f")
	msg[0] = byte(len(msg) - 1)

	got, _, err := unpackString(msg, 0)
	if err != nil {
		t.Fatal(err)
	}

	if want := `abcdef\015\\\"ghi\004mmm\127`; want != got {
		t.Errorf("expected %q, got %q", want, got)
	}
}

func BenchmarkUnpackString(b *testing.B) {
	msg := []byte("\x00abcdef\x0f\\\"ghi\x04mmm")
	msg[0] = byte(len(msg) - 1)

	for n := 0; n < b.N; n++ {
		got, _, err := unpackString(msg, 0)
		if err != nil {
			b.Fatal(err)
		}

		if want := `abcdef\015\\\"ghi\004mmm`; want != got {
			b.Errorf("expected %q, got %q", want, got)
		}
	}
}
