package remote

import (
	"testing"
	"time"
)

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

func BenchmarkCompressions_V11(b *testing.B) {
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
		// {"BrotliComp", BrotliComp},
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
		for i := 0; i < 2; i++ {
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
		b.Run(c.name, func(b *testing.B) {
			comp := comps[c.algo]
			decomp := decomps[c.algo]

			totalSize := 0
			compTime := 0
			decompTime := 0
			rate := 0.0
			var start time.Time
			for i := 0; i < b.N; i++ {
				start = time.Now()
				res, err := comp.Compress(data)
				if err != nil {
					b.Fatal(err)
				}
				compTime += int(time.Since(start))
				totalSize += len(res)
				rate += float64(len(data)) / float64(len(res))

				start = time.Now()
				_, err = decomp.Decompress(res)
				if err != nil {
					b.Fatal(err)
				}
				decompTime += int(time.Since(start))
			}
			b.ReportMetric(float64(totalSize)/float64(b.N), "compressedSize/op")
			b.ReportMetric(float64(compTime)/float64(b.N), "nsCompress/op")
			b.ReportMetric(float64(decompTime)/float64(b.N), "nsDecompress/op")
			b.ReportMetric(rate/float64(b.N), "compressionX/op")
		})
	}
}

func BenchmarkCompressions_V1(b *testing.B) {
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
		{"Lzw", Lzw},
		{"FlateFast", FlateFast},
		{"FlateComp", FlateComp},
		{"BrotliFast", BrotliFast},
		// {"BrotliComp", BrotliComp},
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
		for i := 0; i < 2; i++ {
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
		b.Run(c.name, func(b *testing.B) {
			comp := comps[c.algo]
			decomp := decomps[c.algo]

			totalSize := 0
			compTime := 0
			decompTime := 0
			rate := 0.0
			var start time.Time
			for i := 0; i < b.N; i++ {
				start = time.Now()
				res, err := comp.Compress(data)
				if err != nil {
					b.Fatal(err)
				}
				compTime += int(time.Since(start))
				totalSize += len(res)
				rate += float64(len(data)) / float64(len(res))

				start = time.Now()
				_, err = decomp.Decompress(res)
				if err != nil {
					b.Fatal(err)
				}
				decompTime += int(time.Since(start))
			}
			b.ReportMetric(float64(totalSize)/float64(b.N), "compressedSize/op")
			b.ReportMetric(float64(compTime)/float64(b.N), "nsCompress/op")
			b.ReportMetric(float64(decompTime)/float64(b.N), "nsDecompress/op")
			b.ReportMetric(rate/float64(b.N), "compressionX/op")
		})
	}
}
