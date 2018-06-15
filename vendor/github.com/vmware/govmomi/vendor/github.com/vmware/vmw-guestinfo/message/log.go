// Copyright 2016 VMware, Inc. All Rights Reserved.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//    http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package message

import "log"

var DefaultLogger Logger

type Logger interface {
	Errorf(format string, args ...interface{})
	Debugf(format string, args ...interface{})
	Infof(format string, args ...interface{})
}

func init() {
	DefaultLogger = &logger{}
}

type logger struct {
	DebugLevel bool
}

func (l *logger) Errorf(format string, args ...interface{}) {
	log.Printf(format, args...)
}

func (l *logger) Debugf(format string, args ...interface{}) {
	if !l.DebugLevel {
		return
	}

	log.Printf(format, args...)
}

func (l *logger) Infof(format string, args ...interface{}) {
	log.Printf(format, args...)
}

func Errorf(format string, args ...interface{}) {
	DefaultLogger.Errorf(format, args...)
}

func Debugf(format string, args ...interface{}) {
	DefaultLogger.Debugf(format, args...)
}

func Infof(format string, args ...interface{}) {
	DefaultLogger.Infof(format, args...)
}
