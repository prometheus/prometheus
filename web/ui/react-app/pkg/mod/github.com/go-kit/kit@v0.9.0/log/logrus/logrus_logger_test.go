package logrus_test

import (
	"bytes"
	"errors"
	"strings"
	"testing"

	log "github.com/go-kit/kit/log/logrus"
	"github.com/sirupsen/logrus"
)

func TestLogrusLogger(t *testing.T) {
	t.Parallel()
	buf := &bytes.Buffer{}
	logrusLogger := logrus.New()
	logrusLogger.Out = buf
	logrusLogger.Formatter = &logrus.TextFormatter{TimestampFormat: "02-01-2006 15:04:05", FullTimestamp: true}
	logger := log.NewLogrusLogger(logrusLogger)

	if err := logger.Log("hello", "world"); err != nil {
		t.Fatal(err)
	}
	if want, have := "hello=world\n", strings.Split(buf.String(), " ")[3]; want != have {
		t.Errorf("want %#v, have %#v", want, have)
	}

	buf.Reset()
	if err := logger.Log("a", 1, "err", errors.New("error")); err != nil {
		t.Fatal(err)
	}
	if want, have := "a=1 err=error", strings.TrimSpace(strings.SplitAfterN(buf.String(), " ", 4)[3]); want != have {
		t.Errorf("want %#v, have %#v", want, have)
	}

	buf.Reset()
	if err := logger.Log("a", 1, "b"); err != nil {
		t.Fatal(err)
	}
	if want, have := "a=1 b=\"(MISSING)\"", strings.TrimSpace(strings.SplitAfterN(buf.String(), " ", 4)[3]); want != have {
		t.Errorf("want %#v, have %#v", want, have)
	}

	buf.Reset()
	if err := logger.Log("my_map", mymap{0: 0}); err != nil {
		t.Fatal(err)
	}
	if want, have := "my_map=special_behavior", strings.TrimSpace(strings.Split(buf.String(), " ")[3]); want != have {
		t.Errorf("want %#v, have %#v", want, have)
	}
}

type mymap map[int]int

func (m mymap) String() string { return "special_behavior" }
