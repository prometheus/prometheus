// Copyright (c) 2016, 2018, Oracle and/or its affiliates. All rights reserved.

package auth

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"fmt"
	"github.com/stretchr/testify/assert"
	"math/big"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestUrlBasedX509CertificateRetriever_BadCertificate(t *testing.T) {
	expectedCert := make([]byte, 100)
	rand.Read(expectedCert)
	certServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, string(expectedCert))
	}))
	defer certServer.Close()

	retriever := newURLBasedX509CertificateRetriever(certServer.URL, "", "")
	err := retriever.Refresh()

	assert.Error(t, err)
}
func TestUrlBasedX509CertificateRetriever_RefreshWithoutPrivateKeyUrl(t *testing.T) {
	_, expectedCert := generateRandomCertificate()
	certServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, string(expectedCert))
	}))
	defer certServer.Close()

	retriever := newURLBasedX509CertificateRetriever(certServer.URL, "", "")
	err := retriever.Refresh()

	assert.NoError(t, err)

	assert.Equal(t, expectedCert, retriever.CertificatePemRaw())
	actualCert := retriever.Certificate()
	actualCertPem := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: actualCert.Raw})
	assert.Equal(t, expectedCert, actualCertPem)

	assert.Nil(t, retriever.PrivateKeyPemRaw())
	assert.Nil(t, retriever.PrivateKey())
}

func TestUrlBasedX509CertificateRetriever_RefreshWithPrivateKeyUrl(t *testing.T) {
	expectedPrivateKey, expectedCert := generateRandomCertificate()
	certServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, string(expectedCert))
	}))
	defer certServer.Close()
	privateKeyServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, string(expectedPrivateKey))
	}))
	defer privateKeyServer.Close()

	retriever := newURLBasedX509CertificateRetriever(certServer.URL, privateKeyServer.URL, "")
	err := retriever.Refresh()

	assert.NoError(t, err)

	assert.Equal(t, expectedCert, retriever.CertificatePemRaw())
	actualCert := retriever.Certificate()
	actualCertPem := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: actualCert.Raw})
	assert.Equal(t, expectedCert, actualCertPem)

	assert.Equal(t, expectedPrivateKey, retriever.PrivateKeyPemRaw())
	actualPrivateKey := retriever.PrivateKey()
	actualPrivateKeyPem := pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(actualPrivateKey)})
	assert.Equal(t, expectedPrivateKey, actualPrivateKeyPem)
}

func TestUrlBasedX509CertificateRetriever_RefreshCertNotFound(t *testing.T) {
	certServer := httptest.NewServer(http.NotFoundHandler())
	defer certServer.Close()

	retriever := newURLBasedX509CertificateRetriever(certServer.URL, "", "")
	err := retriever.Refresh()

	assert.Error(t, err)
	assert.Nil(t, retriever.CertificatePemRaw())
	assert.Nil(t, retriever.Certificate())
	assert.Nil(t, retriever.PrivateKeyPemRaw())
	assert.Nil(t, retriever.PrivateKey())
}

func TestUrlBasedX509CertificateRetriever_RefreshPrivateKeyNotFound(t *testing.T) {
	_, expectedCert := generateRandomCertificate()
	certServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, string(expectedCert))
	}))
	defer certServer.Close()
	privateKeyServer := httptest.NewServer(http.NotFoundHandler())
	defer privateKeyServer.Close()

	retriever := newURLBasedX509CertificateRetriever(certServer.URL, privateKeyServer.URL, "")
	err := retriever.Refresh()

	assert.Error(t, err)
	assert.Nil(t, retriever.CertificatePemRaw())
	assert.Nil(t, retriever.Certificate())
	assert.Nil(t, retriever.PrivateKeyPemRaw())
	assert.Nil(t, retriever.PrivateKey())
}

func internalServerError(w http.ResponseWriter, r *http.Request) {
	http.Error(w, "500 internal server error", http.StatusInternalServerError)
}

func TestUrlBasedX509CertificateRetriever_RefreshCertInternalServerError(t *testing.T) {
	certServer := httptest.NewServer(http.HandlerFunc(internalServerError))
	defer certServer.Close()

	retriever := newURLBasedX509CertificateRetriever(certServer.URL, "", "")
	err := retriever.Refresh()

	assert.Error(t, err)
	assert.Nil(t, retriever.CertificatePemRaw())
	assert.Nil(t, retriever.Certificate())
	assert.Nil(t, retriever.PrivateKeyPemRaw())
	assert.Nil(t, retriever.PrivateKey())
}

func TestUrlBasedX509CertificateRetriever_RefreshPrivateKeyInternalServerError(t *testing.T) {
	_, expectedCert := generateRandomCertificate()
	certServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, string(expectedCert))
	}))
	defer certServer.Close()
	privateKeyServer := httptest.NewServer(http.HandlerFunc(internalServerError))
	defer privateKeyServer.Close()

	retriever := newURLBasedX509CertificateRetriever(certServer.URL, privateKeyServer.URL, "")
	err := retriever.Refresh()

	assert.Error(t, err)
	assert.Nil(t, retriever.CertificatePemRaw())
	assert.Nil(t, retriever.Certificate())
	assert.Nil(t, retriever.PrivateKeyPemRaw())
	assert.Nil(t, retriever.PrivateKey())
}

func TestUrlBasedX509CertificateRetriever_FailureAtomicity(t *testing.T) {
	privateKeyServerFailed := false

	expectedPrivateKey, expectedCert := generateRandomCertificate()
	_, anotherCert := generateRandomCertificate()

	certServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if privateKeyServerFailed {
			fmt.Fprint(w, string(anotherCert))

		} else {
			fmt.Fprint(w, string(expectedCert))
		}
	}))
	defer certServer.Close()

	privateKeyServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if privateKeyServerFailed {
			internalServerError(w, r)
		} else {
			fmt.Fprint(w, string(expectedPrivateKey))
		}
	}))
	defer privateKeyServer.Close()

	retriever := newURLBasedX509CertificateRetriever(certServer.URL, privateKeyServer.URL, "")
	err := retriever.Refresh()

	assert.NoError(t, err)

	privateKeyServerFailed = true

	err = retriever.Refresh()

	assert.Error(t, err)
	assert.Equal(t, expectedCert, retriever.CertificatePemRaw()) // Not anotherCert but expectedCert
	actualCert := retriever.Certificate()
	actualCertPem := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: actualCert.Raw})
	assert.Equal(t, expectedCert, actualCertPem)

	assert.Equal(t, expectedPrivateKey, retriever.PrivateKeyPemRaw())
	actualPrivateKey := retriever.PrivateKey()
	actualPrivateKeyPem := pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(actualPrivateKey)})
	assert.Equal(t, expectedPrivateKey, actualPrivateKeyPem)
}

func generateRandomCertificate() (privateKeyPem, certPem []byte) {
	serialNumberLimit := new(big.Int).Lsh(big.NewInt(1), 128)
	serialNumber, _ := rand.Int(rand.Reader, serialNumberLimit)
	notBefore := time.Now()
	notAfter := notBefore.Add(365 * 24 * time.Hour)

	template := x509.Certificate{
		SerialNumber: serialNumber,
		Issuer: pkix.Name{
			CommonName: "PKISVC Identity Intermediate r2",
		},
		Subject: pkix.Name{
			CommonName: "ocid1.instance.oc1.phx.bluhbluhbluh",
		},
		NotBefore:          notBefore,
		NotAfter:           notAfter,
		PublicKeyAlgorithm: x509.RSA,
		SignatureAlgorithm: x509.SHA256WithRSA,
	}

	privateKey, _ := rsa.GenerateKey(rand.Reader, 2048)
	newCertBytes, _ := x509.CreateCertificate(rand.Reader, &template, &template, privateKey.Public(), privateKey)

	privateKeyPem = pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(privateKey)})
	certPem = pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: newCertBytes})
	return
}
