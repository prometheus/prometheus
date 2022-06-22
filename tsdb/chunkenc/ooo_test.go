package chunkenc

import (
	"testing"

	"github.com/stretchr/testify/require"
)

const testMaxSize int = 32

// formulas chosen to make testing easy:
func valPre(pos int) int { return pos*2 + 2 } // s[0]=2, s[1]=4, s[2]=6, ..., s[31]=64 // predictable pre-existing values
func valNew(pos int) int { return pos*2 + 1 } // s[0]=1, s[1]=3, s[2]=5, ..., s[31]=63 // new values will interject at chosen position because they sort before the pre-existing vals

func samplify(v int) sample { return sample{int64(v), float64(v)} }

func makePre(n int) []sample {
	s := make([]sample, n)
	for i := 0; i < n; i++ {
		s[i] = samplify(valPre(i))
	}
	return s
}

// TestOOOInsert tests the following cases:
// number of pre-existing samples anywhere from 0 to testMaxSize-1
// insert new sample before first pre-existing samples, after the last, and anywhere in between
// with a chunk initial capacity of testMaxSize/8 and testMaxSize, which lets us test non-full and full chunks, and chunks that need to expand themselves.
// Note: in all samples used, t always equals v in numeric value. when we talk about 'value' we just refer to a value that will be used for both sample.t and sample.v
func TestOOOInsert(t *testing.T) {
	for numPre := 0; numPre <= testMaxSize; numPre++ {
		// for example, if we have numPre 2, then:
		// chunk.samples indexes filled        0   1
		// chunk.samples with these values     2   4     // valPre
		// we want to test inserting at index  0   1   2 // insertPos=0..numPre
		// we can do this by using values      1,  3   5 // valNew(insertPos)

		for insertPos := 0; insertPos <= numPre; insertPos++ {
			for capacity := range []int{testMaxSize / 8, testMaxSize} {
				chunk := NewOOOChunk(capacity)
				chunk.samples = makePre(numPre)
				newSample := samplify(valNew(insertPos))
				chunk.Insert(newSample.t, newSample.v)

				var expSamples []sample
				// our expected new samples slice, will be first the original samples...
				for i := 0; i < insertPos; i++ {
					expSamples = append(expSamples, samplify(valPre(i)))
				}
				// ... then the new sample ...
				expSamples = append(expSamples, newSample)
				// ... followed by any original samples that were pushed back by the new one
				for i := insertPos; i < numPre; i++ {
					expSamples = append(expSamples, samplify(valPre(i)))
				}

				require.Equal(t, expSamples, chunk.samples, "numPre %d, insertPos %d", numPre, insertPos)
			}
		}
	}
}

// TestOOOInsertDuplicate tests the correct behavior when inserting a sample that is a duplicate of any
// pre-existing samples, with between 1 and testMaxSize pre-existing samples and
// with a chunk initial capacity of testMaxSize/8 and testMaxSize, which lets us test non-full and full chunks, and chunks that need to expand themselves.
func TestOOOInsertDuplicate(t *testing.T) {
	for numPre := 1; numPre <= testMaxSize; numPre++ {
		for dupPos := 0; dupPos < numPre; dupPos++ {
			for capacity := range []int{testMaxSize / 8, testMaxSize} {
				chunk := NewOOOChunk(capacity)
				chunk.samples = makePre(numPre)

				dupSample := chunk.samples[dupPos]
				dupSample.v = 0.123 // unmistakeably different from any of the pre-existing values, so we can properly detect the correct value below

				ok := chunk.Insert(dupSample.t, dupSample.v)

				expSamples := makePre(numPre) // we expect no change
				require.False(t, ok)
				require.Equal(t, expSamples, chunk.samples, "numPre %d, dupPos %d", numPre, dupPos)
			}
		}
	}
}
