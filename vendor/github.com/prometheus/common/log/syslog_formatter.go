package log

import (
	"fmt"
	"log/syslog"
	"os"

	"github.com/Sirupsen/logrus"
)

var ceeTag = []byte("@cee:")

type syslogger struct {
	wrap logrus.Formatter
	out  *syslog.Writer
}

func newSyslogger(appname string, facility string, fmter logrus.Formatter) (*syslogger, error) {
	priority, err := getFacility(facility)
	if err != nil {
		return nil, err
	}
	out, err := syslog.New(priority, appname)
	return &syslogger{
		out:  out,
		wrap: fmter,
	}, err
}

func getFacility(facility string) (syslog.Priority, error) {
	switch facility {
	case "0":
		return syslog.LOG_LOCAL0, nil
	case "1":
		return syslog.LOG_LOCAL1, nil
	case "2":
		return syslog.LOG_LOCAL2, nil
	case "3":
		return syslog.LOG_LOCAL3, nil
	case "4":
		return syslog.LOG_LOCAL4, nil
	case "5":
		return syslog.LOG_LOCAL5, nil
	case "6":
		return syslog.LOG_LOCAL6, nil
	case "7":
		return syslog.LOG_LOCAL7, nil
	}
	return syslog.LOG_LOCAL0, fmt.Errorf("invalid local(%s) for syslog", facility)
}

func (s *syslogger) Format(e *logrus.Entry) ([]byte, error) {
	data, err := s.wrap.Format(e)
	if err != nil {
		fmt.Fprintf(os.Stderr, "syslogger: can't format entry: %v\n", err)
		return data, err
	}
	// only append tag to data sent to syslog (line), not to what
	// is returned
	line := string(append(ceeTag, data...))

	switch e.Level {
	case logrus.PanicLevel:
		err = s.out.Crit(line)
	case logrus.FatalLevel:
		err = s.out.Crit(line)
	case logrus.ErrorLevel:
		err = s.out.Err(line)
	case logrus.WarnLevel:
		err = s.out.Warning(line)
	case logrus.InfoLevel:
		err = s.out.Info(line)
	case logrus.DebugLevel:
		err = s.out.Debug(line)
	default:
		err = s.out.Notice(line)
	}

	if err != nil {
		fmt.Fprintf(os.Stderr, "syslogger: can't send log to syslog: %v\n", err)
	}

	return data, err
}
