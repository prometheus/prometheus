package acm

import (
	"testing"
)

var s = "GBK与UTF-8 编码转换测试"

func TestGbkToUtf8(t *testing.T) {
	gbk, err := Utf8ToGbk([]byte(s))
	if err != nil {
		t.Error(err)
	}

	utf8, err := GbkToUtf8([]byte(gbk))
	if err != nil {
		t.Error(err)
	}

	if string(utf8) != s {
		t.Error("Iconv Fail:", utf8, s)
	}
}

func TestUtf8ToGbk(t *testing.T) {
	_, err := Utf8ToGbk([]byte(s))
	if err != nil {
		t.Error(err)
	}

	//fmt.Printf("%s", string(gbk))
}
