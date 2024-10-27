// Copyright 2024 The Prometheus Authors
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

//go:build linux

package fileutil

import (
	"errors"
	"fmt"
	"io"
	"os"
	"unsafe"

	"golang.org/x/sys/unix"
)

const (
	requiredAlignment = 4096
	blockSizeUnit     = requiredAlignment
	defaultBufSize    = blockSizeUnit
)

var errWriterInvalid = errors.New("the last flush resulted in an unaligned offset, the writer can no longer ensure contiguous writes")

// directIOWriter is a specialized bufio.Writer that supports Direct IO to a file
// by ensuring all alignment restrictions are satisfied.
// The writer can handle files whose initial offsets are not aligned.
// Once Direct IO is in use, if an explicit call to Flush() results in an unaligned offset, the writer
// should no longer be used, as it can no longer support continuous writes.
type directIOWriter struct {
	buf []byte
	n   int

	f *os.File
	// offsetAlignmentGap represents the number of bytes needed to reach the nearest
	// offset alignment, making Direct IO possible.
	offsetAlignmentGap int

	err     error
	invalid bool
}

func newDirectIOWriter(f *os.File, size int) (*directIOWriter, error) {
	if size <= 0 {
		size = defaultBufSize
	}
	if size%blockSizeUnit != 0 {
		return nil, fmt.Errorf("size %d should be a multiple of %d", size, blockSizeUnit)
	}
	gap, err := checkInitialUnalignedOffset(f)
	if err != nil {
		return nil, err
	}

	return &directIOWriter{
		buf:                alignedBlock(size, requiredAlignment),
		f:                  f,
		offsetAlignmentGap: gap,
	}, nil
}

func (b *directIOWriter) Available() int { return len(b.buf) - b.n }

func (b *directIOWriter) Buffered() int { return b.n }

// fillInitialOffsetGap writes the necessary bytes from the buffer without Direct IO
// to fill offsetAlignmentGap and align the file offset, enabling Direct IO usage.
// Once alignment is achieved, Direct IO is enabled.
func (b *directIOWriter) fillInitialOffsetGap() {
	if b.n == 0 || b.offsetAlignmentGap == 0 {
		return
	}

	bytesToAlign := min(b.n, b.offsetAlignmentGap)
	n, err := b.f.Write(b.buf[:bytesToAlign])
	if n < bytesToAlign && err == nil {
		err = io.ErrShortWrite
	}
	if n > 0 {
		copy(b.buf[0:b.n-n], b.buf[n:b.n])
		b.n -= n
	}
	// If the file offset was aligned, enable Direct IO.
	b.offsetAlignmentGap -= n
	if b.offsetAlignmentGap == 0 {
		err = errors.Join(err, enableDirectIO(b.f.Fd()))
	}
	b.err = err
}

func (b *directIOWriter) directIOWrite(p []byte, padding int) (int, error) {
	relevant := len(p) - padding

	n, err := b.f.Write(p)
	switch {
	case n < relevant:
		relevant = n
		if err == nil {
			err = io.ErrShortWrite
		}
	case n > relevant:
		// Adjust the offset to discard the padding that was written.
		writtenPadding := int64(n - relevant)
		_, err := b.f.Seek(-writtenPadding, io.SeekCurrent)
		if err != nil {
			b.err = errors.Join(b.err, fmt.Errorf("seek to discard written padding %d: %w", writtenPadding, err))
		}
	}

	if relevant%blockSizeUnit != 0 {
		b.invalid = true
	}
	return relevant, err
}

// canDirectIOWrite returns true when all Direct IO alignment restrictions
// are met for the p block to be written into the file.
func (b *directIOWriter) canDirectIOWrite(p []byte) bool {
	return isAligned(p) && b.offsetAlignmentGap == 0
}

func (b *directIOWriter) Write(p []byte) (nn int, err error) {
	if b.invalid {
		return 0, errWriterInvalid
	}

	for len(p) > b.Available() && b.err == nil {
		var n1, n2 int
		if b.Buffered() == 0 && b.canDirectIOWrite(p) {
			// Large write, empty buffer.
			// Write from p via Direct IO as the block and the file offset are aligned,
			// to avoid copy.
			n1, b.err = b.directIOWrite(p, 0)
		} else {
			n1 = copy(b.buf[b.n:], p)
			b.n += n1
			if b.offsetAlignmentGap != 0 {
				b.fillInitialOffsetGap()
				// Refill the buffer.
				n2 = copy(b.buf[b.n:], p[n1:])
				b.n += n2
			}
			if b.Available() == 0 {
				// Avoid flushing in case the second refill wasn't complete.
				b.flush()
			}
		}
		nn += n1 + n2
		p = p[n1+n2:]
	}

	if b.err != nil {
		return nn, b.err
	}

	n := copy(b.buf[b.n:], p)
	b.n += n
	nn += n
	return nn, nil
}

func (b *directIOWriter) flush() error {
	if b.invalid {
		return errWriterInvalid
	}
	if b.err != nil {
		return b.err
	}
	if b.n == 0 {
		return nil
	}

	// Ensure the block alignment restriction is met.
	// If the buffer length isn't a multiple of blockSizeUnit, round
	// it to the nearest upper multiple and add zero padding.
	uOffset := b.n
	if uOffset%blockSizeUnit != 0 {
		uOffset = ((uOffset / blockSizeUnit) + 1) * blockSizeUnit
		for i := b.n; i < uOffset; i++ {
			b.buf[i] = 0
		}
	}
	n, err := b.directIOWrite(b.buf[:uOffset], uOffset-b.n)
	if err != nil {
		if n > 0 && n < b.n {
			copy(b.buf[0:b.n-n], b.buf[n:b.n])
		}
		b.n -= n
		b.err = errors.Join(b.err, err)
		return err
	}

	b.n = 0
	return nil
}

func (b *directIOWriter) Flush() error {
	if b.offsetAlignmentGap != 0 {
		b.fillInitialOffsetGap()
		if b.err != nil {
			return b.err
		}
	}
	return b.flush()
}

func (b *directIOWriter) Reset(f *os.File) error {
	if b.buf == nil {
		b.buf = alignedBlock(defaultBufSize, requiredAlignment)
	}
	gap, err := checkInitialUnalignedOffset(f)
	if err != nil {
		return err
	}
	b.err = nil
	b.invalid = false
	b.n = 0
	b.f = f
	b.offsetAlignmentGap = gap
	return nil
}

func alignmentOffset(block []byte) int {
	return computeAlignmentOffset(block, requiredAlignment)
}

func computeAlignmentOffset(block []byte, alignment int) int {
	if alignment == 0 {
		return 0
	}
	if len(block) == 0 {
		panic("empty block not supported")
	}
	return int(uintptr(unsafe.Pointer(&block[0])) & uintptr(alignment-1))
}

// isAligned checks if the length of the block is a multiple of blockSizeUnit
// and if its address is aligned with requiredAlignment.
func isAligned(block []byte) bool {
	return alignmentOffset(block) == 0 && len(block)%blockSizeUnit == 0
}

// alignedBlock returns a block whose address is alignment aligned.
// The size should be a multiple of blockSizeUnit.
func alignedBlock(size, alignment int) []byte {
	if size == 0 || size%blockSizeUnit != 0 {
		panic(fmt.Errorf("size %d should be > 0 and a multiple of blockSizeUnit=%d", size, blockSizeUnit))
	}
	if alignment == 0 {
		return make([]byte, size)
	}

	block := make([]byte, size+alignment)
	a := alignmentOffset(block)
	if a == 0 {
		return block[:size]
	}

	offset := alignment - a
	block = block[offset : offset+size]
	if !isAligned(block) {
		// Assuming this to be rare, if not impossible.
		panic("cannot create an aligned block")
	}
	return block
}

func currentFileOffset(f *os.File) (int, error) {
	curOff, err := f.Seek(0, io.SeekCurrent)
	if err != nil {
		return 0, fmt.Errorf("cannot get the current offset: %w", err)
	}
	return int(curOff), nil
}

func fileStatusFlags(fd uintptr) (int, error) {
	flag, err := unix.FcntlInt(fd, unix.F_GETFL, 0)
	if err != nil {
		return 0, fmt.Errorf("cannot get file status flags: %w", err)
	}
	return flag, err
}

// enableDirectIO enabled Direct IO on the file if needed.
func enableDirectIO(fd uintptr) error {
	flag, err := fileStatusFlags(fd)
	if err != nil {
		return err
	}

	if (flag & unix.O_DIRECT) == unix.O_DIRECT {
		return nil
	}

	_, err = unix.FcntlInt(fd, unix.F_SETFL, flag|unix.O_DIRECT)
	if err != nil {
		return fmt.Errorf("cannot enable Direct IO: %w", err)
	}
	return nil
}

// checkInitialUnalignedOffset returns the gap between the current offset of the file
// and the nearest aligned offset.
// If the current offset is aligned, Direct IO is enabled on the file.
func checkInitialUnalignedOffset(f *os.File) (int, error) {
	offset, err := currentFileOffset(f)
	if err != nil {
		return 0, err
	}
	gap := (requiredAlignment - offset%requiredAlignment) % requiredAlignment
	if gap == 0 {
		if err := enableDirectIO(f.Fd()); err != nil {
			return 0, err
		}
	}
	return gap, nil
}
