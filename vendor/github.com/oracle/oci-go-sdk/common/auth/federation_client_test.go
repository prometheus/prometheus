// Copyright (c) 2016, 2018, Oracle and/or its affiliates. All rights reserved.

package auth

import (
	"bytes"
	"crypto/rsa"
	"crypto/x509"
	"encoding/pem"
	"fmt"
	"github.com/oracle/oci-go-sdk/common"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestX509FederationClient_VeryFirstSecurityToken(t *testing.T) {
	authServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Verify request
		expectedKeyID := fmt.Sprintf("%s/fed-x509/%s", tenancyID, leafCertFingerprint)
		assert.True(t, strings.HasPrefix(r.Header.Get("Authorization"), fmt.Sprintf(`Signature version="1",headers="date (request-target) content-length content-type x-content-sha256",keyId="%s",algorithm="rsa-sha256",signature=`, expectedKeyID)))
		expectedBody := fmt.Sprintf(`{"certificate":"%s","intermediateCertificates":["%s"],"publicKey":"%s"}`,
			leafCertBodyNoNewLine, intermediateCertBodyNoNewLine, sessionPublicKeyBodyNoNewLine)

		var buf bytes.Buffer
		buf.ReadFrom(r.Body)
		assert.Equal(t, expectedBody, buf.String())

		// Return response
		fmt.Fprintf(w, "\n{\n  \"token\" : \"%s\"\n}\n", expectedSecurityToken)
	}))
	defer authServer.Close()

	mockSessionKeySupplier := new(mockSessionKeySupplier)
	mockSessionKeySupplier.On("Refresh").Return(nil).Once()
	mockSessionKeySupplier.On("PublicKeyPemRaw").Return([]byte(sessionPublicKeyPem))

	mockLeafCertificateRetriever := new(mockCertificateRetriever)
	mockLeafCertificateRetriever.On("Refresh").Return(nil).Once()
	mockLeafCertificateRetriever.On("CertificatePemRaw").Return([]byte(leafCertPem))
	mockLeafCertificateRetriever.On("Certificate").Return(parseCertificate(leafCertPem))
	mockLeafCertificateRetriever.On("PrivateKey").Return(parsePrivateKey(leafCertPrivateKeyPem))

	mockIntermediateCertificateRetriever := new(mockCertificateRetriever)
	mockIntermediateCertificateRetriever.On("Refresh").Return(nil).Once()
	mockIntermediateCertificateRetriever.On("CertificatePemRaw").Return([]byte(intermediateCertPem))

	federationClient := &x509FederationClient{
		tenancyID:                         tenancyID,
		sessionKeySupplier:                mockSessionKeySupplier,
		leafCertificateRetriever:          mockLeafCertificateRetriever,
		intermediateCertificateRetrievers: []x509CertificateRetriever{mockIntermediateCertificateRetriever},
	}
	federationClient.authClient = newAuthClient(whateverRegion, federationClient)
	// Overwrite with the authServer's URL
	federationClient.authClient.Host = authServer.URL
	federationClient.authClient.BasePath = ""

	actualSecurityToken, err := federationClient.SecurityToken()

	assert.NoError(t, err)
	assert.Equal(t, expectedSecurityToken, actualSecurityToken)
	mockSessionKeySupplier.AssertExpectations(t)
	mockLeafCertificateRetriever.AssertExpectations(t)
	mockIntermediateCertificateRetriever.AssertExpectations(t)
}

func TestX509FederationClient_RenewSecurityToken(t *testing.T) {
	authServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Verify request
		expectedKeyID := fmt.Sprintf("%s/fed-x509/%s", tenancyID, leafCertFingerprint)
		assert.True(t, strings.HasPrefix(r.Header.Get("Authorization"), fmt.Sprintf(`Signature version="1",headers="date (request-target) content-length content-type x-content-sha256",keyId="%s",algorithm="rsa-sha256",signature=`, expectedKeyID)))

		expectedBody := fmt.Sprintf(`{"certificate":"%s","intermediateCertificates":["%s"],"publicKey":"%s"}`,
			leafCertBodyNoNewLine, intermediateCertBodyNoNewLine, sessionPublicKeyBodyNoNewLine)
		var buf bytes.Buffer
		buf.ReadFrom(r.Body)
		assert.Equal(t, expectedBody, buf.String())

		// Return response
		fmt.Fprintf(w, "\n{\n  \"token\" : \"%s\"\n}\n", expectedSecurityToken)
	}))
	defer authServer.Close()

	mockSessionKeySupplier := new(mockSessionKeySupplier)
	mockSessionKeySupplier.On("Refresh").Return(nil).Once()
	mockSessionKeySupplier.On("PublicKeyPemRaw").Return([]byte(sessionPublicKeyPem))

	mockLeafCertificateRetriever := new(mockCertificateRetriever)
	mockLeafCertificateRetriever.On("Refresh").Return(nil).Once()
	mockLeafCertificateRetriever.On("CertificatePemRaw").Return([]byte(leafCertPem))
	mockLeafCertificateRetriever.On("Certificate").Return(parseCertificate(leafCertPem))
	mockLeafCertificateRetriever.On("PrivateKey").Return(parsePrivateKey(leafCertPrivateKeyPem))

	mockIntermediateCertificateRetriever := new(mockCertificateRetriever)
	mockIntermediateCertificateRetriever.On("Refresh").Return(nil).Once()
	mockIntermediateCertificateRetriever.On("CertificatePemRaw").Return([]byte(intermediateCertPem))

	mockSecurityToken := new(mockSecurityToken)
	mockSecurityToken.On("Valid").Return(false)

	federationClient := &x509FederationClient{
		tenancyID:                         tenancyID,
		sessionKeySupplier:                mockSessionKeySupplier,
		leafCertificateRetriever:          mockLeafCertificateRetriever,
		intermediateCertificateRetrievers: []x509CertificateRetriever{mockIntermediateCertificateRetriever},
	}
	federationClient.authClient = newAuthClient(whateverRegion, federationClient)
	// Overwrite with the authServer's URL
	federationClient.authClient.Host = authServer.URL
	federationClient.authClient.BasePath = ""
	federationClient.securityToken = mockSecurityToken

	actualSecurityToken, err := federationClient.SecurityToken()

	assert.NoError(t, err)
	assert.Equal(t, expectedSecurityToken, actualSecurityToken)
	mockSessionKeySupplier.AssertExpectations(t)
	mockLeafCertificateRetriever.AssertExpectations(t)
	mockIntermediateCertificateRetriever.AssertExpectations(t)
}

func TestX509FederationClient_GetCachedSecurityToken(t *testing.T) {
	mockSessionKeySupplier := new(mockSessionKeySupplier)
	mockLeafCertificateRetriever := new(mockCertificateRetriever)
	mockIntermediateCertificateRetriever := new(mockCertificateRetriever)

	mockSecurityToken := new(mockSecurityToken)
	mockSecurityToken.On("Valid").Return(true)
	mockSecurityToken.On("String").Return(expectedSecurityToken)

	federationClient := &x509FederationClient{
		tenancyID:                         tenancyID,
		sessionKeySupplier:                mockSessionKeySupplier,
		leafCertificateRetriever:          mockLeafCertificateRetriever,
		intermediateCertificateRetrievers: []x509CertificateRetriever{mockIntermediateCertificateRetriever},
	}
	federationClient.authClient = newAuthClient(whateverRegion, federationClient)
	federationClient.securityToken = mockSecurityToken

	actualSecurityToken, err := federationClient.SecurityToken()

	assert.NoError(t, err)
	assert.Equal(t, expectedSecurityToken, actualSecurityToken)

	mockSessionKeySupplier.AssertNotCalled(t, "Refresh")
	mockSessionKeySupplier.AssertNotCalled(t, "PublicKeyPemRaw")
	mockLeafCertificateRetriever.AssertNotCalled(t, "Refresh")
	mockLeafCertificateRetriever.AssertNotCalled(t, "CertificatePemRaw")
	mockLeafCertificateRetriever.AssertNotCalled(t, "Certificate")
	mockLeafCertificateRetriever.AssertNotCalled(t, "PrivateKey")
	mockIntermediateCertificateRetriever.AssertNotCalled(t, "Refresh")
	mockIntermediateCertificateRetriever.AssertNotCalled(t, "CertificatePemRaw")
}

func TestX509FederationClient_RenewSecurityTokenSessionKeySupplierError(t *testing.T) {
	mockSessionKeySupplier := new(mockSessionKeySupplier)
	expectedErrorMessage := "TestSessionKeySupplierRefreshError"
	mockSessionKeySupplier.On("Refresh").Return(fmt.Errorf(expectedErrorMessage)).Once()

	mockLeafCertificateRetriever := new(mockCertificateRetriever)
	mockIntermediateCertificateRetriever := new(mockCertificateRetriever)

	mockSecurityToken := new(mockSecurityToken)
	mockSecurityToken.On("Valid").Return(false)

	federationClient := &x509FederationClient{
		tenancyID:                         tenancyID,
		sessionKeySupplier:                mockSessionKeySupplier,
		leafCertificateRetriever:          mockLeafCertificateRetriever,
		intermediateCertificateRetrievers: []x509CertificateRetriever{mockIntermediateCertificateRetriever},
	}
	federationClient.authClient = newAuthClient(whateverRegion, federationClient)
	federationClient.securityToken = mockSecurityToken

	actualSecurityToken, actualError := federationClient.SecurityToken()

	assert.Empty(t, actualSecurityToken)
	assert.EqualError(t, actualError, fmt.Sprintf("failed to renew security token: failed to refresh session key: %s", expectedErrorMessage))
}

func TestX509FederationClient_RenewSecurityTokenLeafCertificateRetrieverError(t *testing.T) {
	mockSessionKeySupplier := new(mockSessionKeySupplier)
	mockSessionKeySupplier.On("Refresh").Return(nil).Once()

	mockLeafCertificateRetriever := new(mockCertificateRetriever)
	expectedErrorMessage := "TestLeafCertificateRetrieverError"
	mockLeafCertificateRetriever.On("Refresh").Return(fmt.Errorf(expectedErrorMessage)).Once()

	mockIntermediateCertificateRetriever := new(mockCertificateRetriever)

	mockSecurityToken := new(mockSecurityToken)
	mockSecurityToken.On("Valid").Return(false)

	federationClient := &x509FederationClient{
		tenancyID:                         tenancyID,
		sessionKeySupplier:                mockSessionKeySupplier,
		leafCertificateRetriever:          mockLeafCertificateRetriever,
		intermediateCertificateRetrievers: []x509CertificateRetriever{mockIntermediateCertificateRetriever},
	}
	federationClient.authClient = newAuthClient(whateverRegion, federationClient)
	federationClient.securityToken = mockSecurityToken

	actualSecurityToken, actualError := federationClient.SecurityToken()

	assert.Empty(t, actualSecurityToken)
	assert.EqualError(t, actualError, fmt.Sprintf("failed to renew security token: failed to refresh leaf certificate: %s", expectedErrorMessage))
}

func TestX509FederationClient_RenewSecurityTokenIntermediateCertificateRetrieverError(t *testing.T) {
	mockSessionKeySupplier := new(mockSessionKeySupplier)
	mockSessionKeySupplier.On("Refresh").Return(nil).Once()

	mockLeafCertificateRetriever := new(mockCertificateRetriever)
	mockLeafCertificateRetriever.On("Refresh").Return(nil).Once()
	mockLeafCertificateRetriever.On("Certificate").Return(parseCertificate(leafCertPem))

	mockIntermediateCertificateRetriever := new(mockCertificateRetriever)
	expectedErrorMessage := "TestLeafCertificateRetrieverError"
	mockIntermediateCertificateRetriever.On("Refresh").Return(fmt.Errorf(expectedErrorMessage)).Once()

	mockSecurityToken := new(mockSecurityToken)
	mockSecurityToken.On("Valid").Return(false)

	federationClient := &x509FederationClient{
		tenancyID:                         tenancyID,
		sessionKeySupplier:                mockSessionKeySupplier,
		leafCertificateRetriever:          mockLeafCertificateRetriever,
		intermediateCertificateRetrievers: []x509CertificateRetriever{mockIntermediateCertificateRetriever},
	}
	federationClient.authClient = newAuthClient(whateverRegion, federationClient)
	federationClient.securityToken = mockSecurityToken

	actualSecurityToken, actualError := federationClient.SecurityToken()

	assert.Empty(t, actualSecurityToken)
	assert.EqualError(t, actualError, fmt.Sprintf("failed to renew security token: failed to refresh intermediate certificate: %s", expectedErrorMessage))
}

func TestX509FederationClient_RenewSecurityTokenUnexpectedTenancyIdUpdateError(t *testing.T) {
	mockSessionKeySupplier := new(mockSessionKeySupplier)
	mockSessionKeySupplier.On("Refresh").Return(nil).Once()

	mockLeafCertificateRetriever := new(mockCertificateRetriever)
	mockLeafCertificateRetriever.On("Refresh").Return(nil).Once()
	mockLeafCertificateRetriever.On("Certificate").Return(parseCertificate(leafCertPem))

	mockIntermediateCertificateRetriever := new(mockCertificateRetriever)

	mockSecurityToken := new(mockSecurityToken)
	mockSecurityToken.On("Valid").Return(false)

	previousTenancyID := "ocidv1:tenancy:oc1:phx:1234567890:foobarfoobar"

	federationClient := &x509FederationClient{
		tenancyID:                         previousTenancyID,
		sessionKeySupplier:                mockSessionKeySupplier,
		leafCertificateRetriever:          mockLeafCertificateRetriever,
		intermediateCertificateRetrievers: []x509CertificateRetriever{mockIntermediateCertificateRetriever},
	}
	federationClient.authClient = newAuthClient(whateverRegion, federationClient)
	federationClient.securityToken = mockSecurityToken

	actualSecurityToken, actualError := federationClient.SecurityToken()

	assert.Empty(t, actualSecurityToken)
	assert.EqualError(t, actualError, fmt.Sprintf("failed to renew security token: unexpected update of tenancy OCID in the leaf certificate. Previous tenancy: %s, Updated: %s", previousTenancyID, tenancyID))
}

func TestX509FederationClient_AuthServerInternalError(t *testing.T) {
	authServer := httptest.NewServer(http.HandlerFunc(internalServerError))
	defer authServer.Close()

	mockSessionKeySupplier := new(mockSessionKeySupplier)
	mockSessionKeySupplier.On("Refresh").Return(nil).Once()
	mockSessionKeySupplier.On("PublicKeyPemRaw").Return([]byte(sessionPublicKeyPem))

	mockLeafCertificateRetriever := new(mockCertificateRetriever)
	mockLeafCertificateRetriever.On("Refresh").Return(nil).Once()
	mockLeafCertificateRetriever.On("CertificatePemRaw").Return([]byte(leafCertPem))
	mockLeafCertificateRetriever.On("Certificate").Return(parseCertificate(leafCertPem))
	mockLeafCertificateRetriever.On("PrivateKey").Return(parsePrivateKey(leafCertPrivateKeyPem))

	mockIntermediateCertificateRetriever := new(mockCertificateRetriever)
	mockIntermediateCertificateRetriever.On("Refresh").Return(nil).Once()
	mockIntermediateCertificateRetriever.On("CertificatePemRaw").Return([]byte(intermediateCertPem))

	federationClient := &x509FederationClient{
		tenancyID:                         tenancyID,
		sessionKeySupplier:                mockSessionKeySupplier,
		leafCertificateRetriever:          mockLeafCertificateRetriever,
		intermediateCertificateRetrievers: []x509CertificateRetriever{mockIntermediateCertificateRetriever},
	}
	federationClient.authClient = newAuthClient(whateverRegion, federationClient)
	// Overwrite with the authServer's URL
	federationClient.authClient.Host = authServer.URL
	federationClient.authClient.BasePath = ""

	_, err := federationClient.SecurityToken()

	assert.Error(t, err)
}

func parseCertificate(certPem string) *x509.Certificate {
	var block *pem.Block
	block, _ = pem.Decode([]byte(certPem))
	cert, _ := x509.ParseCertificate(block.Bytes)
	return cert
}

func parsePrivateKey(privateKeyPem string) *rsa.PrivateKey {
	block, _ := pem.Decode([]byte(privateKeyPem))
	key, _ := x509.ParsePKCS1PrivateKey(block.Bytes)
	return key
}

const (
	leafCertBody = `MIIEfzCCA2egAwIBAgIRAJNzEqD3n5To66H1rEDhKyMwDQYJKoZIhvcNAQELBQAw
gfgxgccwHAYDVQQLExVvcGMtY2VydHR5cGU6aW5zdGFuY2UwKgYDVQQLEyNvcGMt
aW5zdGFuY2Uub2NpZDEucGh4LmJsdWhibHVoYmx1aDA5BgNVBAsTMm9wYy1jb21w
YXJ0bWVudDpvY2lkMS5jb21wYXJ0bWVudC5vYzEuYmx1aGJsdWhibHVoMEAGA1UE
CxM5b3BjLXRlbmFudDpvY2lkdjE6dGVuYW5jeTpvYzE6cGh4OjEyMzQ1Njc4OTA6
Ymx1aGJsdWhibHVoMSwwKgYDVQQDEyNvY2lkMS5pbnN0YW5jZS5vYzEucGh4LmJs
dWhibHVoYmx1aDAeFw0xNzExMzAwMDA4MzRaFw0xODExMzAwMDA4MzRaMIH4MYHH
MBwGA1UECxMVb3BjLWNlcnR0eXBlOmluc3RhbmNlMCoGA1UECxMjb3BjLWluc3Rh
bmNlLm9jaWQxLnBoeC5ibHVoYmx1aGJsdWgwOQYDVQQLEzJvcGMtY29tcGFydG1l
bnQ6b2NpZDEuY29tcGFydG1lbnQub2MxLmJsdWhibHVoYmx1aDBABgNVBAsTOW9w
Yy10ZW5hbnQ6b2NpZHYxOnRlbmFuY3k6b2MxOnBoeDoxMjM0NTY3ODkwOmJsdWhi
bHVoYmx1aDEsMCoGA1UEAxMjb2NpZDEuaW5zdGFuY2Uub2MxLnBoeC5ibHVoYmx1
aGJsdWgwggEiMA0GCSqGSIb3DQEBAQUAA4IBDwAwggEKAoIBAQDOEGPhWFLIU/1w
gncMPddP01Mo80GEZ1h2A+QGC/1ha0gxp/yVNIi2rNwwjLiqtAGaMbGnVicaPpOm
uNiHWf92E2jwEcy2Q0kA+FwHINDjmzrjMgQFTI4MDIA0bLdZY9KX5CFqw3pwqkyC
KfsZRjUiOmlELbnCb7NQIyd5t3zZfgFv6EOFW4qPdTHrr5Gql1knpUNjUeYR4ZxI
BB6NIxBV3Dm3PRDClP/2/Q5zwsZepRYmWyBe6WsfIvB86x5xH19CGe4HGYrNr1YF
+q9+iDb8PuLfr+gxW7PJPo/TxBgdko0h981nPYzbju9eVa2KKjK6fmzjSvNkgJWn
11ngT5mBAgMBAAGjAjAAMA0GCSqGSIb3DQEBCwUAA4IBAQANjXNFgZ+q/OGZ4bBY
uyFUU/yWz+BtkTzukZa7HT78w4qVE+Gjunc9ad5D2Afr+qTl4sazTTVWTI+KyRJW
5aHWwTUSXSwYrUOQbV0rktLbS4vJggVnU50Y8pdaXq1q+3B4t1DIJwrN2sWu3krA
bQKgOQzEUwm2OZJf0lPA9bp+HVv0iA17wUazgvHVPt/93Yr8e1VQT43ygG990KIT
gipcAk/44ONyBS8W8QRhzpuErEHCcSR+3+KBtwVZgzaVR6+mb5KFYFpXBrgs8YOe
P2X4jUFjRbpUMbNi9Q6V8mAzejPc8Hqk3116Fpl62PGiW802n4CMtDXYxawJSRxV
WO1p`
	leafCertPrivateKeyBody = `MIIEowIBAAKCAQEAzhBj4VhSyFP9cIJ3DD3XT9NTKPNBhGdYdgPkBgv9YWtIMaf8
lTSItqzcMIy4qrQBmjGxp1YnGj6TprjYh1n/dhNo8BHMtkNJAPhcByDQ45s64zIE
BUyODAyANGy3WWPSl+QhasN6cKpMgin7GUY1IjppRC25wm+zUCMnebd82X4Bb+hD
hVuKj3Ux66+RqpdZJ6VDY1HmEeGcSAQejSMQVdw5tz0QwpT/9v0Oc8LGXqUWJlsg
XulrHyLwfOsecR9fQhnuBxmKza9WBfqvfog2/D7i36/oMVuzyT6P08QYHZKNIffN
Zz2M247vXlWtiioyun5s40rzZICVp9dZ4E+ZgQIDAQABAoIBAQCdvLAoVIrx7FEp
6cSla0VBRrv2sdbqOo3dsPbApjbsdsoJsNTJhjBM3Z+jzmShzy8W0Il0VZ+TGGnA
Cuk9GuhRg2QluQpiTrk4c+VGU5lzUWVPev7W65YkpQESoFHtrFsNiEUIS+CTE9mD
Hg2neDW+IMZpuTLkIss5Qd+67Xk1pj19CtHfWVbeqc7B9DPAZa4CBCVsWFPhaiyv
tsz/PJKFDDXllgrQaAyCK1gyrKR4q4noARKhOcfMUPFFTK+1lPqLxLlkQbfr/T5n
izhXHsf1e8cK0gFIa5iE3acaxwbsuFcIDlYEqWiTGxIYk2mXNz7uXlY8QpkbDHQR
z1RFTjABAoGBAPa1g/mFvhYzCZnKOskDyAyZikZqZgTlLtXMoxKhGBsZU072xUQl
Z6USrRy+00rbn8ankkjZRqOnVc6jAi0mvfyVLYMG96q8jN24RwUD52IPKWpHS/BK
LUbQlkg9ajASQm5qt83AU6czdW5X/8nxsV4uOpEML5bsI4s/SfKFLtxBAoGBANXT
BaukiAQSCUDHjXFaLnt/rbdhfhFjflzCbNDCF/Ym3nqCHDL8uHOtAIkCY5LwGKxd
xL2wd81M10Dld+WOtzdt4VVibhT3NfYScr3aa5o3GoUznfkgJsBKP2TE1/NyC1lF
LTgpgu2uU7j8TjvXyQMjzmK38vbfQv6b6V0bIm1BAoGAaQnpcdCGmS8LtGXM1477
mpm4rLhaTVVCtpaVC7Z46/jBZopcfOIsGbU07Vs13NZbVZo9BzUzBTSWrQ7sO0sW
crcVFIdf5Vq34yK1YiZCWpa3/F70rw717gObKJC1aFgt3pMjRL/RHgwjwGJJLrLv
4HhwSRdWH7zUeVHt6wrXY8ECgYBmfwQN1g2ZHegvlDh56Ie1jWuBJwueXDn7TvuI
SjHgPZuR0AKickAcuwYxpuKCUfMR1NT1NL0IvVfFdPm3IWUz/cjw/ADWrfXA4fD8
jtHbl6Rvy2FjRQUuUaj3rd/yg21rOlzFuihXtKPPXapGx1ZE2goZiiG+MyFTGPuR
NOuYwQKBgDH+p2Ec/CZ5lFJYqqGBVR7fESBBeqpxr6UgfO8NBIXGuL5CCi5iCK07
hKG53Vba9OYmWWmVVYdTTjUBoG6ObqLbgzfNbkF1E70ShPoWO+6WZYpaMuO/2DjA
ysvMnQwaC0432ceRJ3r6vPAI2EPRd9KOE7Va1IFNJNmOuIkmRx8t`
	// The code to generate the PEM encoded bytes for "leafCertBody" and "leafCertPrivateKeyBody" above:
	//
	//func generateLeafCertificate() (privateKeyPem, certPem []byte) {
	//	serialNumberLimit := new(big.Int).Lsh(big.NewInt(1), 128)
	//	serialNumber, _ := rand.Int(rand.Reader, serialNumberLimit)
	//	notBefore := time.now()
	//	notAfter := notBefore.Add(365 * 24 * time.Hour)
	//
	//	template := x509.Certificate{
	//		SerialNumber: serialNumber,
	//		Issuer: pkix.Name{
	//			CommonName: "PKISVC Identity Intermediate r2",
	//		},
	//		Subject: pkix.Name{
	//			CommonName: "ocid1.instance.oc1.phx.bluhbluhbluh",
	//			OrganizationalUnit: []string{
	//				"opc-certtype:instance",
	//				"opc-instance.ocid1.phx.bluhbluhbluh",
	//				"opc-compartment:ocid1.compartment.oc1.bluhbluhbluh",
	//				fmt.Sprintf("opc-tenant:%s", tenancyID),
	//			},
	//		},
	//		NotBefore:          notBefore,
	//		NotAfter:           notAfter,
	//		PublicKeyAlgorithm: x509.RSA,
	//		SignatureAlgorithm: x509.SHA256WithRSA,
	//	}
	//
	//	privateKey, _ := rsa.GenerateKey(rand.Reader, 2048)
	//	newCertBytes, _ := x509.CreateCertificate(rand.Reader, &template, &template, privateKey.Public(), privateKey)
	//
	//	privateKeyPem = pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(privateKey)})
	//	certPem = pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: newCertBytes})
	//	return
	//}
	leafCertFingerprint  = `52:3c:9d:93:8b:b8:07:21:ce:36:30:98:ba:fc:e2:4a:bc:3a:2e:0b`
	intermediateCertBody = `MIIC4TCCAcmgAwIBAgIRAK7jQKVEO6ssUBICuPw4OwQwDQYJKoZIhvcNAQELBQAw
KjEoMCYGA1UEAxMfUEtJU1ZDIElkZW50aXR5IEludGVybWVkaWF0ZSByMjAeFw0x
NzExMzAwMDE0MDhaFw0xODExMzAwMDE0MDhaMCoxKDAmBgNVBAMTH1BLSVNWQyBJ
ZGVudGl0eSBJbnRlcm1lZGlhdGUgcjIwggEiMA0GCSqGSIb3DQEBAQUAA4IBDwAw
ggEKAoIBAQCj06NzfxfPYK6BLDKiD3tb6YLCi/XsksdBFpBsfp77eE0Wc8d6Te9M
fVaAswknpmxl4idHar7vIvJDtXq8U7UwyM75di3eMp46QR1UtmhzvCRRCQrj7uKX
ax1tAg73CJ/364YKIZpXc5p4ngOLb6bsyvd81oNDvnRXBFo3nIOpL451DiuIQGrt
6xa1wDGczvlbmTkecWH/ncIyy8JveaTsTbWfLTOgneeZEOj0OJm4eg6fudXQwqQX
TrQJTq7Nnsr1zV4eA0FVy+qVqIlGiloZHKSWkzO6ubIVmLBkKg4BXwV+zC3Lm+pW
w9U+pBoRnt5FjVHLK9U2NgfT2vTOO6G5AgMBAAGjAjAAMA0GCSqGSIb3DQEBCwUA
A4IBAQAwYegxBGNQUKTQT1OYPO6bBogzonzknPRyRcHoPdcisTUnq1EFwxz6PVrv
2cU+MrljCYblhFgnsUiwVTwBM8Eqb88HU/+8wbOyNtIc8EO4CjD7r1IU9hTv8CET
DB2LhVHN0ADwbvHGpUjC6uyyMG+5ZT+fTssx9ukCMO8hJNi2P8tBU0KGJAc0HoWV
4fEVcp0GPcXZ5IAnQfsfF4cV6DKSXGmeRZozDDlIPnWtD0f9rc3sqRtERIYS8ReS
NwVA90jrn5mKp+9B8L+o6e8qHNGoFOCY9MlM8UCqeH+wI7xUliNS9RkkYjKFg9ao
gAv8TMjAPcdnYRUXgs3UAxJQSgtB`
	// The code to generate the PEM encoded bytes for "intermediateCertBody" above:
	//
	//func generateIntermediateCertificate() (privateKeyPem, certPem []byte) {
	//	serialNumberLimit := new(big.Int).Lsh(big.NewInt(1), 128)
	//	serialNumber, _ := rand.Int(rand.Reader, serialNumberLimit)
	//	notBefore := time.now()
	//	notAfter := notBefore.Add(365 * 24 * time.Hour)
	//
	//	template := x509.Certificate{
	//		SerialNumber: serialNumber,
	//		Issuer: pkix.Name{
	//			CommonName: "Oracle BMCS Instance Identity Root - r2",
	//		},
	//		Subject: pkix.Name{
	//			CommonName: "PKISVC Identity Intermediate r2",
	//		},
	//		NotBefore:          notBefore,
	//		NotAfter:           notAfter,
	//		PublicKeyAlgorithm: x509.RSA,
	//		SignatureAlgorithm: x509.SHA256WithRSA,
	//	}
	//
	//	privateKey, _ := rsa.GenerateKey(rand.Reader, 2048)
	//	newCertBytes, _ := x509.CreateCertificate(rand.Reader, &template, &template, privateKey.Public(), privateKey)
	//
	//	privateKeyPem = pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(privateKey)})
	//	certPem = pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: newCertBytes})
	//	return
	//}
	sessionPublicKeyBody = `MIIBIjANBgkqhkiG9w0BAQEFAAOCAQ8AMIIBCgKCAQEAyQn1VgH+aBOfN8PfN6WU
ZOyiGWbozd1WaxDyP/rYCPJJIinKupp1ZcissN+A2dgcknQqJteX9XWYz31WzeAk
NEVAi9R4ZnrP0u/4921ZWmiCKIqBVxSWGE+PJcHUWJRSFNS1mQcX//UjwEYNPDKV
tPcwQqt7CYTxL77YFy0Z+s9WUmZaOJakgrCLSokeQBWdi0JibYp1mZPZv6pqsIm9
X86ef1hXyNjvEQRxuf1Bx96Y32m7FjsD251XeOEzzdESCa90Z+bHN6k7wsTRrU79
dYZF0puZUEmHID4xIF5AprOHVarrhawiddwayMQWH7GZuVzhJ2Z/Q4CK2DneR8Lr
fwIDAQAB`
	tenancyID             = `ocidv1:tenancy:oc1:phx:1234567890:bluhbluhbluh`
	expectedSecurityToken = `eyJhbGciOiJSUzI1NiIsImtpZCI6ImFzdyIsInR5cCI6IkpXVCJ9.eyJhdWQiOiJvcGMub3JhY2xlLmNvbSIsImV4cCI6MTUxMTgzODc5MywiaWF0IjoxNTExODE3MTkzLCJpc3MiOiJhdXRoU2VydmljZS5vcmFjbGUuY29tIiwib3BjLWNlcnR0eXBlIjoiaW5zdGFuY2UiLCJvcGMtY29tcGFydG1lbnQiOiJvY2lkMS5jb21wYXJ0bWVudC5vYzEuLmJsdWhibHVoYmx1aCIsIm9wYy1pbnN0YW5jZSI6Im9jaWQxLmluc3RhbmNlLm9jMS5waHguYmx1aGJsdWhibHVoIiwib3BjLXRlbmFudCI6Im9jaWR2MTp0ZW5hbmN5Om9jMTpwaHg6MTIzNDU2Nzg5MDpibHVoYmx1aGJsdWgiLCJwdHlwZSI6Imluc3RhbmNlIiwic3ViIjoib2NpZDEuaW5zdGFuY2Uub2MxLnBoeC5ibHVoYmx1aGJsdWgiLCJ0ZW5hbnQiOiJvY2lkdjE6dGVuYW5jeTpvYzE6cGh4OjEyMzQ1Njc4OTA6Ymx1aGJsdWhibHVoIiwidHR5cGUiOiJ4NTA5In0.zen7q2yJSpMjzH4ym_H7VEwZA0-vTT4Wcild-HRfLxX6A1ej4tlpACa7A24j5JoZYI4mHooZVJ8e7ZezFenK0zZx5j8RbIjsqJKwroYXExOiBXLCUwMWOLXIndEsUzzGLqnPfKHXd80vrhMLmtkVTCJqBMzvPUSYkH_ciWgmjP9m0YETdQ9ifghkADhZGt9IlnOswg0s3Bx9ASwxFZEtom0BmU9GwEuITTTZfKvndk785BlNeZMOjhovaD97-LYpv5B_PiWEz8zialK5zxjijLCw06zyA8CQRQqmVCagNUPilfz_BcPyImzvFDuzQcPyDkTcsB7weX35tafHmA_Ulg`
)

var (
	leafCertPem                   = fmt.Sprintf("-----BEGIN CERTIFICATE-----\n%s\n-----END CERTIFICATE-----\n", leafCertBody)
	leafCertBodyNoNewLine         = strings.Replace(leafCertBody, "\n", "", -1)
	leafCertPrivateKeyPem         = fmt.Sprintf("-----BEGIN RSA PRIVATE KEY-----\n%s\n-----END RSA PRIVATE KEY-----\n", leafCertPrivateKeyBody)
	intermediateCertPem           = fmt.Sprintf("-----BEGIN CERTIFICATE-----\n%s\n-----END CERTIFICATE-----\n", intermediateCertBody)
	intermediateCertBodyNoNewLine = strings.Replace(intermediateCertBody, "\n", "", -1)
	sessionPublicKeyPem           = fmt.Sprintf("-----BEGIN PUBLIC KEY-----\n%s\n-----END PUBLIC KEY-----\n", sessionPublicKeyBody)
	sessionPublicKeyBodyNoNewLine = strings.Replace(sessionPublicKeyBody, "\n", "", -1)
)

const whateverRegion = common.RegionPHX

type mockSessionKeySupplier struct {
	mock.Mock
}

func (m *mockSessionKeySupplier) Refresh() error {
	args := m.Called()
	return args.Error(0)
}

func (m *mockSessionKeySupplier) PrivateKey() *rsa.PrivateKey {
	args := m.Called()
	return args.Get(0).(*rsa.PrivateKey)
}

func (m *mockSessionKeySupplier) PublicKeyPemRaw() []byte {
	args := m.Called()
	return args.Get(0).([]byte)
}

type mockCertificateRetriever struct {
	mock.Mock
}

func (m *mockCertificateRetriever) Refresh() error {
	args := m.Called()
	return args.Error(0)
}

func (m *mockCertificateRetriever) CertificatePemRaw() []byte {
	args := m.Called()
	return args.Get(0).([]byte)
}

func (m *mockCertificateRetriever) Certificate() *x509.Certificate {
	args := m.Called()
	return args.Get(0).(*x509.Certificate)
}

func (m *mockCertificateRetriever) PrivateKeyPemRaw() []byte {
	args := m.Called()
	return args.Get(0).([]byte)
}

func (m *mockCertificateRetriever) PrivateKey() *rsa.PrivateKey {
	args := m.Called()
	return args.Get(0).(*rsa.PrivateKey)
}

type mockSecurityToken struct {
	mock.Mock
}

func (m *mockSecurityToken) String() string {
	args := m.Called()
	return args.String(0)
}

func (m *mockSecurityToken) Valid() bool {
	args := m.Called()
	return args.Bool(0)
}
