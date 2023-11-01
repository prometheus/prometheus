package remote

import "testing"

func TestCompressions(t *testing.T) {
	data := []byte("Hello World")
	tc := []struct {
		name string
		algo CompAlgorithm
	}{
		{"Snappy", Snappy},
		{"SnappyAlt", SnappyAlt},
		{"S2", S2},
		{"ZstdFast", ZstdFast},
		{"ZstdDefault", ZstdDefault},
		{"ZstdBestComp", ZstdBestComp},
		{"GzipFast", GzipFast},
		{"GzipComp", GzipComp},
		{"Lzw", Lzw},
		{"FlateFast", FlateFast},
		{"FlateComp", FlateComp},
		{"BrotliFast", BrotliFast},
		{"BrotliComp", BrotliComp},
		{"BrotliDefault", BrotliDefault},
	}

	for _, c := range tc {
		t.Run(c.name, func(t *testing.T) {
			UseAlgorithm = c.algo
			comp := createComp()
			compressed, err := comp.Compress(data)
			if err != nil {
				t.Fatal(err)
			}
			decompressed, err := comp.Decompress(compressed)
			if err != nil {
				t.Fatal(err)
			}
			if string(decompressed) != string(data) {
				t.Fatalf("decompressed data is not equal to original data")
			}
		})
	}
}

func BenchmarkCompressions(b *testing.B) {
	data := makeUncompressedWriteRequestBenchData(b)
	bc := []struct {
		name string
		algo CompAlgorithm
	}{
		{"Snappy", Snappy},
		{"SnappyAlt", SnappyAlt},
		{"S2", S2},
		{"ZstdFast", ZstdFast},
		{"ZstdDefault", ZstdDefault},
		{"ZstdBestComp", ZstdBestComp},
		{"GzipFast", GzipFast},
		{"GzipComp", GzipComp},
		{"Lzw", Lzw},
		{"FlateFast", FlateFast},
		{"FlateComp", FlateComp},
		{"BrotliFast", BrotliFast},
		{"BrotliComp", BrotliComp},
		{"BrotliDefault", BrotliDefault},
	}
	comps := make(map[CompAlgorithm]Compression)
	for _, c := range bc {
		UseAlgorithm = c.algo
		comp := createComp()
		comps[c.algo] = comp
		// warmup
		for i := 0; i < 10; i++ {
			compressed, _ := comp.Compress(data)
			// if err != nil {
			// 	b.Fatal(err)
			// }
			_, _ = comp.Decompress(compressed)
			// if err != nil {
			// 	b.Fatal(err)
			// }
		}
	}

	for _, c := range bc {
		b.Run("compress-"+c.name, func(b *testing.B) {
			comp := comps[c.algo]
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				_, err := comp.Compress(data)
				if err != nil {
					b.Fatal(err)
				}
			}
		})
		b.Run("decompress-"+c.name, func(b *testing.B) {
			comp := comps[c.algo]
			compressed, err := comp.Compress(data)
			if err != nil {
				b.Fatal(err)
			}
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				_, err = comp.Decompress(compressed)
				if err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}
