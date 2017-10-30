package eureka

import (
	"github.com/ArthurHlt/gominlog"
	"log"
)

var logger *gominlog.MinLog

func GetLogger() *log.Logger {
	return logger.GetLogger()
}

func SetLogger(loggerLog *log.Logger) {
	logger.SetLogger(loggerLog)
}

func init() {
	// Default logger uses the go default log.
	logger = gominlog.NewClassicMinLogWithPackageName("go-eureka-client")
}
