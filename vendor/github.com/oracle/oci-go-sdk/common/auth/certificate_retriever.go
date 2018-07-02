// Copyright (c) 2016, 2018, Oracle and/or its affiliates. All rights reserved.

package auth

import (
	"bytes"
	"crypto/rsa"
	"crypto/x509"
	"encoding/pem"
	"fmt"
	"github.com/oracle/oci-go-sdk/common"
	"sync"
)

// x509CertificateRetriever provides an X509 certificate with the RSA private key
type x509CertificateRetriever interface {
	Refresh() error
	CertificatePemRaw() []byte
	Certificate() *x509.Certificate
	PrivateKeyPemRaw() []byte
	PrivateKey() *rsa.PrivateKey
}

// urlBasedX509CertificateRetriever retrieves PEM-encoded X509 certificates from the given URLs.
type urlBasedX509CertificateRetriever struct {
	certURL           string
	privateKeyURL     string
	passphrase        string
	certificatePemRaw []byte
	certificate       *x509.Certificate
	privateKeyPemRaw  []byte
	privateKey        *rsa.PrivateKey
	mux               sync.Mutex
}

func newURLBasedX509CertificateRetriever(certURL, privateKeyURL, passphrase string) x509CertificateRetriever {
	return &urlBasedX509CertificateRetriever{
		certURL:       certURL,
		privateKeyURL: privateKeyURL,
		passphrase:    passphrase,
		mux:           sync.Mutex{},
	}
}

// Refresh() is failure atomic, i.e., CertificatePemRaw(), Certificate(), PrivateKeyPemRaw(), and PrivateKey() would
// return their previous values if Refresh() fails.
func (r *urlBasedX509CertificateRetriever) Refresh() error {
	common.Debugln("Refreshing certificate")

	r.mux.Lock()
	defer r.mux.Unlock()

	var err error

	var certificatePemRaw []byte
	var certificate *x509.Certificate
	if certificatePemRaw, certificate, err = r.renewCertificate(r.certURL); err != nil {
		return fmt.Errorf("failed to renew certificate: %s", err.Error())
	}

	var privateKeyPemRaw []byte
	var privateKey *rsa.PrivateKey
	if r.privateKeyURL != "" {
		if privateKeyPemRaw, privateKey, err = r.renewPrivateKey(r.privateKeyURL, r.passphrase); err != nil {
			return fmt.Errorf("failed to renew private key: %s", err.Error())
		}
	}

	r.certificatePemRaw = certificatePemRaw
	r.certificate = certificate
	r.privateKeyPemRaw = privateKeyPemRaw
	r.privateKey = privateKey
	return nil
}

func (r *urlBasedX509CertificateRetriever) renewCertificate(url string) (certificatePemRaw []byte, certificate *x509.Certificate, err error) {
	var body bytes.Buffer
	if body, err = httpGet(url); err != nil {
		return nil, nil, fmt.Errorf("failed to get certificate from %s: %s", url, err.Error())
	}

	certificatePemRaw = body.Bytes()
	var block *pem.Block
	block, _ = pem.Decode(certificatePemRaw)
	if block == nil {
		return nil, nil, fmt.Errorf("failed to parse the new certificate, not valid pem data")
	}

	if certificate, err = x509.ParseCertificate(block.Bytes); err != nil {
		return nil, nil, fmt.Errorf("failed to parse the new certificate: %s", err.Error())
	}

	return certificatePemRaw, certificate, nil
}

func (r *urlBasedX509CertificateRetriever) renewPrivateKey(url, passphrase string) (privateKeyPemRaw []byte, privateKey *rsa.PrivateKey, err error) {
	var body bytes.Buffer
	if body, err = httpGet(url); err != nil {
		return nil, nil, fmt.Errorf("failed to get private key from %s: %s", url, err.Error())
	}

	privateKeyPemRaw = body.Bytes()
	if privateKey, err = common.PrivateKeyFromBytes(privateKeyPemRaw, &passphrase); err != nil {
		return nil, nil, fmt.Errorf("failed to parse the new private key: %s", err.Error())
	}

	return privateKeyPemRaw, privateKey, nil
}

func (r *urlBasedX509CertificateRetriever) CertificatePemRaw() []byte {
	r.mux.Lock()
	defer r.mux.Unlock()

	if r.certificatePemRaw == nil {
		return nil
	}

	c := make([]byte, len(r.certificatePemRaw))
	copy(c, r.certificatePemRaw)
	return c
}

func (r *urlBasedX509CertificateRetriever) Certificate() *x509.Certificate {
	r.mux.Lock()
	defer r.mux.Unlock()

	if r.certificate == nil {
		return nil
	}

	c := *r.certificate
	return &c
}

func (r *urlBasedX509CertificateRetriever) PrivateKeyPemRaw() []byte {
	r.mux.Lock()
	defer r.mux.Unlock()

	if r.privateKeyPemRaw == nil {
		return nil
	}

	c := make([]byte, len(r.privateKeyPemRaw))
	copy(c, r.privateKeyPemRaw)
	return c
}

func (r *urlBasedX509CertificateRetriever) PrivateKey() *rsa.PrivateKey {
	r.mux.Lock()
	defer r.mux.Unlock()

	if r.privateKey == nil {
		return nil
	}

	c := *r.privateKey
	return &c
}
