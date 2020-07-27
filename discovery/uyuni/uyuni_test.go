package uyuni

import "testing"

func TestExtractPortFromFormulaData(t *testing.T) {
	type args struct {
		args    string
		address string
	}
	tests := []struct {
		name    string
		args    args
		want    string
		wantErr bool
	}{
		{name: `TestArgs`, args: args{args: `--web.listen-address=":9100"`}, want: `9100`},
		{name: `TestAddress`, args: args{address: `:9100`}, want: `9100`},
		{name: `TestArgsAndAddress`, args: args{args: `--web.listen-address=":9100"`, address: `9999`}, want: `9100`},
		{name: `TestMissingPort`, args: args{args: `localhost`}, wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := extractPortFromFormulaData(tt.args.args, tt.args.address)
			if (err != nil) != tt.wantErr {
				t.Errorf("extractPortFromFormulaData() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if got != tt.want {
				t.Errorf("extractPortFromFormulaData() got = %v, want %v", got, tt.want)
			}
		})
	}
}
