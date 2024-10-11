package logger

func noop(string, ...interface{}) {}

// These variables are set by the common log package.
var (
	Errorf = noop
	Warnf  = noop
	Infof  = noop
	Debugf = noop
)
