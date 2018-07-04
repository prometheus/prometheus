// Copyright (c) 2016, 2018, Oracle and/or its affiliates. All rights reserved.

package common

import (
	"fmt"
	"io/ioutil"
	"log"
	"os"
	"sync"
)

// Simple logging proxy to distinguish control for logging messages
// Debug logging is turned on/off by the presence of the environment variable "OCI_GO_SDK_DEBUG"
var debugLog = log.New(os.Stderr, "DEBUG ", log.Ldate|log.Ltime|log.Lshortfile)
var mainLog = log.New(os.Stderr, "", log.Ldate|log.Ltime|log.Lshortfile)
var isDebugLogEnabled bool
var checkDebug sync.Once

func setOutputForEnv() {
	checkDebug.Do(func() {
		isDebugLogEnabled = *new(bool)
		_, isDebugLogEnabled = os.LookupEnv("OCI_GO_SDK_DEBUG")

		if !isDebugLogEnabled {
			debugLog.SetOutput(ioutil.Discard)
		}
	})
}

// Debugf logs v with the provided format if debug mode is set
func Debugf(format string, v ...interface{}) {
	setOutputForEnv()
	debugLog.Output(3, fmt.Sprintf(format, v...))
}

// Debug  logs v if debug mode is set
func Debug(v ...interface{}) {
	setOutputForEnv()
	debugLog.Output(3, fmt.Sprint(v...))
}

// Debugln logs v appending a new line if debug mode is set
func Debugln(v ...interface{}) {
	setOutputForEnv()
	debugLog.Output(3, fmt.Sprintln(v...))
}

// IfDebug executes closure if debug is enabled
func IfDebug(fn func()) {
	if isDebugLogEnabled {
		fn()
	}
}

// Logln logs v appending a new line at the end
func Logln(v ...interface{}) {
	mainLog.Output(3, fmt.Sprintln(v...))
}

// Logf logs v with the provided format
func Logf(format string, v ...interface{}) {
	mainLog.Output(3, fmt.Sprintf(format, v...))
}
