package dns

import "testing"

func TestDedup(t *testing.T) {
	testcases := map[[3]RR][]string{
		[...]RR{
			testRR("mIek.nl. IN A 127.0.0.1"),
			testRR("mieK.nl. IN A 127.0.0.1"),
			testRR("miek.Nl. IN A 127.0.0.1"),
		}: {"mIek.nl.\t3600\tIN\tA\t127.0.0.1"},
		[...]RR{
			testRR("miEk.nl. 2000 IN A 127.0.0.1"),
			testRR("mieK.Nl. 1000 IN A 127.0.0.1"),
			testRR("Miek.nL. 500 IN A 127.0.0.1"),
		}: {"miEk.nl.\t500\tIN\tA\t127.0.0.1"},
		[...]RR{
			testRR("miek.nl. IN A 127.0.0.1"),
			testRR("miek.nl. CH A 127.0.0.1"),
			testRR("miek.nl. IN A 127.0.0.1"),
		}: {"miek.nl.\t3600\tIN\tA\t127.0.0.1",
			"miek.nl.\t3600\tCH\tA\t127.0.0.1",
		},
		[...]RR{
			testRR("miek.nl. CH A 127.0.0.1"),
			testRR("miek.nl. IN A 127.0.0.1"),
			testRR("miek.de. IN A 127.0.0.1"),
		}: {"miek.nl.\t3600\tCH\tA\t127.0.0.1",
			"miek.nl.\t3600\tIN\tA\t127.0.0.1",
			"miek.de.\t3600\tIN\tA\t127.0.0.1",
		},
		[...]RR{
			testRR("miek.de. IN A 127.0.0.1"),
			testRR("miek.nl. 200 IN A 127.0.0.1"),
			testRR("miek.nl. 300 IN A 127.0.0.1"),
		}: {"miek.de.\t3600\tIN\tA\t127.0.0.1",
			"miek.nl.\t200\tIN\tA\t127.0.0.1",
		},
	}

	for rr, expected := range testcases {
		out := Dedup([]RR{rr[0], rr[1], rr[2]}, nil)
		for i, o := range out {
			if o.String() != expected[i] {
				t.Fatalf("expected %v, got %v", expected[i], o.String())
			}
		}
	}
}

func BenchmarkDedup(b *testing.B) {
	rrs := []RR{
		testRR("miEk.nl. 2000 IN A 127.0.0.1"),
		testRR("mieK.Nl. 1000 IN A 127.0.0.1"),
		testRR("Miek.nL. 500 IN A 127.0.0.1"),
	}
	m := make(map[string]RR)
	for i := 0; i < b.N; i++ {
		Dedup(rrs, m)
	}
}

func TestNormalizedString(t *testing.T) {
	tests := map[RR]string{
		testRR("mIEk.Nl. 3600 IN A 127.0.0.1"):     "miek.nl.\tIN\tA\t127.0.0.1",
		testRR("m\\ iek.nL. 3600 IN A 127.0.0.1"):  "m\\ iek.nl.\tIN\tA\t127.0.0.1",
		testRR("m\\\tIeK.nl. 3600 in A 127.0.0.1"): "m\\009iek.nl.\tIN\tA\t127.0.0.1",
	}
	for tc, expected := range tests {
		n := normalizedString(tc)
		if n != expected {
			t.Errorf("expected %s, got %s", expected, n)
		}
	}
}
