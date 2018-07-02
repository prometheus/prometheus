// Copyright (c) 2016, 2018, Oracle and/or its affiliates. All rights reserved.

package common

import (
	"bytes"
	"crypto/rsa"
	"io/ioutil"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

var (
	testTenancyOCID    = "ocid1.tenancy.oc1..aaaaaaaaba3pv6wkcr4jqae5f15p2b2m2yt2j6rx32uzr4h25vqstifsfdsq"
	testUserOCID       = "ocid1.user.oc1..aaaaaaaat5nvwcna5j6aqzjcaty5eqbb6qt2jvpkanghtgdaqedqw3rynjq"
	testFingerprint    = "20:3b:97:13:55:1c:5b:0d:d3:37:d8:50:4e:c5:3a:34"
	testComparmentOCID = "ocid1.compartment.oc1..aaaaaaaam3we6vgnherjq5q2idnccdflvjsnog7mlr6rtdb25gilchfeyjxa"

	testURL = "https://iaas.us-phoenix-1.oraclecloud.com/20160918/instances" +
		"?availabilityDomain=Pjwf%3A%20PHX-AD-1&" +
		"compartmentId=ocid1.compartment.oc1..aaaaaaaam3we6vgnherjq5q2idnccdflvjsnog7mlr6rtdb25gilchfeyjxa" +
		"&displayName=TeamXInstances&volumeId=ocid1.volume.oc1.phx.abyhqljrgvttnlx73nmrwfaux7kcvzfs3s66izvxf2h4lgvyndsdsnoiwr5q"
	testURL2 = "https://iaas.us-phoenix-1.oraclecloud.com/20160918/volumeAttachments"
	testBody = `{
    "compartmentId": "ocid1.compartment.oc1..aaaaaaaam3we6vgnherjq5q2idnccdflvjsnog7mlr6rtdb25gilchfeyjxa",
    "instanceId": "ocid1.instance.oc1.phx.abuw4ljrlsfiqw6vzzxb43vyypt4pkodawglp3wqxjqofakrwvou52gb6s5a",
    "volumeId": "ocid1.volume.oc1.phx.abyhqljrgvttnlx73nmrwfaux7kcvzfs3s66izvxf2h4lgvyndsdsnoiwr5q"
}`

	testPrivateKey = `-----BEGIN RSA PRIVATE KEY-----
MIICXgIBAAKBgQDCFENGw33yGihy92pDjZQhl0C36rPJj+CvfSC8+q28hxA161QF
NUd13wuCTUcq0Qd2qsBe/2hFyc2DCJJg0h1L78+6Z4UMR7EOcpfdUE9Hf3m/hs+F
UR45uBJeDK1HSFHD8bHKD6kv8FPGfJTotc+2xjJwoYi+1hqp1fIekaxsyQIDAQAB
AoGBAJR8ZkCUvx5kzv+utdl7T5MnordT1TvoXXJGXK7ZZ+UuvMNUCdN2QPc4sBiA
QWvLw1cSKt5DsKZ8UETpYPy8pPYnnDEz2dDYiaew9+xEpubyeW2oH4Zx71wqBtOK
kqwrXa/pzdpiucRRjk6vE6YY7EBBs/g7uanVpGibOVAEsqH1AkEA7DkjVH28WDUg
f1nqvfn2Kj6CT7nIcE3jGJsZZ7zlZmBmHFDONMLUrXR/Zm3pR5m0tCmBqa5RK95u
412jt1dPIwJBANJT3v8pnkth48bQo/fKel6uEYyboRtA5/uHuHkZ6FQF7OUkGogc
mSJluOdc5t6hI1VsLn0QZEjQZMEOWr+wKSMCQQCC4kXJEsHAve77oP6HtG/IiEn7
kpyUXRNvFsDE0czpJJBvL/aRFUJxuRK91jhjC68sA7NsKMGg5OXb5I5Jj36xAkEA
gIT7aFOYBFwGgQAQkWNKLvySgKbAZRTeLBacpHMuQdl1DfdntvAyqpAZ0lY0RKmW
G6aFKaqQfOXKCyWoUiVknQJAXrlgySFci/2ueKlIE1QqIiLSZ8V8OlpFLRnb1pzI
7U1yQXnTAEFYM560yJlzUpOb1V4cScGd365tiSMvxLOvTA==
-----END RSA PRIVATE KEY-----`

	expectedSigningString = "date: Thu, 05 Jan 2014 21:31:40 GMT\n" +
		"(request-target): get /20160918/instances?availabilityDomain=Pjwf%3A%20PH" +
		"X-AD-1&compartmentId=ocid1.compartment.oc1..aaaaaaaam3we6vgnherjq5q2i" +
		"dnccdflvjsnog7mlr6rtdb25gilchfeyjxa&displayName=TeamXInstances&" +
		"volumeId=ocid1.volume.oc1.phx.abyhqljrgvttnlx73nmrwfaux7kcvzfs3s66izvxf2h4lgvyndsdsnoiwr5q\n" +
		"host: iaas.us-phoenix-1.oraclecloud.com"
	expectedSigningString2 = `date: Thu, 05 Jan 2014 21:31:40 GMT
(request-target): post /20160918/volumeAttachments
host: iaas.us-phoenix-1.oraclecloud.com
content-length: 316
content-type: application/json
x-content-sha256: V9Z20UJTvkvpJ50flBzKE32+6m2zJjweHpDMX/U4Uy0=`

	expectedSignature = "GBas7grhyrhSKHP6AVIj/h5/Vp8bd/peM79H9Wv8kjoaCivujVXlpbKLjMPe" +
		"DUhxkFIWtTtLBj3sUzaFj34XE6YZAHc9r2DmE4pMwOAy/kiITcZxa1oHPOeRheC0jP2dqbTll" +
		"8fmTZVwKZOKHYPtrLJIJQHJjNvxFWeHQjMaR7M="
	expectedSignature2 = "Mje8vIDPlwIHmD/cTDwRxE7HaAvBg16JnVcsuqaNRim23fFPgQfLoOOxae6WqKb1uPjYEl0qIdazWaBy/Ml8DRhqlocMwoSXv0fbukP8J5N80LCmzT/FFBvIvTB91XuXI3hYfP9Zt1l7S6ieVadHUfqBedWH0itrtPJBgKmrWso="
)

type testKeyProvider struct{}

func (kp testKeyProvider) PrivateRSAKey() (*rsa.PrivateKey, error) {
	pass := ""
	key, e := PrivateKeyFromBytes([]byte(testPrivateKey), &pass)
	return key, e
}

func (kp testKeyProvider) KeyID() (string, error) {
	keyID := strings.Join([]string{testTenancyOCID, testUserOCID, testFingerprint}, "/")
	return keyID, nil
}

func TestOCIRequestSigner_HTTPRequest(t *testing.T) {
	s := ociRequestSigner{
		KeyProvider:    testKeyProvider{},
		GenericHeaders: defaultGenericHeaders,
		ShouldHashBody: defaultBodyHashPredicate,
		BodyHeaders:    defaultBodyHeaders,
	}

	r, err := http.NewRequest("GET", "http://localhost:7000/api", nil)
	assert.NoError(t, err)

	r.Header.Set(requestHeaderDate, "Thu, 05 Jan 2014 21:31:40 GMT")

	err = s.Sign(r)
	assert.NoError(t, err)

	signature := s.getSigningString(r)
	assert.Equal(t, "date: Thu, 05 Jan 2014 21:31:40 GMT\n(request-target): get /api\nhost: localhost:7000", signature)
}

func TestOCIRequestSigner_SigningString(t *testing.T) {
	s := ociRequestSigner{
		KeyProvider:    testKeyProvider{},
		GenericHeaders: defaultGenericHeaders,
		ShouldHashBody: defaultBodyHashPredicate,
		BodyHeaders:    defaultBodyHeaders}

	url, _ := url.Parse(testURL)
	r := http.Request{
		Proto:      "HTTP/1.1",
		ProtoMajor: 1,
		ProtoMinor: 1,
		Header:     make(http.Header),
		URL:        url,
	}
	r.Header.Set(requestHeaderDate, "Thu, 05 Jan 2014 21:31:40 GMT")
	r.Method = http.MethodGet
	signature := s.getSigningString(&r)

	assert.Equal(t, expectedSigningString, signature)
}

func TestOCIRequestSigner_ComputeSignature(t *testing.T) {
	s := ociRequestSigner{
		KeyProvider:    testKeyProvider{},
		GenericHeaders: defaultGenericHeaders,
		ShouldHashBody: defaultBodyHashPredicate,
		BodyHeaders:    defaultBodyHeaders}
	url, _ := url.Parse(testURL)
	r := http.Request{
		Proto:      "HTTP/1.1",
		ProtoMajor: 1,
		ProtoMinor: 1,
		Header:     make(http.Header),
		URL:        url,
	}
	r.Header.Set(requestHeaderDate, "Thu, 05 Jan 2014 21:31:40 GMT")
	r.Method = http.MethodGet
	signature, err := s.computeSignature(&r)

	assert.NoError(t, err)
	assert.Equal(t, expectedSignature, signature)
}

func TestOCIRequestSigner_Sign(t *testing.T) {
	s := ociRequestSigner{
		KeyProvider:    testKeyProvider{},
		GenericHeaders: defaultGenericHeaders,
		ShouldHashBody: defaultBodyHashPredicate,
		BodyHeaders:    defaultBodyHeaders}
	url, _ := url.Parse(testURL)
	r := http.Request{
		Proto:      "HTTP/1.1",
		ProtoMajor: 1,
		ProtoMinor: 1,
		Header:     make(http.Header),
		URL:        url,
	}
	r.Header.Set(requestHeaderDate, "Thu, 05 Jan 2014 21:31:40 GMT")
	r.Method = http.MethodGet
	err := s.Sign(&r)

	expectedAuthHeader := strings.Replace(`Signature version="1",headers="date (request-target) host",keyId="ocid1.t
enancy.oc1..aaaaaaaaba3pv6wkcr4jqae5f15p2b2m2yt2j6rx32uzr4h25vqstifsfdsq/ocid1.user.oc1..aaaaaaaat5nvwcna5j6aqzjcaty5eqbb6qt2jvpkanghtgdaqedqw3ryn
jq/20:3b:97:13:55:1c:5b:0d:d3:37:d8:50:4e:c5:3a:34",algorithm="rsa-sha256",signature="GBas7grhyrhSKHP6AVIj/h5/Vp8bd/peM79H9Wv8kjoaCivujVXlpbKLjMPe
DUhxkFIWtTtLBj3sUzaFj34XE6YZAHc9r2DmE4pMwOAy/kiITcZxa1oHPOeRheC0jP2dqbTll
8fmTZVwKZOKHYPtrLJIJQHJjNvxFWeHQjMaR7M="`, "\n", "", -1)

	assert.NoError(t, err)
	assert.Equal(t, expectedAuthHeader, r.Header.Get(requestHeaderAuthorization))

}

func TestOCIRequestSigner_SignString2(t *testing.T) {
	s := ociRequestSigner{
		KeyProvider:    testKeyProvider{},
		GenericHeaders: defaultGenericHeaders,
		ShouldHashBody: defaultBodyHashPredicate,
		BodyHeaders:    defaultBodyHeaders}
	u, _ := url.Parse(testURL2)
	r := http.Request{
		Proto:      "HTTP/1.1",
		ProtoMajor: 1,
		ProtoMinor: 1,
		Header:     make(http.Header),
		URL:        u,
	}
	bodyBuffer := bytes.NewBufferString(testBody)
	r.Body = ioutil.NopCloser(bodyBuffer)
	r.ContentLength = int64(bodyBuffer.Len())
	r.Header.Set(requestHeaderDate, "Thu, 05 Jan 2014 21:31:40 GMT")
	r.Header.Set(requestHeaderContentType, "application/json")
	r.Header.Set(requestHeaderContentLength, strconv.FormatInt(r.ContentLength, 10))
	r.Method = http.MethodPost
	calculateHashOfBody(&r)
	signingString := s.getSigningString(&r)

	assert.Equal(t, r.ContentLength, int64(316))
	assert.Equal(t, expectedSigningString2, signingString)
}

func TestOCIRequestSigner_ComputeSignature2(t *testing.T) {
	s := ociRequestSigner{
		KeyProvider:    testKeyProvider{},
		GenericHeaders: defaultGenericHeaders,
		ShouldHashBody: defaultBodyHashPredicate,
		BodyHeaders:    defaultBodyHeaders}
	u, _ := url.Parse(testURL2)
	r := http.Request{
		Proto:      "HTTP/1.1",
		ProtoMajor: 1,
		ProtoMinor: 1,
		Header:     make(http.Header),
		URL:        u,
	}
	bodyBuffer := bytes.NewBufferString(testBody)
	r.Body = ioutil.NopCloser(bodyBuffer)
	r.ContentLength = int64(bodyBuffer.Len())
	r.Header.Set(requestHeaderDate, "Thu, 05 Jan 2014 21:31:40 GMT")
	r.Header.Set(requestHeaderContentType, "application/json")
	r.Header.Set(requestHeaderContentLength, strconv.FormatInt(r.ContentLength, 10))
	r.Method = http.MethodPost
	calculateHashOfBody(&r)
	signature, err := s.computeSignature(&r)

	assert.NoError(t, err)
	assert.Equal(t, r.ContentLength, int64(316))
	assert.Equal(t, expectedSignature2, signature)
}

func TestOCIRequestSigner_Sign2(t *testing.T) {
	s := ociRequestSigner{
		KeyProvider:    testKeyProvider{},
		GenericHeaders: defaultGenericHeaders,
		ShouldHashBody: defaultBodyHashPredicate,
		BodyHeaders:    defaultBodyHeaders}
	u, _ := url.Parse(testURL2)
	r := http.Request{
		Proto:      "HTTP/1.1",
		ProtoMajor: 1,
		ProtoMinor: 1,
		Header:     make(http.Header),
		URL:        u,
	}
	bodyBuffer := bytes.NewBufferString(testBody)
	r.Body = ioutil.NopCloser(bodyBuffer)
	r.ContentLength = int64(bodyBuffer.Len())
	r.Header.Set(requestHeaderDate, "Thu, 05 Jan 2014 21:31:40 GMT")
	r.Header.Set(requestHeaderContentType, "application/json")
	r.Header.Set(requestHeaderContentLength, strconv.FormatInt(r.ContentLength, 10))
	r.Method = http.MethodPost
	err := s.Sign(&r)

	expectedAuthHeader := `Signature version="1",headers="date (request-target) host content-length content-type x-content-sha256",keyId="ocid1.tenancy.oc1..aaaaaaaaba3pv6wkcr4jqae5f15p2b2m2yt2j6rx32uzr4h25vqstifsfdsq/ocid1.user.oc1..aaaaaaaat5nvwcna5j6aqzjcaty5eqbb6qt2jvpkanghtgdaqedqw3rynjq/20:3b:97:13:55:1c:5b:0d:d3:37:d8:50:4e:c5:3a:34",algorithm="rsa-sha256",signature="Mje8vIDPlwIHmD/cTDwRxE7HaAvBg16JnVcsuqaNRim23fFPgQfLoOOxae6WqKb1uPjYEl0qIdazWaBy/Ml8DRhqlocMwoSXv0fbukP8J5N80LCmzT/FFBvIvTB91XuXI3hYfP9Zt1l7S6ieVadHUfqBedWH0itrtPJBgKmrWso="`
	assert.NoError(t, err)
	assert.Equal(t, r.ContentLength, int64(316))
	assert.Equal(t, expectedAuthHeader, r.Header.Get(requestHeaderAuthorization))
}

func TestOCIRequestSigner_SignEmptyBody(t *testing.T) {
	s := ociRequestSigner{KeyProvider: testKeyProvider{},
		ShouldHashBody: defaultBodyHashPredicate,
	}
	u, _ := url.Parse(testURL2)
	r := http.Request{
		Proto:      "HTTP/1.1",
		ProtoMajor: 1,
		ProtoMinor: 1,
		Header:     make(http.Header),
		URL:        u,
	}
	bodyBuffer := bytes.NewBufferString("")
	r.Body = ioutil.NopCloser(bodyBuffer)
	r.ContentLength = int64(bodyBuffer.Len())
	r.Header.Set(requestHeaderDate, "Thu, 05 Jan 2014 21:31:40 GMT")
	r.Header.Set(requestHeaderContentType, "application/json")
	r.Header.Set(requestHeaderContentLength, strconv.FormatInt(r.ContentLength, 10))
	r.Method = http.MethodPost
	err := s.Sign(&r)

	assert.NoError(t, err)
	assert.Equal(t, r.ContentLength, int64(0))
	assert.NotEmpty(t, r.Header.Get(requestHeaderAuthorization))
	assert.NotEmpty(t, r.Header.Get(requestHeaderXContentSHA256))
}
