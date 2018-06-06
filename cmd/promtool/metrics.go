package main

import (
	"fmt"
	"io/ioutil"
	"os"
)

func GetMetrics() ([]byte, error) {
	_, body, err := promClient.Do("/metrics")

	return body, err
}

func DebugMetrics() int {
	buf, err := GetMetrics()
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
	}
	if err := ioutil.WriteFile("metrics.txt", buf, 0644); err != nil {
		fmt.Fprintln(os.Stderr, err)
	}
	fmt.Println(string(buf))
	return 0
}
