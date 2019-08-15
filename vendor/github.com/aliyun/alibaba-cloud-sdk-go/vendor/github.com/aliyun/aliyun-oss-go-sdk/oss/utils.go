package oss

import (
	"bytes"
	"errors"
	"fmt"
	"hash/crc64"
	"net/http"
	"os"
	"os/exec"
	"runtime"
	"strconv"
	"strings"
	"time"
)

// userAgent gets user agent
// It has the SDK version information, OS information and GO version
func userAgent() string {
	sys := getSysInfo()
	return fmt.Sprintf("aliyun-sdk-go/%s (%s/%s/%s;%s)", Version, sys.name,
		sys.release, sys.machine, runtime.Version())
}

type sysInfo struct {
	name    string // OS name such as windows/Linux
	release string // OS version 2.6.32-220.23.2.ali1089.el5.x86_64 etc
	machine string // CPU type amd64/x86_64
}

// getSysInfo gets system info
// gets the OS information and CPU type
func getSysInfo() sysInfo {
	name := runtime.GOOS
	release := "-"
	machine := runtime.GOARCH
	if out, err := exec.Command("uname", "-s").CombinedOutput(); err == nil {
		name = string(bytes.TrimSpace(out))
	}
	if out, err := exec.Command("uname", "-r").CombinedOutput(); err == nil {
		release = string(bytes.TrimSpace(out))
	}
	if out, err := exec.Command("uname", "-m").CombinedOutput(); err == nil {
		machine = string(bytes.TrimSpace(out))
	}
	return sysInfo{name: name, release: release, machine: machine}
}

// unpackedRange
type unpackedRange struct {
	hasStart bool  // Flag indicates if the start point is specified
	hasEnd   bool  // Flag indicates if the end point is specified
	start    int64 // Start point
	end      int64 // End point
}

// invalidRangeError returns invalid range error
func invalidRangeError(r string) error {
	return fmt.Errorf("InvalidRange %s", r)
}

// parseRange parse various styles of range such as bytes=M-N
func parseRange(normalizedRange string) (*unpackedRange, error) {
	var err error
	hasStart := false
	hasEnd := false
	var start int64
	var end int64

	// Bytes==M-N or ranges=M-N
	nrSlice := strings.Split(normalizedRange, "=")
	if len(nrSlice) != 2 || nrSlice[0] != "bytes" {
		return nil, invalidRangeError(normalizedRange)
	}

	// Bytes=M-N,X-Y
	rSlice := strings.Split(nrSlice[1], ",")
	rStr := rSlice[0]

	if strings.HasSuffix(rStr, "-") { // M-
		startStr := rStr[:len(rStr)-1]
		start, err = strconv.ParseInt(startStr, 10, 64)
		if err != nil {
			return nil, invalidRangeError(normalizedRange)
		}
		hasStart = true
	} else if strings.HasPrefix(rStr, "-") { // -N
		len := rStr[1:]
		end, err = strconv.ParseInt(len, 10, 64)
		if err != nil {
			return nil, invalidRangeError(normalizedRange)
		}
		if end == 0 { // -0
			return nil, invalidRangeError(normalizedRange)
		}
		hasEnd = true
	} else { // M-N
		valSlice := strings.Split(rStr, "-")
		if len(valSlice) != 2 {
			return nil, invalidRangeError(normalizedRange)
		}
		start, err = strconv.ParseInt(valSlice[0], 10, 64)
		if err != nil {
			return nil, invalidRangeError(normalizedRange)
		}
		hasStart = true
		end, err = strconv.ParseInt(valSlice[1], 10, 64)
		if err != nil {
			return nil, invalidRangeError(normalizedRange)
		}
		hasEnd = true
	}

	return &unpackedRange{hasStart, hasEnd, start, end}, nil
}

// adjustRange returns adjusted range, adjust the range according to the length of the file
func adjustRange(ur *unpackedRange, size int64) (start, end int64) {
	if ur == nil {
		return 0, size
	}

	if ur.hasStart && ur.hasEnd {
		start = ur.start
		end = ur.end + 1
		if ur.start < 0 || ur.start >= size || ur.end > size || ur.start > ur.end {
			start = 0
			end = size
		}
	} else if ur.hasStart {
		start = ur.start
		end = size
		if ur.start < 0 || ur.start >= size {
			start = 0
		}
	} else if ur.hasEnd {
		start = size - ur.end
		end = size
		if ur.end < 0 || ur.end > size {
			start = 0
			end = size
		}
	}
	return
}

// GetNowSec returns Unix time, the number of seconds elapsed since January 1, 1970 UTC.
// gets the current time in Unix time, in seconds.
func GetNowSec() int64 {
	return time.Now().Unix()
}

// GetNowNanoSec returns t as a Unix time, the number of nanoseconds elapsed
// since January 1, 1970 UTC. The result is undefined if the Unix time
// in nanoseconds cannot be represented by an int64. Note that this
// means the result of calling UnixNano on the zero Time is undefined.
// gets the current time in Unix time, in nanoseconds.
func GetNowNanoSec() int64 {
	return time.Now().UnixNano()
}

// GetNowGMT gets the current time in GMT format.
func GetNowGMT() string {
	return time.Now().UTC().Format(http.TimeFormat)
}

// FileChunk is the file chunk definition
type FileChunk struct {
	Number int   // Chunk number
	Offset int64 // Chunk offset
	Size   int64 // Chunk size.
}

// SplitFileByPartNum splits big file into parts by the num of parts.
// Split the file with specified parts count, returns the split result when error is nil.
func SplitFileByPartNum(fileName string, chunkNum int) ([]FileChunk, error) {
	if chunkNum <= 0 || chunkNum > 10000 {
		return nil, errors.New("chunkNum invalid")
	}

	file, err := os.Open(fileName)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	stat, err := file.Stat()
	if err != nil {
		return nil, err
	}

	if int64(chunkNum) > stat.Size() {
		return nil, errors.New("oss: chunkNum invalid")
	}

	var chunks []FileChunk
	var chunk = FileChunk{}
	var chunkN = (int64)(chunkNum)
	for i := int64(0); i < chunkN; i++ {
		chunk.Number = int(i + 1)
		chunk.Offset = i * (stat.Size() / chunkN)
		if i == chunkN-1 {
			chunk.Size = stat.Size()/chunkN + stat.Size()%chunkN
		} else {
			chunk.Size = stat.Size() / chunkN
		}
		chunks = append(chunks, chunk)
	}

	return chunks, nil
}

// SplitFileByPartSize splits big file into parts by the size of parts.
// Splits the file by the part size. Returns the FileChunk when error is nil.
func SplitFileByPartSize(fileName string, chunkSize int64) ([]FileChunk, error) {
	if chunkSize <= 0 {
		return nil, errors.New("chunkSize invalid")
	}

	file, err := os.Open(fileName)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	stat, err := file.Stat()
	if err != nil {
		return nil, err
	}
	var chunkN = stat.Size() / chunkSize
	if chunkN >= 10000 {
		return nil, errors.New("Too many parts, please increase part size")
	}

	var chunks []FileChunk
	var chunk = FileChunk{}
	for i := int64(0); i < chunkN; i++ {
		chunk.Number = int(i + 1)
		chunk.Offset = i * chunkSize
		chunk.Size = chunkSize
		chunks = append(chunks, chunk)
	}

	if stat.Size()%chunkSize > 0 {
		chunk.Number = len(chunks) + 1
		chunk.Offset = int64(len(chunks)) * chunkSize
		chunk.Size = stat.Size() % chunkSize
		chunks = append(chunks, chunk)
	}

	return chunks, nil
}

// GetPartEnd calculates the end position
func GetPartEnd(begin int64, total int64, per int64) int64 {
	if begin+per > total {
		return total - 1
	}
	return begin + per - 1
}

// crcTable returns the table constructed from the specified polynomial
var crcTable = func() *crc64.Table {
	return crc64.MakeTable(crc64.ECMA)
}
