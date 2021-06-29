package chunkenc

import "github.com/prometheus/prometheus/pkg/histogram"

func writeHistoChunkMeta(b *bstream, schema int32, posSpans, negSpans []histogram.Span) {
	putInt64VBBucket(b, int64(schema))
	putHistoChunkMetaSpans(b, posSpans)
	putHistoChunkMetaSpans(b, negSpans)
}

func putHistoChunkMetaSpans(b *bstream, spans []histogram.Span) {
	putInt64VBBucket(b, int64(len(spans)))
	for _, s := range spans {
		putInt64VBBucket(b, int64(s.Length))
		putInt64VBBucket(b, int64(s.Offset))
	}
}

func readHistoChunkMeta(b *bstreamReader) (int32, []histogram.Span, []histogram.Span, error) {

	v, err := readInt64VBBucket(b)
	if err != nil {
		return 0, nil, nil, err
	}
	schema := int32(v)

	posSpans, err := readHistoChunkMetaSpans(b)
	if err != nil {
		return 0, nil, nil, err
	}

	negSpans, err := readHistoChunkMetaSpans(b)
	if err != nil {
		return 0, nil, nil, err
	}

	return schema, posSpans, negSpans, nil
}

func readHistoChunkMetaSpans(b *bstreamReader) ([]histogram.Span, error) {
	var spans []histogram.Span
	num, err := readInt64VBBucket(b)
	if err != nil {
		return nil, err
	}
	for i := 0; i < int(num); i++ {

		length, err := readInt64VBBucket(b)
		if err != nil {
			return nil, err
		}

		offset, err := readInt64VBBucket(b)
		if err != nil {
			return nil, err
		}

		spans = append(spans, histogram.Span{
			Length: uint32(length),
			Offset: int32(offset),
		})
	}
	return spans, nil
}
