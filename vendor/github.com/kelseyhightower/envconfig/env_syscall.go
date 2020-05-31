// +build !appengine,!go1.5

package envconfig

import "syscall"

var lookupEnv = syscall.Getenv
