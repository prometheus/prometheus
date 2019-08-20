package acm

import (
	"bytes"
	"io"
	"io/ioutil"

	"golang.org/x/text/encoding/simplifiedchinese"
	"golang.org/x/text/transform"
)

func GbkToUtf8(s []byte) ([]byte, error) {
	reader := GbkToUtf8Reader(bytes.NewReader(s))
	return ioutil.ReadAll(reader)
}

func Utf8ToGbk(s []byte) ([]byte, error) {
	reader := Utf8ToGbkReader(bytes.NewReader(s))
	return ioutil.ReadAll(reader)
}

//return utf8
func GbkToUtf8Reader(s io.Reader) io.Reader {
	return transform.NewReader(s, simplifiedchinese.GBK.NewDecoder())
}

//return gbk
func Utf8ToGbkReader(s io.Reader) io.Reader {
	return transform.NewReader(s, simplifiedchinese.GBK.NewEncoder())
}
