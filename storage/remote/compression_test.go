package remote

import "testing"

func TestCompressions(t *testing.T) {
	data := makeUncompressedReducedWriteRequestBenchData(t)
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
			compressedCopy := make([]byte, len(compressed))
			copy(compressedCopy, compressed)
			decompressed, err := comp.Decompress(compressedCopy)
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
	data := makeUncompressedReducedWriteRequestBenchData(b)
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
		{"Lzw", Lzw},
		{"FlateFast", FlateFast},
		{"FlateComp", FlateComp},
		{"BrotliFast", BrotliFast},
		{"BrotliComp", BrotliComp},
		{"BrotliDefault", BrotliDefault},
	}
	comps := make(map[CompAlgorithm]Compression)
	decomps := make(map[CompAlgorithm]Compression)
	for _, c := range bc {
		UseAlgorithm = c.algo
		comp := createComp()
		decomp := createComp()
		comps[c.algo] = comp
		decomps[c.algo] = decomp
		// warmup
		for i := 0; i < 10; i++ {
			compressed, err := comp.Compress(data)
			if err != nil {
				b.Fatal(err)
			}
			_, err = decomp.Decompress(compressed)
			if err != nil {
				b.Fatal(err)
			}
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
			decomp := decomps[c.algo]
			compressed, err := comp.Compress(data)
			if err != nil {
				b.Fatal(err)
			}
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				_, err = decomp.Decompress(compressed)
				if err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}
