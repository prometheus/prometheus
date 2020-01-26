package dns

import (
	"testing"
)

func TestCmToM(t *testing.T) {
	s := cmToM(0, 0)
	if s != "0.00" {
		t.Error("0, 0")
	}

	s = cmToM(1, 0)
	if s != "0.01" {
		t.Error("1, 0")
	}

	s = cmToM(3, 1)
	if s != "0.30" {
		t.Error("3, 1")
	}

	s = cmToM(4, 2)
	if s != "4" {
		t.Error("4, 2")
	}

	s = cmToM(5, 3)
	if s != "50" {
		t.Error("5, 3")
	}

	s = cmToM(7, 5)
	if s != "7000" {
		t.Error("7, 5")
	}

	s = cmToM(9, 9)
	if s != "90000000" {
		t.Error("9, 9")
	}
}

func TestSplitN(t *testing.T) {
	xs := splitN("abc", 5)
	if len(xs) != 1 && xs[0] != "abc" {
		t.Errorf("failure to split abc")
	}

	s := ""
	for i := 0; i < 255; i++ {
		s += "a"
	}

	xs = splitN(s, 255)
	if len(xs) != 1 && xs[0] != s {
		t.Errorf("failure to split 255 char long string")
	}

	s += "b"
	xs = splitN(s, 255)
	if len(xs) != 2 || xs[1] != "b" {
		t.Errorf("failure to split 256 char long string: %d", len(xs))
	}

	// Make s longer
	for i := 0; i < 255; i++ {
		s += "a"
	}
	xs = splitN(s, 255)
	if len(xs) != 3 || xs[2] != "a" {
		t.Errorf("failure to split 510 char long string: %d", len(xs))
	}
}

func TestSprintName(t *testing.T) {
	got := sprintName("abc\\.def\007\"\127@\255\x05\xef\\")

	if want := "abc\\.def\\007\\\"W\\@\\173\\005\\239"; got != want {
		t.Errorf("expected %q, got %q", got, want)
	}
}

func TestSprintTxtOctet(t *testing.T) {
	got := sprintTxtOctet("abc\\.def\007\"\127@\255\x05\xef\\")

	if want := "\"abc\\.def\\007\"W@\\173\\005\\239\""; got != want {
		t.Errorf("expected %q, got %q", got, want)
	}
}

func TestSprintTxt(t *testing.T) {
	got := sprintTxt([]string{
		"abc\\.def\007\"\127@\255\x05\xef\\",
		"example.com",
	})

	if want := "\"abc.def\\007\\\"W@\\173\\005\\239\" \"example.com\""; got != want {
		t.Errorf("expected %q, got %q", got, want)
	}
}

func TestRPStringer(t *testing.T) {
	rp := &RP{
		Hdr: RR_Header{
			Name:   "test.example.com.",
			Rrtype: TypeRP,
			Class:  ClassINET,
			Ttl:    600,
		},
		Mbox: "\x05first.example.com.",
		Txt:  "second.\x07example.com.",
	}

	const expected = "test.example.com.\t600\tIN\tRP\t\\005first.example.com. second.\\007example.com."
	if rp.String() != expected {
		t.Errorf("expected %v, got %v", expected, rp)
	}

	_, err := NewRR(rp.String())
	if err != nil {
		t.Fatalf("error parsing %q: %v", rp, err)
	}
}

func BenchmarkSprintName(b *testing.B) {
	for n := 0; n < b.N; n++ {
		got := sprintName("abc\\.def\007\"\127@\255\x05\xef\\")

		if want := "abc\\.def\\007\\\"W\\@\\173\\005\\239"; got != want {
			b.Fatalf("expected %q, got %q", got, want)
		}
	}
}

func BenchmarkSprintTxtOctet(b *testing.B) {
	for n := 0; n < b.N; n++ {
		got := sprintTxtOctet("abc\\.def\007\"\127@\255\x05\xef\\")

		if want := "\"abc\\.def\\007\"W@\\173\\005\\239\""; got != want {
			b.Fatalf("expected %q, got %q", got, want)
		}
	}
}

func BenchmarkSprintTxt(b *testing.B) {
	txt := []string{
		"abc\\.def\007\"\127@\255\x05\xef\\",
		"example.com",
	}

	for n := 0; n < b.N; n++ {
		got := sprintTxt(txt)

		if want := "\"abc.def\\007\\\"W@\\173\\005\\239\" \"example.com\""; got != want {
			b.Fatalf("expected %q, got %q", got, want)
		}
	}
}
