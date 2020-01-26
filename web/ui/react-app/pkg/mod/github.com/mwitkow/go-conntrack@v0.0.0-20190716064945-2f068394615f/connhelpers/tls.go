// Copyright 2016 Michal Witkowski. All Rights Reserved.
// See LICENSE for licensing terms.

package connhelpers

import (
	"crypto/tls"
	"fmt"
)

// TlsConfigForServerCerts is a returns a simple `tls.Config` with the given server cert loaded.
// This is useful if you can't use `http.ListenAndServerTLS` when using a custom `net.Listener`.
func TlsConfigForServerCerts(certFile string, keyFile string) (*tls.Config, error) {
	var err error
	config := new(tls.Config)
	config.Certificates = make([]tls.Certificate, 1)
	config.Certificates[0], err = tls.LoadX509KeyPair(certFile, keyFile)
	if err != nil {
		return nil, err
	}
	return config, nil
}

// TlsConfigWithHttp2Enabled makes it easy to configure the given `tls.Config` to prefer H2 connections.
// This is useful if you can't use `http.ListenAndServerTLS` when using a custom `net.Listener`.
func TlsConfigWithHttp2Enabled(config *tls.Config) (*tls.Config, error) {
	// mostly based on http2 code in the standards library.
	if config.CipherSuites != nil {
		// If they already provided a CipherSuite list, return
		// an error if it has a bad order or is missing
		// ECDHE_RSA_WITH_AES_128_GCM_SHA256.
		const requiredCipher = tls.TLS_ECDHE_RSA_WITH_AES_128_GCM_SHA256
		haveRequired := false
		for _, cs := range config.CipherSuites {
			if cs == requiredCipher {
				haveRequired = true
			}
		}
		if !haveRequired {
			return nil, fmt.Errorf("http2: TLSConfig.CipherSuites is missing HTTP/2-required TLS_ECDHE_RSA_WITH_AES_128_GCM_SHA256")
		}
	}

	config.PreferServerCipherSuites = true

	haveNPN := false
	for _, p := range config.NextProtos {
		if p == "h2" {
			haveNPN = true
			break
		}
	}
	if !haveNPN {
		config.NextProtos = append(config.NextProtos, "h2")
	}
	config.NextProtos = append(config.NextProtos, "h2-14")
	// make sure http 1.1 is *after* all of the other ones.
	config.NextProtos = append(config.NextProtos, "http/1.1")
	return config, nil
}
