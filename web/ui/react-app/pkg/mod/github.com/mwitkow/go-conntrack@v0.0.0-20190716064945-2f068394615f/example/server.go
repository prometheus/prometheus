// Copyright 2016 Michal Witkowski. All Rights Reserved.
// See LICENSE for licensing terms.

package main

import (
	"crypto/tls"
	"flag"
	"fmt"
	"log"
	"net"
	"net/http"
	"time"

	"github.com/mwitkow/go-conntrack"
	"github.com/mwitkow/go-conntrack/connhelpers"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"golang.org/x/net/context/ctxhttp"
	_ "golang.org/x/net/trace"
)

var (
	port            = flag.Int("port", 9090, "whether to use tls or not")
	useTls          = flag.Bool("tls", true, "Whether to use TLS and HTTP2.")
	tlsCertFilePath = flag.String("tls_cert_file", "certs/localhost.crt", "Path to the CRT/PEM file.")
	tlsKeyFilePath  = flag.String("tls_key_file", "certs/localhost.key", "Path to the private key file.")
)

func main() {
	flag.Parse()

	// Make sure all outbound connections use the wrapped dialer.
	http.DefaultTransport.(*http.Transport).DialContext = conntrack.NewDialContextFunc(
		conntrack.DialWithTracing(),
		conntrack.DialWithDialer(&net.Dialer{
			Timeout:   30 * time.Second,
			KeepAlive: 30 * time.Second,
		}),
	)
	// Since we're using a dynamic name, let's preregister it with prometheus.
	conntrack.PreRegisterDialerMetrics("google")

	handler := func(resp http.ResponseWriter, req *http.Request) {
		resp.WriteHeader(http.StatusOK)
		resp.Header().Add("Content-Type", "application/json")
		resp.Write([]byte(`{"msg": "hello"}`))
		callCtx := conntrack.DialNameToContext(req.Context(), "google")
		_, err := ctxhttp.Get(callCtx, http.DefaultClient, "https://www.google.comx")
		log.Printf("Google reached with err: %v", err)
		log.Printf("Got request: %v", req)
	}

	http.DefaultServeMux.Handle("/", http.HandlerFunc(handler))
	http.DefaultServeMux.Handle("/metrics", promhttp.Handler())

	httpServer := http.Server{
		Handler: http.DefaultServeMux,
	}
	var httpListener net.Listener
	listener, err := net.Listen("tcp", fmt.Sprintf(":%d", *port))
	if err != nil {
		log.Fatalf("Failed to listen: %v", err)
	}
	listener = conntrack.NewListener(listener, conntrack.TrackWithTracing())
	if !*useTls {
		httpListener = listener
	} else {
		tlsConfig, err := connhelpers.TlsConfigForServerCerts(*tlsCertFilePath, *tlsKeyFilePath)
		if err != nil {
			log.Fatalf("Failed configuring TLS: %v", err)
		}
		tlsConfig, err = connhelpers.TlsConfigWithHttp2Enabled(tlsConfig)
		if err != nil {
			log.Fatalf("Failed configuring TLS: %v", err)
		}
		log.Printf("Listening with TLS")
		tlsListener := tls.NewListener(listener, tlsConfig)
		httpListener = tlsListener
	}
	//httpListener.Addr()
	log.Printf("Listening on: %s", listener.Addr().String())
	if err := httpServer.Serve(httpListener); err != nil {
		log.Fatalf("Failed listning: %v", err)
	}
}
