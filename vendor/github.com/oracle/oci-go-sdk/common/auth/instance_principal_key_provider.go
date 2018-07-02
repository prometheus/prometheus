// Copyright (c) 2016, 2018, Oracle and/or its affiliates. All rights reserved.

package auth

import (
	"bytes"
	"crypto/rsa"
	"fmt"
	"github.com/oracle/oci-go-sdk/common"
)

const (
	regionURL                            = `http://169.254.169.254/opc/v1/instance/region`
	leafCertificateURL                   = `http://169.254.169.254/opc/v1/identity/cert.pem`
	leafCertificateKeyURL                = `http://169.254.169.254/opc/v1/identity/key.pem`
	leafCertificateKeyPassphrase         = `` // No passphrase for the private key for Compute instances
	intermediateCertificateURL           = `http://169.254.169.254/opc/v1/identity/intermediate.pem`
	intermediateCertificateKeyURL        = ``
	intermediateCertificateKeyPassphrase = `` // No passphrase for the private key for Compute instances
)

// instancePrincipalKeyProvider implements KeyProvider to provide a key ID and its corresponding private key
// for an instance principal by getting a security token via x509FederationClient.
//
// The region name of the endpoint for x509FederationClient is obtained from the metadata service on the compute
// instance.
type instancePrincipalKeyProvider struct {
	regionForFederationClient common.Region
	federationClient          federationClient
	tenancyID                 string
}

// newInstancePrincipalKeyProvider creates and returns an instancePrincipalKeyProvider instance based on
// x509FederationClient.
//
// NOTE: There is a race condition between PrivateRSAKey() and KeyID().  These two pieces are tightly coupled; KeyID
// includes a security token obtained from Auth service by giving a public key which is paired with PrivateRSAKey.
// The x509FederationClient caches the security token in memory until it is expired.  Thus, even if a client obtains a
// KeyID that is not expired at the moment, the PrivateRSAKey that the client acquires at a next moment could be
// invalid because the KeyID could be already expired.
func newInstancePrincipalKeyProvider() (provider *instancePrincipalKeyProvider, err error) {
	var region common.Region
	if region, err = getRegionForFederationClient(regionURL); err != nil {
		err = fmt.Errorf("failed to get the region name from %s: %s", regionURL, err.Error())
		common.Logln(err)
		return nil, err
	}

	leafCertificateRetriever := newURLBasedX509CertificateRetriever(
		leafCertificateURL, leafCertificateKeyURL, leafCertificateKeyPassphrase)
	intermediateCertificateRetrievers := []x509CertificateRetriever{
		newURLBasedX509CertificateRetriever(
			intermediateCertificateURL, intermediateCertificateKeyURL, intermediateCertificateKeyPassphrase),
	}

	if err = leafCertificateRetriever.Refresh(); err != nil {
		err = fmt.Errorf("failed to refresh the leaf certificate: %s", err.Error())
		return nil, err
	}
	tenancyID := extractTenancyIDFromCertificate(leafCertificateRetriever.Certificate())

	federationClient := newX509FederationClient(
		region, tenancyID, leafCertificateRetriever, intermediateCertificateRetrievers)

	provider = &instancePrincipalKeyProvider{regionForFederationClient: region, federationClient: federationClient, tenancyID: tenancyID}
	return
}

func getRegionForFederationClient(url string) (r common.Region, err error) {
	var body bytes.Buffer
	if body, err = httpGet(url); err != nil {
		return
	}
	return common.StringToRegion(body.String()), nil
}

func (p *instancePrincipalKeyProvider) RegionForFederationClient() common.Region {
	return p.regionForFederationClient
}

func (p *instancePrincipalKeyProvider) PrivateRSAKey() (privateKey *rsa.PrivateKey, err error) {
	if privateKey, err = p.federationClient.PrivateKey(); err != nil {
		err = fmt.Errorf("failed to get private key: %s", err.Error())
		return nil, err
	}
	return privateKey, nil
}

func (p *instancePrincipalKeyProvider) KeyID() (string, error) {
	var securityToken string
	var err error
	if securityToken, err = p.federationClient.SecurityToken(); err != nil {
		return "", fmt.Errorf("failed to get security token: %s", err.Error())
	}
	return fmt.Sprintf("ST$%s", securityToken), nil
}

func (p *instancePrincipalKeyProvider) TenancyOCID() (string, error) {
	return p.tenancyID, nil
}
