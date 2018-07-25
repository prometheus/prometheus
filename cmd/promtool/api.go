package main

import (
	"fmt"

	"github.com/prometheus/client_golang/api"
	"github.com/prometheus/client_golang/api/prometheus/v1"
)

type prometheusAPI struct {
	v1.API
}

func newPrometheusAPI(serverURL string) (*prometheusAPI, error) {
	c, err := api.NewClient(api.Config{Address: serverURL})
	if err != nil {
		return nil, fmt.Errorf("error creating HTTP client: %s", err)
	}
	api := v1.NewAPI(c)
	return &prometheusAPI{
		api,
	}, nil
}
