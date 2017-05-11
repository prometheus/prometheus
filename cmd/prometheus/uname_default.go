// +build !linux

package main

// Uname for any platform other than linux
func Uname() string {
	return "(Printing uname isn't yet supported on this platform)"
}
