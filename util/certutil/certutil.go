// Copyright The Prometheus Authors
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
// http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

// Package certutil provides utilities for loading and parsing X.509 certificates
// and private keys from various formats (PEM, PKCS12/PFX).
package certutil

import (
	"crypto/x509"
	"encoding/pem"
	"errors"
	"os"

	"golang.org/x/crypto/pkcs12"
)

// CertificateData represents parsed certificate data including certificates and private key.
type CertificateData struct {
	// Certificates is a slice of X.509 certificates (leaf certificate first, followed by any intermediates)
	Certificates []*x509.Certificate
	// PrivateKey is the private key associated with the leaf certificate
	PrivateKey any
}

// ParseCertificateFiles loads and parses certificate(s) and private key from files.
// It supports both PEM and PKCS12/PFX formats.
//
// Parameters:
//   - certPath: Path to the certificate file (required)
//   - keyPath: Path to a separate private key file (optional, only used for PEM format)
//   - password: Password for PKCS12/PFX files (optional, only used for PKCS12 format)
//
// Returns the parsed certificate data or an error.
func ParseCertificateFiles(certPath, keyPath, password string) (*CertificateData, error) {
	// Read certificate file
	certData, err := os.ReadFile(certPath)
	if err != nil {
		return nil, errors.New("failed to read certificate file " + certPath + ": " + err.Error())
	}

	// Detect format: check if PEM format first
	if isPEMFormat(certData) {
		return parsePEMData(certData, keyPath, certPath)
	}

	// Must be PKCS12/PFX format
	return parsePKCS12Data(certData, password, certPath)
}

// isPEMFormat checks if data is in PEM format.
func isPEMFormat(data []byte) bool {
	block, _ := pem.Decode(data)
	return block != nil
}

// parsePEMData parses PEM-encoded certificate and private key data.
func parsePEMData(certData []byte, keyPath, certPath string) (*CertificateData, error) {
	var certs []*x509.Certificate
	var privateKey any

	// Parse certificates and keys from certificate file
	rest := certData
	for {
		var block *pem.Block
		block, rest = pem.Decode(rest)
		if block == nil {
			break
		}

		switch block.Type {
		case "CERTIFICATE":
			cert, err := x509.ParseCertificate(block.Bytes)
			if err != nil {
				return nil, errors.New("failed to parse certificate from " + certPath + ": " + err.Error())
			}
			certs = append(certs, cert)
		case "PRIVATE KEY":
			if privateKey == nil { // Only take the first private key
				var err error
				privateKey, err = x509.ParsePKCS8PrivateKey(block.Bytes)
				if err != nil {
					return nil, errors.New("failed to parse PKCS8 private key from " + certPath + ": " + err.Error())
				}
			}
		case "RSA PRIVATE KEY":
			if privateKey == nil { // Only take the first private key
				var err error
				privateKey, err = x509.ParsePKCS1PrivateKey(block.Bytes)
				if err != nil {
					return nil, errors.New("failed to parse RSA private key from " + certPath + ": " + err.Error())
				}
			}
		case "EC PRIVATE KEY":
			if privateKey == nil { // Only take the first private key
				var err error
				privateKey, err = x509.ParseECPrivateKey(block.Bytes)
				if err != nil {
					return nil, errors.New("failed to parse EC private key from " + certPath + ": " + err.Error())
				}
			}
		}
	}

	// If no private key found in main file and separate key file is provided, read it
	if privateKey == nil && keyPath != "" {
		keyData, err := os.ReadFile(keyPath)
		if err != nil {
			return nil, errors.New("failed to read private key file " + keyPath + ": " + err.Error())
		}

		block, _ := pem.Decode(keyData)
		if block == nil {
			return nil, errors.New("failed to decode PEM private key from " + keyPath)
		}

		switch block.Type {
		case "PRIVATE KEY":
			privateKey, err = x509.ParsePKCS8PrivateKey(block.Bytes)
			if err != nil {
				return nil, errors.New("failed to parse PKCS8 private key from " + keyPath + ": " + err.Error())
			}
		case "RSA PRIVATE KEY":
			privateKey, err = x509.ParsePKCS1PrivateKey(block.Bytes)
			if err != nil {
				return nil, errors.New("failed to parse RSA private key from " + keyPath + ": " + err.Error())
			}
		case "EC PRIVATE KEY":
			privateKey, err = x509.ParseECPrivateKey(block.Bytes)
			if err != nil {
				return nil, errors.New("failed to parse EC private key from " + keyPath + ": " + err.Error())
			}
		default:
			return nil, errors.New("unsupported private key type in " + keyPath + ": " + block.Type)
		}
	}

	return &CertificateData{
		Certificates: certs,
		PrivateKey:   privateKey,
	}, nil
}

// parsePKCS12Data parses PKCS12/PFX-encoded certificate and private key data.
func parsePKCS12Data(certData []byte, password, certPath string) (*CertificateData, error) {
	// Convert PKCS12 to PEM blocks
	pemBlocks, err := pkcs12.ToPEM(certData, password)
	if err != nil {
		return nil, errors.New("failed to parse PFX certificate " + certPath + ": " + err.Error())
	}

	var certs []*x509.Certificate
	var privateKey any

	// Parse PEM blocks to extract certificates and private key
	for _, block := range pemBlocks {
		switch block.Type {
		case "CERTIFICATE":
			cert, err := x509.ParseCertificate(block.Bytes)
			if err != nil {
				return nil, errors.New("failed to parse certificate from PFX " + certPath + ": " + err.Error())
			}
			certs = append(certs, cert)
		case "PRIVATE KEY":
			// PKCS8 private key
			if privateKey == nil { // Only take the first private key
				privateKey, err = x509.ParsePKCS8PrivateKey(block.Bytes)
				if err != nil {
					return nil, errors.New("failed to parse PKCS8 private key from PFX " + certPath + ": " + err.Error())
				}
			}
		case "RSA PRIVATE KEY":
			// PKCS1 RSA private key
			if privateKey == nil { // Only take the first private key
				privateKey, err = x509.ParsePKCS1PrivateKey(block.Bytes)
				if err != nil {
					return nil, errors.New("failed to parse RSA private key from PFX " + certPath + ": " + err.Error())
				}
			}
		case "EC PRIVATE KEY":
			// EC private key
			if privateKey == nil { // Only take the first private key
				privateKey, err = x509.ParseECPrivateKey(block.Bytes)
				if err != nil {
					return nil, errors.New("failed to parse EC private key from PFX " + certPath + ": " + err.Error())
				}
			}
		}
	}

	return &CertificateData{
		Certificates: certs,
		PrivateKey:   privateKey,
	}, nil
}
