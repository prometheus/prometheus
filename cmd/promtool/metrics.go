package main

import (
	"fmt"
	"io/ioutil"
	"net/http"
	"os"
)

var debugMetrics DebugMetrics

type DebugMetrics struct {
	Request *http.Request
}

func initDebugMetrics(path string) {
	req, err := http.NewRequest(http.MethodGet, promClient.Server.String()+path, nil)
	if err != nil {
		panic(err)
	}
	debugMetrics = DebugMetrics{
		Request: req,
	}
}

func (c *DebugMetrics) Get() ([]byte, error) {
	_, body, err := promClient.Do(c.Request)

	return body, err
}

func (c *DebugMetrics) Exec() int {
	buf, err := c.Get()
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
	}
	if err := ioutil.WriteFile("metrics.txt", buf, 0644); err != nil {
		fmt.Fprintln(os.Stderr, err)
	}
	fmt.Println(string(buf))
	return 0
}
