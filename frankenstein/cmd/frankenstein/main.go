package main

import (
	"flag"

	"github.com/prometheus/common/log"
	"github.com/prometheus/prometheus/frankenstein"
)

func main() {
	var (
		listen       string
		consulHost   string
		consulPrefix string
	)

	flag.StringVar(&listen, "web.listen-address", ":9094", "HTTP server listen address.")
	flag.StringVar(&consulHost, "consul.hostname", "consul:8500", "Hostname and port of Consul.")
	flag.StringVar(&consulPrefix, "consul.prefix", "collectors/", "Prefix for keys in Consul.")

	_, err := frankenstein.NewDistributor(frankenstein.DistributorConfig{
		ConsulHost:   consulHost,
		ConsulPrefix: consulPrefix,
	})
	if err != nil {
		log.Fatal(err)
	}
}
