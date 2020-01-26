package misc_tests

import (
	"bytes"
	"testing"

	"github.com/json-iterator/go"
	"github.com/stretchr/testify/require"
	"strings"
	"time"
)

func Test_empty_object(t *testing.T) {
	should := require.New(t)
	iter := jsoniter.ParseString(jsoniter.ConfigDefault, `{}`)
	field := iter.ReadObject()
	should.Equal("", field)
	iter = jsoniter.ParseString(jsoniter.ConfigDefault, `{}`)
	iter.ReadObjectCB(func(iter *jsoniter.Iterator, field string) bool {
		should.FailNow("should not call")
		return true
	})
}

func Test_one_field(t *testing.T) {
	should := require.New(t)
	iter := jsoniter.ParseString(jsoniter.ConfigDefault, `{"a": "stream"}`)
	field := iter.ReadObject()
	should.Equal("a", field)
	value := iter.ReadString()
	should.Equal("stream", value)
	field = iter.ReadObject()
	should.Equal("", field)
	iter = jsoniter.ParseString(jsoniter.ConfigDefault, `{"a": "stream"}`)
	should.True(iter.ReadObjectCB(func(iter *jsoniter.Iterator, field string) bool {
		should.Equal("a", field)
		iter.Skip()
		return true
	}))

}

func Test_two_field(t *testing.T) {
	should := require.New(t)
	iter := jsoniter.ParseString(jsoniter.ConfigDefault, `{ "a": "stream" , "c": "d" }`)
	field := iter.ReadObject()
	should.Equal("a", field)
	value := iter.ReadString()
	should.Equal("stream", value)
	field = iter.ReadObject()
	should.Equal("c", field)
	value = iter.ReadString()
	should.Equal("d", value)
	field = iter.ReadObject()
	should.Equal("", field)
	iter = jsoniter.ParseString(jsoniter.ConfigDefault, `{"field1": "1", "field2": 2}`)
	for field := iter.ReadObject(); field != ""; field = iter.ReadObject() {
		switch field {
		case "field1":
			iter.ReadString()
		case "field2":
			iter.ReadInt64()
		default:
			iter.ReportError("bind object", "unexpected field")
		}
	}
}

func Test_write_object(t *testing.T) {
	should := require.New(t)
	buf := &bytes.Buffer{}
	stream := jsoniter.NewStream(jsoniter.Config{IndentionStep: 2}.Froze(), buf, 4096)
	stream.WriteObjectStart()
	stream.WriteObjectField("hello")
	stream.WriteInt(1)
	stream.WriteMore()
	stream.WriteObjectField("world")
	stream.WriteInt(2)
	stream.WriteObjectEnd()
	stream.Flush()
	should.Nil(stream.Error)
	should.Equal("{\n  \"hello\": 1,\n  \"world\": 2\n}", buf.String())
}

func Test_reader_and_load_more(t *testing.T) {
	should := require.New(t)
	type TestObject struct {
		CreatedAt time.Time
	}
	reader := strings.NewReader(`
{
	"agency": null,
	"candidateId": 0,
	"candidate": "Blah Blah",
	"bookingId": 0,
	"shiftId": 1,
	"shiftTypeId": 0,
	"shift": "Standard",
	"bonus": 0,
	"bonusNI": 0,
	"days": [],
	"totalHours": 27,
	"expenses": [],
	"weekEndingDateSystem": "2016-10-09",
	"weekEndingDateClient": "2016-10-09",
	"submittedAt": null,
	"submittedById": null,
	"approvedAt": "2016-10-10T18:38:04Z",
	"approvedById": 0,
	"authorisedAt": "2016-10-10T18:38:04Z",
	"authorisedById": 0,
	"invoicedAt": "2016-10-10T20:00:00Z",
	"revokedAt": null,
	"revokedById": null,
	"revokeReason": null,
	"rejectedAt": null,
	"rejectedById": null,
	"rejectReasonCode": null,
	"rejectReason": null,
	"createdAt": "2016-10-03T00:00:00Z",
	"updatedAt": "2016-11-09T10:26:13Z",
	"updatedById": null,
	"overrides": [],
	"bookingApproverId": null,
	"bookingApprover": null,
	"status": "approved"
}
	`)
	decoder := jsoniter.ConfigCompatibleWithStandardLibrary.NewDecoder(reader)
	obj := TestObject{}
	should.Nil(decoder.Decode(&obj))
}

func Test_unmarshal_into_existing_value(t *testing.T) {
	should := require.New(t)
	type TestObject struct {
		Field1 int
		Field2 interface{}
	}
	var obj TestObject
	m := map[string]interface{}{}
	obj.Field2 = &m
	cfg := jsoniter.Config{UseNumber: true}.Froze()
	err := cfg.Unmarshal([]byte(`{"Field1":1,"Field2":{"k":"v"}}`), &obj)
	should.NoError(err)
	should.Equal(map[string]interface{}{
		"k": "v",
	}, m)
}
