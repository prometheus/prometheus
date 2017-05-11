package main

import (
	"log"
	"syscall"
)

func charsToString(ca []int8) string {
	s := make([]byte, len(ca))
	var lens int
	for ; lens < len(ca); lens++ {
		if ca[lens] == 0 {
			break
		}
		s[lens] = uint8(ca[lens])
	}
	return string(s[0:lens])
}

// Uname returns the uname of the host machine
func Uname() string {
	buf := syscall.Utsname{}
	err := syscall.Uname(&buf)
	if err != nil {
		log.Fatal("Error!")
	}
	str := " " + "(" + charsToString(buf.Sysname[:])
	str += " " + charsToString(buf.Release[:])
	str += " " + charsToString(buf.Version[:])
	str += " " + charsToString(buf.Machine[:])
	str += " " + charsToString(buf.Nodename[:])
	str += " " + charsToString(buf.Domainname[:]) + ")"
	return str
}
