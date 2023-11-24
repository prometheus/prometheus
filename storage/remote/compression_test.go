package remote

import (
	"github.com/stretchr/testify/require"
	"os"
	"testing"
	"time"
)

func TestCompressions(t *testing.T) {
	data, err := makeUncompressedWriteRequestBenchData()
	require.NoError(t, err)
	tc := []struct {
		name string
		algo RemoteWriteCompression
	}{
		{"Snappy", Snappy},
		{"Zstd", Zstd},
		{"Flate", Flate},
		//{"SnappyAlt", SnappyAlt},
		//{"S2", S2},
		//{"ZstdFast", ZstdFast},
		//{"ZstdDefault", ZstdDefault},
		//{"ZstdBestComp", ZstdBestComp},
		//{"Lzw", Lzw},
		//{"FlateFast", FlateFast},
		//{"FlateComp", FlateComp},
		//{"BrotliFast", BrotliFast},
		//{"BrotliComp", BrotliComp},
		//{"BrotliDefault", BrotliDefault},
	}

	for _, c := range tc {
		t.Run(c.name, func(t *testing.T) {
			//UseAlgorithm = c.algo
			//comp := createComp()
			comp := NewCompressor(c.algo)
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

func BenchmarkCompressions_V1(b *testing.B) {
	// Synthetic data, attempts to be representative
	data, err := makeUncompressedWriteRequestBenchData()
	require.NoError(b, err)
	benchmarkCompressionsForData(b, [][]byte{data})
}

//func BenchmarkCompressions_V11(b *testing.B) {
//	// Synthetic data, attempts to be representative
//	data, err := makeUncompressedWriteRequestBenchData()
//	require.NoError(b, err)
//	benchmarkCompressionsForData(b, [][]byte{data})
//}

// Needs the dataset to be present in /home/nicolas/rw11data/v11_raw/
//func BenchmarkCompressions_V11_FileDataSet(b *testing.B) {
//	datas := readAllFiles("/home/nicolas/rw11data/v11_raw/")
//	if len(datas) != 10 {
//		b.Fatal("unexpected number of files")
//	}
//	benchmarkCompressionsForData(b, datas)
//}

// Needs the dataset to be present in /home/nicolas/rw11data/v1_raw/
//func BenchmarkCompressions_V1_FileDataSet(b *testing.B) {
//	datas := readAllFiles("/home/nicolas/rw11data/v1_raw/")
//	if len(datas) != 10 {
//		b.Fatal("unexpected number of files")
//	}
//	benchmarkCompressionsForData(b, datas)
//}

func benchmarkCompressionsForData(b *testing.B, datas [][]byte) {
	bc := []struct {
		name string
		algo RemoteWriteCompression
	}{
		{"Snappy", Snappy},
		{"Zstd", Zstd},
		{"Flate", Flate},
		//{"SnappyAlt", SnappyAlt},
		//{"S2", S2},
		//{"ZstdFast", ZstdFast},
		//{"ZstdDefault", ZstdDefault},
		//{"ZstdBestComp", ZstdBestComp},
		//{"Lzw", Lzw},
		//{"FlateFast", FlateFast},
		//{"FlateComp", FlateComp},
		//{"BrotliFast", BrotliFast},
		//{"BrotliComp", BrotliComp},
		//{"BrotliDefault", BrotliDefault},
	}
	comps := make(map[RemoteWriteCompression]*Compressor)
	decomps := make(map[RemoteWriteCompression]*Compressor)
	for _, c := range bc {
		comp := NewCompressor(c.algo)
		decomp := NewCompressor(c.algo)

		comps[c.algo] = &comp
		decomps[c.algo] = &decomp
		// warmup
		for i := 0; i < 2; i++ {
			for _, data := range datas {
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
	}

	for _, c := range bc {
		b.Run(c.name, func(b *testing.B) {
			comp := comps[c.algo]
			decomp := decomps[c.algo]

			totalSize := 0
			totalRawSize := 0
			compTime := 0
			decompTime := 0
			var start time.Time
			for i := 0; i < b.N; i++ {
				for _, data := range datas {
					start = time.Now()
					res, err := comp.Compress(data)
					if err != nil {
						b.Fatal(err)
					}
					compTime += int(time.Since(start))
					totalSize += len(res)
					totalRawSize += len(data)
					start = time.Now()
					_, err = decomp.Decompress(res)
					if err != nil {
						b.Fatal(err)
					}
					decompTime += int(time.Since(start))
				}
			}
			b.ReportMetric(float64(totalSize)/float64(b.N), "compressedSize/op")
			b.ReportMetric(float64(compTime)/float64(b.N), "nsCompress/op")
			b.ReportMetric(float64(decompTime)/float64(b.N), "nsDecompress/op")
			rate := float64(totalRawSize) / float64(totalSize)
			b.ReportMetric(rate, "compressionX")

		})
	}
}

func readAllFiles(dir string) (res [][]byte) {
	files, err := os.ReadDir(dir)
	if err != nil {
		panic(err)
	}
	for _, file := range files {
		filename := dir + file.Name()
		data, err := os.ReadFile(filename)
		if err != nil {
			panic(err)
		}
		res = append(res, data)
	}
	return
}
