package utils

import (
	"regexp"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestInitStructWithDefaultTag(t *testing.T) {
	config := &struct {
		B bool          `default:"true"`
		S string        `default:"default string"`
		I int           `default:"10"`
		T time.Duration `default:"100"`
		E int           `default:""`
	}{}
	InitStructWithDefaultTag(config)
	assert.NotNil(t, config)
	assert.Equal(t, true, config.B)
	assert.Equal(t, "default string", config.S)
	assert.Equal(t, 10, config.I)
	assert.Equal(t, time.Duration(100), config.T)
	assert.Equal(t, 0, config.E)
}

func TestGetUUID(t *testing.T) {
	uuid := NewUUID()
	assert.Equal(t, 16, len(uuid))
	assert.Equal(t, 36, len(uuid.String()))
	uuidString := GetUUID()
	assert.Equal(t, 32, len(uuidString))
}

func TestGetMD5Base64(t *testing.T) {
	assert.Equal(t, "ERIHLmRX2uZmssDdxQnnxQ==",
		GetMD5Base64([]byte("That's all folks!!")))
	assert.Equal(t, "GsJRdI3kAbAnHo/0+3wWJw==",
		GetMD5Base64([]byte("中文也没啥问题")))
}

func TestGetTimeInFormatRFC2616(t *testing.T) {
	s := GetTimeInFormatRFC2616()
	assert.Equal(t, 29, len(s))
	re := regexp.MustCompile(`^[A-Z][a-z]{2}, [0-9]{2} [A-Z][a-z]{2} [0-9]{4} [0-9]{2}:[0-9]{2}:[0-9]{2} GMT$`)
	assert.True(t, re.MatchString(s))
}

func TestGetTimeInFormatISO8601(t *testing.T) {
	s := GetTimeInFormatISO8601()
	assert.Equal(t, 20, len(s))
	// 2006-01-02T15:04:05Z
	re := regexp.MustCompile(`^[0-9]{4}-[0-9]{2}-[0-9]{2}T[0-9]{2}:[0-9]{2}:[0-9]{2}Z$`)
	assert.True(t, re.MatchString(s))
}

func TestGetUrlFormedMap(t *testing.T) {
	m := make(map[string]string)
	m["key"] = "value"
	s := GetUrlFormedMap(m)
	assert.Equal(t, "key=value", s)
	m["key2"] = "http://domain/?key=value&key2=value2"
	s2 := GetUrlFormedMap(m)
	assert.Equal(t, "key=value&key2=http%3A%2F%2Fdomain%2F%3Fkey%3Dvalue%26key2%3Dvalue2", s2)
}
