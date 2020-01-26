package klog

import (
	"fmt"
	"math"
	"os"
	"sync"

	"github.com/go-kit/kit/log"
	"github.com/go-kit/kit/log/level"
)

var maxLevel Level = math.MaxInt32
var logger = log.NewNopLogger()
var mu = sync.Mutex{}

// SetLogger redirects klog logging to the given logger.
// It must be called prior any call to klog.
func SetLogger(l log.Logger) {
	mu.Lock()
	logger = l
	mu.Unlock()
}

// ClampLevel clamps the leveled logging at the specified value.
// It must be called prior any call to klog.
func ClampLevel(l Level) {
	mu.Lock()
	maxLevel = l
	mu.Unlock()
}

type Level int32

type Verbose bool

func V(level Level) Verbose { return level <= maxLevel }

func (v Verbose) Info(args ...interface{}) {
	if v {
		level.Debug(logger).Log("func", "Verbose.Info", "msg", fmt.Sprint(args...))
	}
}

func (v Verbose) Infoln(args ...interface{}) {
	if v {
		level.Debug(logger).Log("func", "Verbose.Infoln", "msg", fmt.Sprint(args...))
	}
}

func (v Verbose) Infof(format string, args ...interface{}) {
	if v {
		level.Debug(logger).Log("func", "Verbose.Infof", "msg", fmt.Sprintf(format, args...))
	}
}

func Info(args ...interface{}) {
	level.Debug(logger).Log("func", "Info", "msg", fmt.Sprint(args...))
}

func InfoDepth(depth int, args ...interface{}) {
	level.Debug(logger).Log("func", "InfoDepth", "msg", fmt.Sprint(args...))
}

func Infoln(args ...interface{}) {
	level.Debug(logger).Log("func", "Infoln", "msg", fmt.Sprint(args...))
}

func Infof(format string, args ...interface{}) {
	level.Debug(logger).Log("func", "Infof", "msg", fmt.Sprintf(format, args...))
}

func Warning(args ...interface{}) {
	level.Warn(logger).Log("func", "Warning", "msg", fmt.Sprint(args...))
}

func WarningDepth(depth int, args ...interface{}) {
	level.Warn(logger).Log("func", "WarningDepth", "msg", fmt.Sprint(args...))
}

func Warningln(args ...interface{}) {
	level.Warn(logger).Log("func", "Warningln", "msg", fmt.Sprint(args...))
}

func Warningf(format string, args ...interface{}) {
	level.Warn(logger).Log("func", "Warningf", "msg", fmt.Sprintf(format, args...))
}

func Error(args ...interface{}) {
	level.Error(logger).Log("func", "Error", "msg", fmt.Sprint(args...))
}

func ErrorDepth(depth int, args ...interface{}) {
	level.Error(logger).Log("func", "ErrorDepth", "msg", fmt.Sprint(args...))
}

func Errorln(args ...interface{}) {
	level.Error(logger).Log("func", "Errorln", "msg", fmt.Sprint(args...))
}

func Errorf(format string, args ...interface{}) {
	level.Error(logger).Log("func", "Errorf", "msg", fmt.Sprintf(format, args...))
}

func Fatal(args ...interface{}) {
	level.Error(logger).Log("func", "Fatal", "msg", fmt.Sprint(args...))
	os.Exit(255)
}

func FatalDepth(depth int, args ...interface{}) {
	level.Error(logger).Log("func", "FatalDepth", "msg", fmt.Sprint(args...))
	os.Exit(255)
}

func Fatalln(args ...interface{}) {
	level.Error(logger).Log("func", "Fatalln", "msg", fmt.Sprint(args...))
	os.Exit(255)
}

func Fatalf(format string, args ...interface{}) {
	level.Error(logger).Log("func", "Fatalf", "msg", fmt.Sprintf(format, args...))
	os.Exit(255)
}

func Exit(args ...interface{}) {
	level.Error(logger).Log("func", "Exit", "msg", fmt.Sprint(args...))
	os.Exit(1)
}

func ExitDepth(depth int, args ...interface{}) {
	level.Error(logger).Log("func", "ExitDepth", "msg", fmt.Sprint(args...))
	os.Exit(1)
}

func Exitln(args ...interface{}) {
	level.Error(logger).Log("func", "Exitln", "msg", fmt.Sprint(args...))
	os.Exit(1)
}

func Exitf(format string, args ...interface{}) {
	level.Error(logger).Log("func", "Exitf", "msg", fmt.Sprintf(format, args...))
	os.Exit(1)
}
