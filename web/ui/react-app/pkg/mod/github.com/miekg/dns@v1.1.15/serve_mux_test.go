package dns

import "testing"

func TestDotAsCatchAllWildcard(t *testing.T) {
	mux := NewServeMux()
	mux.Handle(".", HandlerFunc(HelloServer))
	mux.Handle("example.com.", HandlerFunc(AnotherHelloServer))

	handler := mux.match("www.miek.nl.", TypeTXT)
	if handler == nil {
		t.Error("wildcard match failed")
	}

	handler = mux.match("www.example.com.", TypeTXT)
	if handler == nil {
		t.Error("example.com match failed")
	}

	handler = mux.match("a.www.example.com.", TypeTXT)
	if handler == nil {
		t.Error("a.www.example.com match failed")
	}

	handler = mux.match("boe.", TypeTXT)
	if handler == nil {
		t.Error("boe. match failed")
	}
}

func TestCaseFolding(t *testing.T) {
	mux := NewServeMux()
	mux.Handle("_udp.example.com.", HandlerFunc(HelloServer))

	handler := mux.match("_dns._udp.example.com.", TypeSRV)
	if handler == nil {
		t.Error("case sensitive characters folded")
	}

	handler = mux.match("_DNS._UDP.EXAMPLE.COM.", TypeSRV)
	if handler == nil {
		t.Error("case insensitive characters not folded")
	}
}

func TestRootServer(t *testing.T) {
	mux := NewServeMux()
	mux.Handle(".", HandlerFunc(HelloServer))

	handler := mux.match(".", TypeNS)
	if handler == nil {
		t.Error("root match failed")
	}
}

func BenchmarkMuxMatch(b *testing.B) {
	mux := NewServeMux()
	mux.Handle("_udp.example.com.", HandlerFunc(HelloServer))

	bench := func(q string) func(*testing.B) {
		return func(b *testing.B) {
			for n := 0; n < b.N; n++ {
				handler := mux.match(q, TypeSRV)
				if handler == nil {
					b.Fatal("couldn't find match")
				}
			}
		}
	}
	b.Run("lowercase", bench("_dns._udp.example.com."))
	b.Run("uppercase", bench("_DNS._UDP.EXAMPLE.COM."))
}
