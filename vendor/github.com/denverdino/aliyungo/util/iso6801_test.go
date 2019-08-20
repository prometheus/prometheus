package util

import (
	"encoding/json"
	"testing"
	"time"
)

func TestISO8601Time(t *testing.T) {
	now := NewISO6801Time(time.Now().UTC())

	data, err := json.Marshal(now)
	if err != nil {
		t.Fatal(err)
	}

	_, err = time.Parse(`"`+formatISO8601+`"`, string(data))
	if err != nil {
		t.Fatal(err)
	}

	var now2 ISO6801Time
	err = json.Unmarshal(data, &now2)
	if err != nil {
		t.Fatal(err)
	}

	if now != now2 {
		t.Errorf("Time %s does not equal expected %s", now2, now)
	}

	if now.String() != now2.String() {
		t.Fatalf("String format for %s does not equal expected %s", now2, now)
	}

	type TestTimeStruct struct {
		A int
		B *ISO6801Time
	}
	var testValue TestTimeStruct
	err = json.Unmarshal([]byte("{\"A\": 1, \"B\":\"\"}"), &testValue)
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("%v", testValue)
	if !testValue.B.IsDefault() {
		t.Fatal("Invaid Unmarshal result for ISO6801Time from empty value")
	}
	t.Logf("ISO6801Time String(): %s", now2.String())
}

func TestISO8601TimeWithoutSeconds(t *testing.T) {

	const dateStr = "\"2015-10-02T12:36Z\""

	var date ISO6801Time

	err := json.Unmarshal([]byte(dateStr), &date)
	if err != nil {
		t.Fatal(err)
	}

	const dateStr2 = "\"2015-10-02T12:36:00Z\""

	var date2 ISO6801Time

	err = json.Unmarshal([]byte(dateStr2), &date2)
	if err != nil {
		t.Fatal(err)
	}

	if date != date2 {
		t.Error("The two dates should be equal.")
	}

}

func TestISO8601TimeInt(t *testing.T) {

	const dateStr = "1405544146000"

	var date ISO6801Time

	err := json.Unmarshal([]byte(dateStr), &date)
	if err != nil {
		t.Fatal(err)
	}

	t.Logf("date: %s", date)

}
