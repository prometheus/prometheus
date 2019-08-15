package responses

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestUnmarshal(t *testing.T) {
	from := []byte(`{}`)
	to := &struct{}{}
	// support auto json type trans

	err := jsonParser.Unmarshal(from, to)
	assert.Nil(t, err)
	str, err := jsonParser.Marshal(to)
	assert.Nil(t, err)
	assert.Equal(t, `{}`, string(str))
}

func TestUnmarshal_int(t *testing.T) {
	to := &struct {
		INT int
	}{}
	from := []byte(`{"INT":100}`)

	err := jsonParser.Unmarshal(from, to)
	assert.Nil(t, err)
	assert.Equal(t, 100, to.INT)
	str, err := jsonParser.Marshal(to)
	assert.Nil(t, err)
	assert.Equal(t, `{"INT":100}`, string(str))

	from = []byte(`{"INT":100.1}`)

	err = jsonParser.Unmarshal(from, to)
	assert.Nil(t, err)
	assert.Equal(t, 100, to.INT)
	str, err = jsonParser.Marshal(to)
	assert.Nil(t, err)
	assert.Equal(t, `{"INT":100}`, string(str))

	// string to int
	from = []byte(`{"INT":"100"}`)

	err = jsonParser.Unmarshal(from, to)
	assert.Nil(t, err)
	assert.Equal(t, 100, to.INT)
	str, err = jsonParser.Marshal(to)
	assert.Nil(t, err)
	assert.Equal(t, `{"INT":100}`, string(str))

	from = []byte(`{"INT":""}`)

	err = jsonParser.Unmarshal(from, to)
	assert.Nil(t, err)
	assert.Equal(t, 0, to.INT)
	str, err = jsonParser.Marshal(to)
	assert.Nil(t, err)
	assert.Equal(t, `{"INT":0}`, string(str))

	// bool to int
	from = []byte(`{"INT":true}`)

	err = jsonParser.Unmarshal(from, to)
	assert.Nil(t, err)
	assert.Equal(t, 1, to.INT)
	str, err = jsonParser.Marshal(to)
	assert.Nil(t, err)
	assert.Equal(t, `{"INT":1}`, string(str))

	from = []byte(`{"INT":false}`)

	err = jsonParser.Unmarshal(from, to)
	assert.Nil(t, err)
	assert.Equal(t, 0, to.INT)
	str, err = jsonParser.Marshal(to)
	assert.Nil(t, err)
	assert.Equal(t, `{"INT":0}`, string(str))

	// nil to int
	from = []byte(`{"INT":null}`)

	err = jsonParser.Unmarshal(from, to)
	assert.Nil(t, err)
	assert.Equal(t, 0, to.INT)
	str, err = jsonParser.Marshal(to)
	assert.Nil(t, err)
	assert.Equal(t, `{"INT":0}`, string(str))

	// fuzzy decode int
	from = []byte(`{"INT":100000000000000000000000000.1}`)

	err = jsonParser.Unmarshal(from, to)
	assert.NotNil(t, err)
	assert.Equal(t, "struct { INT int }.INT: fuzzy decode int: exceed range, error found in #10 byte of ...|00000000.1|..., bigger context ...|100000000000000000000000000.1|...", err.Error())

	from = []byte(`{"INT":{}}`)

	err = jsonParser.Unmarshal(from, to)
	assert.NotNil(t, err)
	assert.Equal(t, "struct { INT int }.INT: readUint64: unexpected character: \xff, error found in #0 byte of ...||..., bigger context ...||...", err.Error())
}

func TestUnmarshal_uint(t *testing.T) {
	to := &struct {
		UINT uint
	}{}
	from := []byte(`{"UINT":100}`)

	err := jsonParser.Unmarshal(from, to)
	assert.Nil(t, err)
	assert.Equal(t, uint(100), to.UINT)
	str, err := jsonParser.Marshal(to)
	assert.Nil(t, err)
	assert.Equal(t, `{"UINT":100}`, string(str))

	from = []byte(`{"UINT":100.1}`)

	err = jsonParser.Unmarshal(from, to)
	assert.Nil(t, err)
	assert.Equal(t, uint(100), to.UINT)
	str, err = jsonParser.Marshal(to)
	assert.Nil(t, err)
	assert.Equal(t, `{"UINT":100}`, string(str))

	// fuzzy decode uint
	from = []byte(`{"UINT":100000000000000000000000000.1}`)

	err = jsonParser.Unmarshal(from, to)
	assert.NotNil(t, err)
	assert.Equal(t, "struct { UINT uint }.UINT: fuzzy decode uint: exceed range, error found in #10 byte of ...|00000000.1|..., bigger context ...|100000000000000000000000000.1|...", err.Error())
}

func TestUnmarshal_int8(t *testing.T) {
	to := &struct {
		INT8 int8
	}{}
	from := []byte(`{"INT8":100}`)

	err := jsonParser.Unmarshal(from, to)
	assert.Nil(t, err)
	assert.Equal(t, int8(100), to.INT8)
	str, err := jsonParser.Marshal(to)
	assert.Nil(t, err)
	assert.Equal(t, `{"INT8":100}`, string(str))

	from = []byte(`{"INT8":100.1}`)

	err = jsonParser.Unmarshal(from, to)
	assert.Nil(t, err)
	assert.Equal(t, int8(100), to.INT8)
	str, err = jsonParser.Marshal(to)
	assert.Nil(t, err)
	assert.Equal(t, `{"INT8":100}`, string(str))

	// fuzzy decode uint
	from = []byte(`{"INT8":100000000000000000000000000.1}`)

	err = jsonParser.Unmarshal(from, to)
	assert.NotNil(t, err)
	assert.Equal(t, "struct { INT8 int8 }.INT8: fuzzy decode int8: exceed range, error found in #10 byte of ...|00000000.1|..., bigger context ...|100000000000000000000000000.1|...", err.Error())
}

func TestUnmarshal_uint8(t *testing.T) {
	to := &struct {
		UINT8 uint8
	}{}
	from := []byte(`{"UINT8":100}`)

	err := jsonParser.Unmarshal(from, to)
	assert.Nil(t, err)
	assert.Equal(t, uint8(100), to.UINT8)
	str, err := jsonParser.Marshal(to)
	assert.Nil(t, err)
	assert.Equal(t, `{"UINT8":100}`, string(str))

	from = []byte(`{"UINT8":100.1}`)

	err = jsonParser.Unmarshal(from, to)
	assert.Nil(t, err)
	assert.Equal(t, uint8(100), to.UINT8)
	str, err = jsonParser.Marshal(to)
	assert.Nil(t, err)
	assert.Equal(t, `{"UINT8":100}`, string(str))

	// fuzzy decode uint
	from = []byte(`{"UINT8":100000000000000000000000000.1}`)

	err = jsonParser.Unmarshal(from, to)
	assert.NotNil(t, err)
	assert.Equal(t, "struct { UINT8 uint8 }.UINT8: fuzzy decode uint8: exceed range, error found in #10 byte of ...|00000000.1|..., bigger context ...|100000000000000000000000000.1|...", err.Error())
}

func TestUnmarshal_int16(t *testing.T) {
	to := &struct {
		INT16 int16
	}{}
	from := []byte(`{"INT16":100}`)

	err := jsonParser.Unmarshal(from, to)
	assert.Nil(t, err)
	assert.Equal(t, int16(100), to.INT16)
	str, err := jsonParser.Marshal(to)
	assert.Nil(t, err)
	assert.Equal(t, `{"INT16":100}`, string(str))

	from = []byte(`{"INT16":100.1}`)

	err = jsonParser.Unmarshal(from, to)
	assert.Nil(t, err)
	assert.Equal(t, int16(100), to.INT16)
	str, err = jsonParser.Marshal(to)
	assert.Nil(t, err)
	assert.Equal(t, `{"INT16":100}`, string(str))

	// fuzzy decode uint
	from = []byte(`{"INT16":100000000000000000000000000.1}`)

	err = jsonParser.Unmarshal(from, to)
	assert.NotNil(t, err)
	assert.Equal(t, "struct { INT16 int16 }.INT16: fuzzy decode int16: exceed range, error found in #10 byte of ...|00000000.1|..., bigger context ...|100000000000000000000000000.1|...", err.Error())
}

func TestUnmarshal_uint16(t *testing.T) {
	to := &struct {
		UINT16 uint16
	}{}
	from := []byte(`{"UINT16":100}`)

	err := jsonParser.Unmarshal(from, to)
	assert.Nil(t, err)
	assert.Equal(t, uint16(100), to.UINT16)
	str, err := jsonParser.Marshal(to)
	assert.Nil(t, err)
	assert.Equal(t, `{"UINT16":100}`, string(str))

	from = []byte(`{"UINT16":100.1}`)

	err = jsonParser.Unmarshal(from, to)
	assert.Nil(t, err)
	assert.Equal(t, uint16(100), to.UINT16)
	str, err = jsonParser.Marshal(to)
	assert.Nil(t, err)
	assert.Equal(t, `{"UINT16":100}`, string(str))

	// fuzzy decode uint
	from = []byte(`{"UINT16":100000000000000000000000000.1}`)

	err = jsonParser.Unmarshal(from, to)
	assert.NotNil(t, err)
	assert.Equal(t, "struct { UINT16 uint16 }.UINT16: fuzzy decode uint16: exceed range, error found in #10 byte of ...|00000000.1|..., bigger context ...|100000000000000000000000000.1|...", err.Error())
}

func TestUnmarshal_int32(t *testing.T) {
	to := &struct {
		INT32 int32
	}{}
	from := []byte(`{"INT32":100}`)

	err := jsonParser.Unmarshal(from, to)
	assert.Nil(t, err)
	assert.Equal(t, int32(100), to.INT32)
	str, err := jsonParser.Marshal(to)
	assert.Nil(t, err)
	assert.Equal(t, `{"INT32":100}`, string(str))

	from = []byte(`{"INT32":100.1}`)

	err = jsonParser.Unmarshal(from, to)
	assert.Nil(t, err)
	assert.Equal(t, int32(100), to.INT32)
	str, err = jsonParser.Marshal(to)
	assert.Nil(t, err)
	assert.Equal(t, `{"INT32":100}`, string(str))

	// fuzzy decode uint
	from = []byte(`{"INT32":100000000000000000000000000.1}`)

	err = jsonParser.Unmarshal(from, to)
	assert.NotNil(t, err)
	assert.Equal(t, "struct { INT32 int32 }.INT32: fuzzy decode int32: exceed range, error found in #10 byte of ...|00000000.1|..., bigger context ...|100000000000000000000000000.1|...", err.Error())
}

func TestUnmarshal_uint32(t *testing.T) {
	to := &struct {
		UINT32 uint32
	}{}
	from := []byte(`{"UINT32":100}`)

	err := jsonParser.Unmarshal(from, to)
	assert.Nil(t, err)
	assert.Equal(t, uint32(100), to.UINT32)
	str, err := jsonParser.Marshal(to)
	assert.Nil(t, err)
	assert.Equal(t, `{"UINT32":100}`, string(str))

	from = []byte(`{"UINT32":100.1}`)

	err = jsonParser.Unmarshal(from, to)
	assert.Nil(t, err)
	assert.Equal(t, uint32(100), to.UINT32)
	str, err = jsonParser.Marshal(to)
	assert.Nil(t, err)
	assert.Equal(t, `{"UINT32":100}`, string(str))

	// fuzzy decode uint
	from = []byte(`{"UINT32":100000000000000000000000000.1}`)

	err = jsonParser.Unmarshal(from, to)
	assert.NotNil(t, err)
	assert.Equal(t, "struct { UINT32 uint32 }.UINT32: fuzzy decode uint32: exceed range, error found in #10 byte of ...|00000000.1|..., bigger context ...|100000000000000000000000000.1|...", err.Error())
}

func TestUnmarshal_int64(t *testing.T) {
	to := &struct {
		INT64 int64
	}{}
	from := []byte(`{"INT64":100}`)

	err := jsonParser.Unmarshal(from, to)
	assert.Nil(t, err)
	assert.Equal(t, int64(100), to.INT64)
	str, err := jsonParser.Marshal(to)
	assert.Nil(t, err)
	assert.Equal(t, `{"INT64":100}`, string(str))

	from = []byte(`{"INT64":100.1}`)

	err = jsonParser.Unmarshal(from, to)
	assert.Nil(t, err)
	assert.Equal(t, int64(100), to.INT64)
	str, err = jsonParser.Marshal(to)
	assert.Nil(t, err)
	assert.Equal(t, `{"INT64":100}`, string(str))

	// fuzzy decode uint
	from = []byte(`{"INT64":100000000000000000000000000.1}`)

	err = jsonParser.Unmarshal(from, to)
	assert.NotNil(t, err)
	assert.Equal(t, "struct { INT64 int64 }.INT64: fuzzy decode int64: exceed range, error found in #10 byte of ...|00000000.1|..., bigger context ...|100000000000000000000000000.1|...", err.Error())
}

func TestUnmarshal_uint64(t *testing.T) {
	to := &struct {
		UINT64 uint64
	}{}
	from := []byte(`{"UINT64":100}`)

	err := jsonParser.Unmarshal(from, to)
	assert.Nil(t, err)
	assert.Equal(t, uint64(100), to.UINT64)
	str, err := jsonParser.Marshal(to)
	assert.Nil(t, err)
	assert.Equal(t, `{"UINT64":100}`, string(str))

	from = []byte(`{"UINT64":100.1}`)

	err = jsonParser.Unmarshal(from, to)
	assert.Nil(t, err)
	assert.Equal(t, uint64(100), to.UINT64)
	str, err = jsonParser.Marshal(to)
	assert.Nil(t, err)
	assert.Equal(t, `{"UINT64":100}`, string(str))

	// fuzzy decode uint
	from = []byte(`{"UINT64":100000000000000000000000000.1}`)

	err = jsonParser.Unmarshal(from, to)
	assert.NotNil(t, err)
	assert.Equal(t, "struct { UINT64 uint64 }.UINT64: fuzzy decode uint64: exceed range, error found in #10 byte of ...|00000000.1|..., bigger context ...|100000000000000000000000000.1|...", err.Error())
}

func TestUnmarshal_string(t *testing.T) {
	to := &struct {
		STRING string
	}{}
	// string to string
	from := []byte(`{"STRING":""}`)

	err := jsonParser.Unmarshal(from, to)
	assert.Nil(t, err)
	assert.Equal(t, "", to.STRING)
	str, err := jsonParser.Marshal(to)
	assert.Nil(t, err)
	assert.Equal(t, `{"STRING":""}`, string(str))

	// number to string
	from = []byte(`{"STRING":100}`)

	err = jsonParser.Unmarshal(from, to)
	assert.Nil(t, err)
	assert.Equal(t, "100", to.STRING)
	str, err = jsonParser.Marshal(to)
	assert.Nil(t, err)
	assert.Equal(t, `{"STRING":"100"}`, string(str))

	// bool to string
	from = []byte(`{"STRING":true}`)

	err = jsonParser.Unmarshal(from, to)
	assert.Nil(t, err)
	assert.Equal(t, "true", to.STRING)
	str, err = jsonParser.Marshal(to)
	assert.Nil(t, err)
	assert.Equal(t, `{"STRING":"true"}`, string(str))

	// nil to string
	from = []byte(`{"STRING":null}`)

	err = jsonParser.Unmarshal(from, to)
	assert.Nil(t, err)
	assert.Equal(t, "", to.STRING)
	str, err = jsonParser.Marshal(to)
	assert.Nil(t, err)
	assert.Equal(t, `{"STRING":""}`, string(str))

	// other to string
	from = []byte(`{"STRING":{}}`)

	err = jsonParser.Unmarshal(from, to)
	assert.NotNil(t, err)
	assert.Equal(t, "struct { STRING string }.STRING: fuzzyStringDecoder: not number or string or bool, error found in #10 byte of ...|{\"STRING\":{}}|..., bigger context ...|{\"STRING\":{}}|...", err.Error())
}

func TestUnmarshal_bool(t *testing.T) {
	to := &struct {
		BOOL bool
	}{}
	// bool to bool
	from := []byte(`{"BOOL":true}`)

	err := jsonParser.Unmarshal(from, to)
	assert.Nil(t, err)
	assert.Equal(t, true, to.BOOL)
	str, err := jsonParser.Marshal(to)
	assert.Nil(t, err)
	assert.Equal(t, `{"BOOL":true}`, string(str))

	// number to bool
	from = []byte(`{"BOOL":100}`)

	err = jsonParser.Unmarshal(from, to)
	assert.Nil(t, err)
	assert.Equal(t, true, to.BOOL)
	str, err = jsonParser.Marshal(to)
	assert.Nil(t, err)
	assert.Equal(t, `{"BOOL":true}`, string(str))

	from = []byte(`{"BOOL":0}`)

	err = jsonParser.Unmarshal(from, to)
	assert.Nil(t, err)
	assert.Equal(t, false, to.BOOL)
	str, err = jsonParser.Marshal(to)
	assert.Nil(t, err)
	assert.Equal(t, `{"BOOL":false}`, string(str))

	// invalid number literal
	from = []byte(`{"BOOL": 1000000000000000000000000000000000000000}`)

	err = jsonParser.Unmarshal(from, to)
	assert.NotNil(t, err)
	assert.Equal(t, "struct { BOOL bool }.BOOL: fuzzyBoolDecoder: get value from json.number failed, error found in #10 byte of ...|0000000000}|..., bigger context ...|{\"BOOL\": 1000000000000000000000000000000000000000}|...", err.Error())

	// bool to string
	from = []byte(`{"BOOL":"true"}`)

	err = jsonParser.Unmarshal(from, to)
	assert.Nil(t, err)
	assert.Equal(t, true, to.BOOL)
	str, err = jsonParser.Marshal(to)
	assert.Nil(t, err)
	assert.Equal(t, `{"BOOL":true}`, string(str))

	from = []byte(`{"BOOL":"false"}`)

	err = jsonParser.Unmarshal(from, to)
	assert.Nil(t, err)
	assert.Equal(t, false, to.BOOL)
	str, err = jsonParser.Marshal(to)
	assert.Nil(t, err)
	assert.Equal(t, `{"BOOL":false}`, string(str))

	from = []byte(`{"BOOL":"other"}`)

	err = jsonParser.Unmarshal(from, to)
	assert.NotNil(t, err)
	assert.Equal(t, "struct { BOOL bool }.BOOL: fuzzyBoolDecoder: unsupported bool value: other, error found in #10 byte of ...|L\":\"other\"}|..., bigger context ...|{\"BOOL\":\"other\"}|...", err.Error())

	// nil to bool
	from = []byte(`{"BOOL":null}`)

	err = jsonParser.Unmarshal(from, to)
	assert.Nil(t, err)
	assert.Equal(t, false, to.BOOL)
	str, err = jsonParser.Marshal(to)
	assert.Nil(t, err)
	assert.Equal(t, `{"BOOL":false}`, string(str))

	// other to string
	from = []byte(`{"BOOL":{}}`)

	err = jsonParser.Unmarshal(from, to)
	assert.NotNil(t, err)
	assert.Equal(t, "struct { BOOL bool }.BOOL: fuzzyBoolDecoder: not number or string or nil, error found in #8 byte of ...|{\"BOOL\":{}}|..., bigger context ...|{\"BOOL\":{}}|...", err.Error())
}

func TestUnmarshal_array(t *testing.T) {
	to := &struct {
		Array []string
	}{}
	// bool to bool
	from := []byte(`{"Array":[]}`)

	err := jsonParser.Unmarshal(from, to)
	assert.Nil(t, err)
	assert.Equal(t, 0, len(to.Array))
	str, err := jsonParser.Marshal(to)
	assert.Nil(t, err)
	assert.Equal(t, `{"Array":[]}`, string(str))
}

func TestUnmarshal_float32(t *testing.T) {
	to := &struct {
		FLOAT32 float32
	}{}
	from := []byte(`{"FLOAT32":100}`)

	err := jsonParser.Unmarshal(from, to)
	assert.Nil(t, err)
	assert.Equal(t, float32(100), to.FLOAT32)
	str, err := jsonParser.Marshal(to)
	assert.Nil(t, err)
	assert.Equal(t, `{"FLOAT32":100}`, string(str))

	from = []byte(`{"FLOAT32":100.1}`)

	err = jsonParser.Unmarshal(from, to)
	assert.Nil(t, err)
	assert.Equal(t, float32(100.1), to.FLOAT32)
	str, err = jsonParser.Marshal(to)
	assert.Nil(t, err)
	assert.Equal(t, `{"FLOAT32":100.1}`, string(str))

	// string to float32
	from = []byte(`{"FLOAT32":"100.1"}`)

	err = jsonParser.Unmarshal(from, to)
	assert.Nil(t, err)
	assert.Equal(t, float32(100.1), to.FLOAT32)
	str, err = jsonParser.Marshal(to)
	assert.Nil(t, err)
	assert.Equal(t, `{"FLOAT32":100.1}`, string(str))

	from = []byte(`{"FLOAT32":""}`)

	err = jsonParser.Unmarshal(from, to)
	assert.Nil(t, err)
	assert.Equal(t, float32(0), to.FLOAT32)
	str, err = jsonParser.Marshal(to)
	assert.Nil(t, err)
	assert.Equal(t, `{"FLOAT32":0}`, string(str))

	// error branch
	from = []byte(`{"FLOAT32":"."}`)

	err = jsonParser.Unmarshal(from, to)
	assert.NotNil(t, err)
	assert.Equal(t, "struct { FLOAT32 float32 }.FLOAT32: readFloat32: leading dot is invalid, error found in #0 byte of ...|.|..., bigger context ...|.|...", err.Error())

	// bool to float32
	from = []byte(`{"FLOAT32":true}`)

	err = jsonParser.Unmarshal(from, to)
	assert.Nil(t, err)
	assert.Equal(t, float32(1), to.FLOAT32)
	str, err = jsonParser.Marshal(to)
	assert.Nil(t, err)
	assert.Equal(t, `{"FLOAT32":1}`, string(str))

	from = []byte(`{"FLOAT32":false}`)

	err = jsonParser.Unmarshal(from, to)
	assert.Nil(t, err)
	assert.Equal(t, float32(0), to.FLOAT32)
	str, err = jsonParser.Marshal(to)
	assert.Nil(t, err)
	assert.Equal(t, `{"FLOAT32":0}`, string(str))

	// nil to float32
	from = []byte(`{"FLOAT32":null}`)

	err = jsonParser.Unmarshal(from, to)
	assert.Nil(t, err)
	assert.Equal(t, float32(0), to.FLOAT32)
	str, err = jsonParser.Marshal(to)
	assert.Nil(t, err)
	assert.Equal(t, `{"FLOAT32":0}`, string(str))

	// others to float32
	from = []byte(`{"FLOAT32":{}}`)

	err = jsonParser.Unmarshal(from, to)
	assert.NotNil(t, err)
	assert.Equal(t, "struct { FLOAT32 float32 }.FLOAT32: nullableFuzzyFloat32Decoder: not number or string, error found in #10 byte of ...|\"FLOAT32\":{}}|..., bigger context ...|{\"FLOAT32\":{}}|...", err.Error())
}

func TestUnmarshal_float64(t *testing.T) {
	to := &struct {
		FLOAT64 float64
	}{}
	from := []byte(`{"FLOAT64":100}`)

	err := jsonParser.Unmarshal(from, to)
	assert.Nil(t, err)
	assert.Equal(t, float64(100), to.FLOAT64)
	str, err := jsonParser.Marshal(to)
	assert.Nil(t, err)
	assert.Equal(t, `{"FLOAT64":100}`, string(str))

	from = []byte(`{"FLOAT64":100.1}`)

	err = jsonParser.Unmarshal(from, to)
	assert.Nil(t, err)
	assert.Equal(t, float64(100.1), to.FLOAT64)
	str, err = jsonParser.Marshal(to)
	assert.Nil(t, err)
	assert.Equal(t, `{"FLOAT64":100.1}`, string(str))

	// string to float64
	from = []byte(`{"FLOAT64":"100.1"}`)

	err = jsonParser.Unmarshal(from, to)
	assert.Nil(t, err)
	assert.Equal(t, float64(100.1), to.FLOAT64)
	str, err = jsonParser.Marshal(to)
	assert.Nil(t, err)
	assert.Equal(t, `{"FLOAT64":100.1}`, string(str))

	from = []byte(`{"FLOAT64":""}`)

	err = jsonParser.Unmarshal(from, to)
	assert.Nil(t, err)
	assert.Equal(t, float64(0), to.FLOAT64)
	str, err = jsonParser.Marshal(to)
	assert.Nil(t, err)
	assert.Equal(t, `{"FLOAT64":0}`, string(str))

	// error branch
	from = []byte(`{"FLOAT64":"."}`)

	err = jsonParser.Unmarshal(from, to)
	assert.NotNil(t, err)
	assert.Equal(t, "struct { FLOAT64 float64 }.FLOAT64: readFloat64: leading dot is invalid, error found in #0 byte of ...|.|..., bigger context ...|.|...", err.Error())

	// bool to float64
	from = []byte(`{"FLOAT64":true}`)

	err = jsonParser.Unmarshal(from, to)
	assert.Nil(t, err)
	assert.Equal(t, float64(1), to.FLOAT64)
	str, err = jsonParser.Marshal(to)
	assert.Nil(t, err)
	assert.Equal(t, `{"FLOAT64":1}`, string(str))

	from = []byte(`{"FLOAT64":false}`)

	err = jsonParser.Unmarshal(from, to)
	assert.Nil(t, err)
	assert.Equal(t, float64(0), to.FLOAT64)
	str, err = jsonParser.Marshal(to)
	assert.Nil(t, err)
	assert.Equal(t, `{"FLOAT64":0}`, string(str))

	// nil to float64
	from = []byte(`{"FLOAT64":null}`)

	err = jsonParser.Unmarshal(from, to)
	assert.Nil(t, err)
	assert.Equal(t, float64(0), to.FLOAT64)
	str, err = jsonParser.Marshal(to)
	assert.Nil(t, err)
	assert.Equal(t, `{"FLOAT64":0}`, string(str))

	// others to float64
	from = []byte(`{"FLOAT64":{}}`)

	err = jsonParser.Unmarshal(from, to)
	assert.NotNil(t, err)
	assert.Equal(t, "struct { FLOAT64 float64 }.FLOAT64: nullableFuzzyFloat64Decoder: not number or string, error found in #10 byte of ...|\"FLOAT64\":{}}|..., bigger context ...|{\"FLOAT64\":{}}|...", err.Error())
}

func TestUnmarshalWithArray(t *testing.T) {
	from := []byte(`[]`)
	to := &struct{}{}
	// TODO: Must support Array
	// support auto json type trans
	err := jsonParser.Unmarshal(from, to)
	assert.NotNil(t, err)
}
