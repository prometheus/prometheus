// Copyright 2017 The Prometheus Authors
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

package httputil

import (
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"io/ioutil"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"

	"github.com/prometheus/prometheus/config"
	"github.com/prometheus/prometheus/util/testutil"
)

const (
	TLSCAChainPath        = "testdata/tls-ca-chain.pem"
	ServerCertificatePath = "testdata/server.crt"
	ServerKeyPath         = "testdata/server.key"
	BarneyCertificatePath = "testdata/barney.crt"
	BarneyKeyNoPassPath   = "testdata/barney-no-pass.key"
	MissingCA             = "missing/ca.crt"
	MissingCert           = "missing/cert.crt"
	MissingKey            = "missing/secret.key"

	ExpectedMessage        = "I'm here to serve you!!!"
	BearerToken            = "theanswertothegreatquestionoflifetheuniverseandeverythingisfortytwo"
	BearerTokenFile        = "testdata/bearer.token"
	MissingBearerTokenFile = "missing/bearer.token"
	ExpectedBearer         = "Bearer " + BearerToken
	ExpectedUsername       = "arthurdent"
	ExpectedPassword       = "42"
)

func newTestServer(handler func(w http.ResponseWriter, r *http.Request)) (*httptest.Server, error) {
	testServer := httptest.NewUnstartedServer(http.HandlerFunc(handler))

	tlsCAChain, err := ioutil.ReadFile(TLSCAChainPath)
	if err != nil {
		return nil, fmt.Errorf("Can't read %s", TLSCAChainPath)
	}
	serverCertificate, err := tls.LoadX509KeyPair(ServerCertificatePath, ServerKeyPath)
	if err != nil {
		return nil, fmt.Errorf("Can't load X509 key pair %s - %s", ServerCertificatePath, ServerKeyPath)
	}

	rootCAs := x509.NewCertPool()
	rootCAs.AppendCertsFromPEM(tlsCAChain)

	testServer.TLS = &tls.Config{
		Certificates: make([]tls.Certificate, 1),
		RootCAs:      rootCAs,
		ClientAuth:   tls.RequireAndVerifyClientCert,
		ClientCAs:    rootCAs}
	testServer.TLS.Certificates[0] = serverCertificate
	testServer.TLS.BuildNameToCertificate()

	testServer.StartTLS()

	return testServer, nil
}

func TestNewClientFromConfig(t *testing.T) {
	var newClientValidConfig = []struct {
		clientConfig config.HTTPClientConfig
		handler      func(w http.ResponseWriter, r *http.Request)
	}{
		{
			clientConfig: config.HTTPClientConfig{
				TLSConfig: config.TLSConfig{
					CAFile:             "",
					CertFile:           BarneyCertificatePath,
					KeyFile:            BarneyKeyNoPassPath,
					ServerName:         "",
					InsecureSkipVerify: true},
			},
			handler: func(w http.ResponseWriter, r *http.Request) {
				fmt.Fprint(w, ExpectedMessage)
			},
		}, {
			clientConfig: config.HTTPClientConfig{
				TLSConfig: config.TLSConfig{
					CAFile:             TLSCAChainPath,
					CertFile:           BarneyCertificatePath,
					KeyFile:            BarneyKeyNoPassPath,
					ServerName:         "",
					InsecureSkipVerify: false},
			},
			handler: func(w http.ResponseWriter, r *http.Request) {
				fmt.Fprint(w, ExpectedMessage)
			},
		}, {
			clientConfig: config.HTTPClientConfig{
				BearerToken: BearerToken,
				TLSConfig: config.TLSConfig{
					CAFile:             TLSCAChainPath,
					CertFile:           BarneyCertificatePath,
					KeyFile:            BarneyKeyNoPassPath,
					ServerName:         "",
					InsecureSkipVerify: false},
			},
			handler: func(w http.ResponseWriter, r *http.Request) {
				bearer := r.Header.Get("Authorization")
				if bearer != ExpectedBearer {
					fmt.Fprintf(w, "The expected Bearer Authorization (%s) differs from the obtained Bearer Authorization (%s)",
						ExpectedBearer, bearer)
				} else {
					fmt.Fprint(w, ExpectedMessage)
				}
			},
		}, {
			clientConfig: config.HTTPClientConfig{
				BearerTokenFile: BearerTokenFile,
				TLSConfig: config.TLSConfig{
					CAFile:             TLSCAChainPath,
					CertFile:           BarneyCertificatePath,
					KeyFile:            BarneyKeyNoPassPath,
					ServerName:         "",
					InsecureSkipVerify: false},
			},
			handler: func(w http.ResponseWriter, r *http.Request) {
				bearer := r.Header.Get("Authorization")
				if bearer != ExpectedBearer {
					fmt.Fprintf(w, "The expected Bearer Authorization (%s) differs from the obtained Bearer Authorization (%s)",
						ExpectedBearer, bearer)
				} else {
					fmt.Fprint(w, ExpectedMessage)
				}
			},
		}, {
			clientConfig: config.HTTPClientConfig{
				BasicAuth: &config.BasicAuth{
					Username: ExpectedUsername,
					Password: ExpectedPassword,
				},
				TLSConfig: config.TLSConfig{
					CAFile:             TLSCAChainPath,
					CertFile:           BarneyCertificatePath,
					KeyFile:            BarneyKeyNoPassPath,
					ServerName:         "",
					InsecureSkipVerify: false},
			},
			handler: func(w http.ResponseWriter, r *http.Request) {
				username, password, ok := r.BasicAuth()
				if ok == false {
					fmt.Fprintf(w, "The Authorization header wasn't set")
				} else if ExpectedUsername != username {
					fmt.Fprintf(w, "The expected username (%s) differs from the obtained username (%s).", ExpectedUsername, username)
				} else if ExpectedPassword != password {
					fmt.Fprintf(w, "The expected password (%s) differs from the obtained password (%s).", ExpectedPassword, password)
				} else {
					fmt.Fprint(w, ExpectedMessage)
				}
			},
		},
	}

	for _, validConfig := range newClientValidConfig {
		testServer, err := newTestServer(validConfig.handler)
		if err != nil {
			t.Fatal(err.Error())
		}
		defer testServer.Close()

		client, err := NewClientFromConfig(validConfig.clientConfig, "test")
		if err != nil {
			t.Errorf("Can't create a client from this config: %+v", validConfig.clientConfig)
			continue
		}
		response, err := client.Get(testServer.URL)
		if err != nil {
			t.Errorf("Can't connect to the test server using this config: %+v", validConfig.clientConfig)
			continue
		}

		message, err := ioutil.ReadAll(response.Body)
		response.Body.Close()
		if err != nil {
			t.Errorf("Can't read the server response body using this config: %+v", validConfig.clientConfig)
			continue
		}

		trimMessage := strings.TrimSpace(string(message))
		if ExpectedMessage != trimMessage {
			t.Errorf("The expected message (%s) differs from the obtained message (%s) using this config: %+v",
				ExpectedMessage, trimMessage, validConfig.clientConfig)
		}
	}
}

func TestNewClientFromInvalidConfig(t *testing.T) {
	var newClientInvalidConfig = []struct {
		clientConfig config.HTTPClientConfig
		errorMsg     string
	}{
		{
			clientConfig: config.HTTPClientConfig{
				TLSConfig: config.TLSConfig{
					CAFile:             MissingCA,
					CertFile:           "",
					KeyFile:            "",
					ServerName:         "",
					InsecureSkipVerify: true},
			},
			errorMsg: fmt.Sprintf("unable to use specified CA cert %s:", MissingCA),
		}, {
			clientConfig: config.HTTPClientConfig{
				BearerTokenFile: MissingBearerTokenFile,
				TLSConfig: config.TLSConfig{
					CAFile:             TLSCAChainPath,
					CertFile:           BarneyCertificatePath,
					KeyFile:            BarneyKeyNoPassPath,
					ServerName:         "",
					InsecureSkipVerify: false},
			},
			errorMsg: fmt.Sprintf("unable to read bearer token file %s:", MissingBearerTokenFile),
		},
	}

	for _, invalidConfig := range newClientInvalidConfig {
		client, err := NewClientFromConfig(invalidConfig.clientConfig, "test")
		if client != nil {
			t.Errorf("A client instance was returned instead of nil using this config: %+v", invalidConfig.clientConfig)
		}
		if err == nil {
			t.Errorf("No error was returned using this config: %+v", invalidConfig.clientConfig)
		}
		if !strings.Contains(err.Error(), invalidConfig.errorMsg) {
			t.Errorf("Expected error %s does not contain %s", err.Error(), invalidConfig.errorMsg)
		}
	}
}

func TestBearerAuthRoundTripper(t *testing.T) {
	const (
		newBearerToken = "goodbyeandthankyouforthefish"
	)

	fakeRoundTripper := testutil.NewRoundTripCheckRequest(func(req *http.Request) {
		bearer := req.Header.Get("Authorization")
		if bearer != ExpectedBearer {
			t.Errorf("The expected Bearer Authorization (%s) differs from the obtained Bearer Authorization (%s)",
				ExpectedBearer, bearer)
		}
	}, nil, nil)

	//Normal flow
	bearerAuthRoundTripper := NewBearerAuthRoundTripper(BearerToken, fakeRoundTripper)
	request, _ := http.NewRequest("GET", "/hitchhiker", nil)
	request.Header.Set("User-Agent", "Douglas Adams mind")
	bearerAuthRoundTripper.RoundTrip(request)

	//Should honor already Authorization header set
	bearerAuthRoundTripperShouldNotModifyExistingAuthorization := NewBearerAuthRoundTripper(newBearerToken, fakeRoundTripper)
	request, _ = http.NewRequest("GET", "/hitchhiker", nil)
	request.Header.Set("Authorization", ExpectedBearer)
	bearerAuthRoundTripperShouldNotModifyExistingAuthorization.RoundTrip(request)
}

func TestBasicAuthRoundTripper(t *testing.T) {
	const (
		newUsername = "fordprefect"
		newPassword = "towel"
	)

	fakeRoundTripper := testutil.NewRoundTripCheckRequest(func(req *http.Request) {
		username, password, ok := req.BasicAuth()
		if ok == false {
			t.Errorf("The Authorization header wasn't set")
		}
		if ExpectedUsername != username {
			t.Errorf("The expected username (%s) differs from the obtained username (%s).", ExpectedUsername, username)
		}
		if ExpectedPassword != password {
			t.Errorf("The expected password (%s) differs from the obtained password (%s).", ExpectedPassword, password)
		}
	}, nil, nil)

	//Normal flow
	basicAuthRoundTripper := NewBasicAuthRoundTripper(ExpectedUsername,
		ExpectedPassword, fakeRoundTripper)
	request, _ := http.NewRequest("GET", "/hitchhiker", nil)
	request.Header.Set("User-Agent", "Douglas Adams mind")
	basicAuthRoundTripper.RoundTrip(request)

	//Should honor already Authorization header set
	basicAuthRoundTripperShouldNotModifyExistingAuthorization := NewBasicAuthRoundTripper(newUsername,
		newPassword, fakeRoundTripper)
	request, _ = http.NewRequest("GET", "/hitchhiker", nil)
	request.SetBasicAuth(ExpectedUsername, ExpectedPassword)
	basicAuthRoundTripperShouldNotModifyExistingAuthorization.RoundTrip(request)
}

func TestTLSConfig(t *testing.T) {
	configTLSConfig := config.TLSConfig{
		CAFile:             TLSCAChainPath,
		CertFile:           BarneyCertificatePath,
		KeyFile:            BarneyKeyNoPassPath,
		ServerName:         "localhost",
		InsecureSkipVerify: false}

	tlsCAChain, err := ioutil.ReadFile(TLSCAChainPath)
	if err != nil {
		t.Fatalf("Can't read the CA certificate chain (%s)",
			TLSCAChainPath)
	}
	rootCAs := x509.NewCertPool()
	rootCAs.AppendCertsFromPEM(tlsCAChain)

	barneyCertificate, err := tls.LoadX509KeyPair(BarneyCertificatePath, BarneyKeyNoPassPath)
	if err != nil {
		t.Fatalf("Can't load the client key pair ('%s' and '%s'). Reason: %s",
			BarneyCertificatePath, BarneyKeyNoPassPath, err)
	}

	expectedTLSConfig := &tls.Config{
		RootCAs:            rootCAs,
		Certificates:       []tls.Certificate{barneyCertificate},
		ServerName:         configTLSConfig.ServerName,
		InsecureSkipVerify: configTLSConfig.InsecureSkipVerify}
	expectedTLSConfig.BuildNameToCertificate()

	tlsConfig, err := NewTLSConfig(configTLSConfig)
	if err != nil {
		t.Fatalf("Can't create a new TLS Config from a configuration (%s).", err)
	}

	if !reflect.DeepEqual(tlsConfig, expectedTLSConfig) {
		t.Fatalf("Unexpected TLS Config result: \n\n%+v\n expected\n\n%+v", tlsConfig, expectedTLSConfig)
	}
}

func TestTLSConfigEmpty(t *testing.T) {
	configTLSConfig := config.TLSConfig{
		CAFile:             "",
		CertFile:           "",
		KeyFile:            "",
		ServerName:         "",
		InsecureSkipVerify: true}

	expectedTLSConfig := &tls.Config{
		InsecureSkipVerify: configTLSConfig.InsecureSkipVerify}
	expectedTLSConfig.BuildNameToCertificate()

	tlsConfig, err := NewTLSConfig(configTLSConfig)
	if err != nil {
		t.Fatalf("Can't create a new TLS Config from a configuration (%s).", err)
	}

	if !reflect.DeepEqual(tlsConfig, expectedTLSConfig) {
		t.Fatalf("Unexpected TLS Config result: \n\n%+v\n expected\n\n%+v", tlsConfig, expectedTLSConfig)
	}
}

func TestTLSConfigInvalidCA(t *testing.T) {
	var invalidTLSConfig = []struct {
		configTLSConfig config.TLSConfig
		errorMessage    string
	}{
		{
			configTLSConfig: config.TLSConfig{
				CAFile:             MissingCA,
				CertFile:           "",
				KeyFile:            "",
				ServerName:         "",
				InsecureSkipVerify: false},
			errorMessage: fmt.Sprintf("unable to use specified CA cert %s:", MissingCA),
		}, {
			configTLSConfig: config.TLSConfig{
				CAFile:             "",
				CertFile:           MissingCert,
				KeyFile:            BarneyKeyNoPassPath,
				ServerName:         "",
				InsecureSkipVerify: false},
			errorMessage: fmt.Sprintf("unable to use specified client cert (%s) & key (%s):", MissingCert, BarneyKeyNoPassPath),
		}, {
			configTLSConfig: config.TLSConfig{
				CAFile:             "",
				CertFile:           BarneyCertificatePath,
				KeyFile:            MissingKey,
				ServerName:         "",
				InsecureSkipVerify: false},
			errorMessage: fmt.Sprintf("unable to use specified client cert (%s) & key (%s):", BarneyCertificatePath, MissingKey),
		},
	}

	for _, anInvalididTLSConfig := range invalidTLSConfig {
		tlsConfig, err := NewTLSConfig(anInvalididTLSConfig.configTLSConfig)
		if tlsConfig != nil && err == nil {
			t.Errorf("The TLS Config could be created even with this %+v", anInvalididTLSConfig.configTLSConfig)
			continue
		}
		if !strings.Contains(err.Error(), anInvalididTLSConfig.errorMessage) {
			t.Errorf("The expected error should contain %s, but got %s", anInvalididTLSConfig.errorMessage, err)
		}
	}
}
