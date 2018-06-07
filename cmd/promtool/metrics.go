package main

import (
	"fmt"
	"io/ioutil"
	"os"
)

func DebugMetrics() int {
	buf, err := NewBodyGetter(BodyGetterConfig{Path: "/metrics"}).Get()
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
	}
	if err := ioutil.WriteFile("metrics.txt", buf, 0644); err != nil {
		fmt.Fprintln(os.Stderr, err)
	}
	fmt.Println(string(buf))
	return 0
}
