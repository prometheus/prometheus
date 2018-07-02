// Copyright (c) 2016, 2018, Oracle and/or its affiliates. All rights reserved.

package auth

import (
	"bytes"
	"crypto/sha1"
	"crypto/x509"
	"fmt"
	"github.com/oracle/oci-go-sdk/common"
	"net/http"
	"net/http/httputil"
	"strings"
)

// httpGet makes a simple HTTP GET request to the given URL, expecting only "200 OK" status code.
// This is basically for the Instance Metadata Service.
func httpGet(url string) (body bytes.Buffer, err error) {
	var response *http.Response
	if response, err = http.Get(url); err != nil {
		return
	}

	common.IfDebug(func() {
		if dump, e := httputil.DumpResponse(response, true); e == nil {
			common.Logf("Dump Response %v", string(dump))
		} else {
			common.Debugln(e)
		}
	})

	defer response.Body.Close()
	if _, err = body.ReadFrom(response.Body); err != nil {
		return
	}

	if response.StatusCode != http.StatusOK {
		err = fmt.Errorf("HTTP Get failed: URL: %s, Status: %s, Message: %s",
			url, response.Status, body.String())
		return
	}

	return
}

func extractTenancyIDFromCertificate(cert *x509.Certificate) string {
	for _, nameAttr := range cert.Subject.Names {
		value := nameAttr.Value.(string)
		if strings.HasPrefix(value, "opc-tenant:") {
			return value[len("opc-tenant:"):]
		}
	}
	return ""
}

func fingerprint(certificate *x509.Certificate) string {
	fingerprint := sha1.Sum(certificate.Raw)
	return colonSeparatedString(fingerprint)
}

func colonSeparatedString(fingerprint [sha1.Size]byte) string {
	spaceSeparated := fmt.Sprintf("% x", fingerprint)
	return strings.Replace(spaceSeparated, " ", ":", -1)
}
