package bytesbuffer

import (
	"bytes"
	"fmt"
	"io"
	"regexp"
)

type BytesBuffer struct {
	Buffer *bytes.Buffer
}

func Str_to_hex(str string, prefix string) string {
	dst := bytes.NewBufferString("")

	slen := len(str)
	for i := 0; i < slen; i++ {
		dst.WriteString(fmt.Sprintf("%s%2X", prefix, ([]byte(str))[i]))
	}

	return dst.String()
}

func Hex_to_str(str string) (string, error) {
	var tmp []byte = make([]byte, 2)

	re := regexp.MustCompile("[^a-fA-F0-9]")
	clean_str := re.ReplaceAllString(str, "")

	src := bytes.NewBufferString(clean_str)
	dst := bytes.NewBufferString("")

	cnt := src.Len() / 2

	for i := 0; i < cnt; i++ {
		num := 0
		_, err := src.Read(tmp)
		if err != nil {
			return "", err
		}
		fmt.Sscanf(string(tmp), "%X", &num)
		dst.WriteByte(byte(num))
	}

	return dst.String(), nil

}

func NewBuffer(buf []byte) *BytesBuffer {
	return &BytesBuffer{Buffer: bytes.NewBuffer(buf)}
}

func NewBufferString(s string) *BytesBuffer {
	return &BytesBuffer{Buffer: bytes.NewBufferString(s)}
}

// \x31\x32\x33\x34	-> 1234
// %31%32%33%34		-> 1234
// 31323334			-> 1234
func (this *BytesBuffer) WriteByteString(byteStr string) error {
	var tmp []byte = make([]byte, 2)

	re := regexp.MustCompile("[^a-fA-F0-9]")
	clean_str := re.ReplaceAllString(byteStr, "")

	src := bytes.NewBufferString(clean_str)

	cnt := src.Len() / 2

	for i := 0; i < cnt; i++ {
		num := 0
		_, err := src.Read(tmp)
		if err != nil {
			return err
		}
		fmt.Sscanf(string(tmp), "%X", &num)
		this.Buffer.WriteByte(byte(num))
	}
	return nil
}

// 1234 + prefix("%")	-> %31%32%33%34
// 1234 + prefix("\x")	-> \x31\x32\x33\x34
// 1234 + prefix("")	-> 31323334
func (this *BytesBuffer) ByteString(prefix string) string {
	slen := this.Buffer.Len()
	dst := bytes.NewBufferString("")

	for i := 0; i < slen; i++ {
		c, _ := this.Buffer.ReadByte()
		dst.WriteString(fmt.Sprintf("%s%02X", prefix, c))
	}

	return dst.String()
}

// Buffer will be empty after Write
func (this *BytesBuffer) WriteToByteString(w io.Writer, prefix string) (n int64, err error) {
	slen := this.Buffer.Len()
	dst := bytes.NewBufferString("")

	for i := 0; i < slen; i++ {
		c, _ := this.Buffer.ReadByte()
		dst.WriteString(fmt.Sprintf("%s%02X", prefix, c))
	}

	return dst.WriteTo(w)
}
