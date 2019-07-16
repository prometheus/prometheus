package cloudmap

import "testing"

func TestAccountParse(t *testing.T) {
	cases := []struct {
		in, want string
	}{
		{"arn:aws:iam::222265345133:role/Monitoring-EU-WEST-1", "222265345133"},
	}
	for _, c := range cases {
		got := ParseAccountNumberFromArn(c.in)
		if got != c.want {
			t.Errorf("ParseAccountNumberFromArn(%q) == %q, want %q", c.in, got, c.want)
		}
	}
}
