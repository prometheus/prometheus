// Package logrus provides an adapter to the
// go-kit log.Logger interface.
package logrus

import (
	"errors"
	"fmt"

	"github.com/go-kit/kit/log"
	"github.com/sirupsen/logrus"
)

type logrusLogger struct {
	logrus.FieldLogger
}

var errMissingValue = errors.New("(MISSING)")

// NewLogrusLogger returns a go-kit log.Logger that sends log events to a Logrus logger.
func NewLogrusLogger(logger logrus.FieldLogger) log.Logger {
	return &logrusLogger{logger}
}

func (l logrusLogger) Log(keyvals ...interface{}) error {
	fields := logrus.Fields{}
	for i := 0; i < len(keyvals); i += 2 {
		if i+1 < len(keyvals) {
			fields[fmt.Sprint(keyvals[i])] = keyvals[i+1]
		} else {
			fields[fmt.Sprint(keyvals[i])] = errMissingValue
		}
	}
	l.WithFields(fields).Info()
	return nil
}
