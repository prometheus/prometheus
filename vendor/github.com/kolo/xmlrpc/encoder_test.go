package xmlrpc

import (
	"testing"
	"time"
)

var marshalTests = []struct {
	value interface{}
	xml   string
}{
	{100, "<value><int>100</int></value>"},
	{"Once upon a time", "<value><string>Once upon a time</string></value>"},
	{"Mike & Mick <London, UK>", "<value><string>Mike &amp; Mick &lt;London, UK&gt;</string></value>"},
	{Base64("T25jZSB1cG9uIGEgdGltZQ=="), "<value><base64>T25jZSB1cG9uIGEgdGltZQ==</base64></value>"},
	{true, "<value><boolean>1</boolean></value>"},
	{false, "<value><boolean>0</boolean></value>"},
	{12.134, "<value><double>12.134</double></value>"},
	{-12.134, "<value><double>-12.134</double></value>"},
	{738777323.0, "<value><double>738777323</double></value>"},
	{time.Unix(1386622812, 0).UTC(), "<value><dateTime.iso8601>20131209T21:00:12</dateTime.iso8601></value>"},
	{[]interface{}{1, "one"}, "<value><array><data><value><int>1</int></value><value><string>one</string></value></data></array></value>"},
	{&struct {
		Title  string
		Amount int
	}{"War and Piece", 20}, "<value><struct><member><name>Title</name><value><string>War and Piece</string></value></member><member><name>Amount</name><value><int>20</int></value></member></struct></value>"},
	{&struct {
		Value interface{} `xmlrpc:"value"`
	}{}, "<value><struct><member><name>value</name><value/></member></struct></value>"},
	{
		map[string]interface{}{"title": "War and Piece", "amount": 20},
		"<value><struct><member><name>amount</name><value><int>20</int></value></member><member><name>title</name><value><string>War and Piece</string></value></member></struct></value>",
	},
	{
		map[string]interface{}{
			"Name":  "John Smith",
			"Age":   6,
			"Wight": []float32{66.67, 100.5},
			"Dates": map[string]interface{}{"Birth": time.Date(1829, time.November, 10, 23, 0, 0, 0, time.UTC), "Death": time.Date(2009, time.November, 10, 23, 0, 0, 0, time.UTC)}},
		"<value><struct><member><name>Age</name><value><int>6</int></value></member><member><name>Dates</name><value><struct><member><name>Birth</name><value><dateTime.iso8601>18291110T23:00:00</dateTime.iso8601></value></member><member><name>Death</name><value><dateTime.iso8601>20091110T23:00:00</dateTime.iso8601></value></member></struct></value></member><member><name>Name</name><value><string>John Smith</string></value></member><member><name>Wight</name><value><array><data><value><double>66.67</double></value><value><double>100.5</double></value></data></array></value></member></struct></value>",
	},
	{&struct {
		Title  string
		Amount int
		Author string `xmlrpc:"author,omitempty"`
	}{
		Title: "War and Piece", Amount: 20,
	}, "<value><struct><member><name>Title</name><value><string>War and Piece</string></value></member><member><name>Amount</name><value><int>20</int></value></member></struct></value>"},
	{&struct {
		Title  string
		Amount int
		Author string `xmlrpc:"author,omitempty"`
	}{
		Title: "War and Piece", Amount: 20, Author: "Leo Tolstoy",
	}, "<value><struct><member><name>Title</name><value><string>War and Piece</string></value></member><member><name>Amount</name><value><int>20</int></value></member><member><name>author</name><value><string>Leo Tolstoy</string></value></member></struct></value>"},
	{&struct {
	}{}, "<value><struct></struct></value>"},
}

func Test_marshal(t *testing.T) {
	sortMapKeys = true

	for _, tt := range marshalTests {
		b, err := marshal(tt.value)
		if err != nil {
			t.Fatalf("unexpected marshal error: %v", err)
		}

		if string(b) != tt.xml {
			t.Fatalf("marshal error:\nexpected: %s\n     got: %s", tt.xml, string(b))
		}

	}
}
