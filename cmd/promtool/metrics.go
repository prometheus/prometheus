package main

import (
	"context"
	"fmt"
	"io/ioutil"
	"net/http"
	"net/url"
	"os"
	"time"

	"github.com/prometheus/client_golang/api"
)

func GetMetrics(url string) ([]byte, error) {
	config := api.Config{
		Address: url,
	}

	c, err := api.NewClient(config)
	if err != nil {
		return nil, err
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	req, err := http.NewRequest(http.MethodGet, url+"/metrics", nil)
	if err != nil {
		return nil, err
	}

	_, body, err := c.Do(ctx, req)
	if err != nil {
		return nil, err
	}
	cancel()

	return body, nil
}

func DebugMetrics(url *url.URL) int {
	buf, err := GetMetrics(url.String())
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
	}
	if err := ioutil.WriteFile("metrics.txt", buf, 0644); err != nil {
		fmt.Fprintln(os.Stderr, err)
	}
	fmt.Println(string(buf))
	return 0
}
