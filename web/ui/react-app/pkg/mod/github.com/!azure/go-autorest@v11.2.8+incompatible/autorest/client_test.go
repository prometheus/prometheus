package autorest

// Copyright 2017 Microsoft Corporation
//
//  Licensed under the Apache License, Version 2.0 (the "License");
//  you may not use this file except in compliance with the License.
//  You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
//  Unless required by applicable law or agreed to in writing, software
//  distributed under the License is distributed on an "AS IS" BASIS,
//  WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
//  See the License for the specific language governing permissions and
//  limitations under the License.

import (
	"bytes"
	"fmt"
	"io/ioutil"
	"log"
	"math/rand"
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"
	"time"

	"github.com/Azure/go-autorest/autorest/mocks"
	"github.com/Azure/go-autorest/tracing"
)

func TestLoggingInspectorWithInspection(t *testing.T) {
	b := bytes.Buffer{}
	c := Client{}
	li := LoggingInspector{Logger: log.New(&b, "", 0)}
	c.RequestInspector = li.WithInspection()

	Prepare(mocks.NewRequestWithContent("Content"),
		c.WithInspection())

	if len(b.String()) <= 0 {
		t.Fatal("autorest: LoggingInspector#WithInspection did not record Request to the log")
	}
}

func TestLoggingInspectorWithInspectionEmitsErrors(t *testing.T) {
	b := bytes.Buffer{}
	c := Client{}
	r := mocks.NewRequestWithContent("Content")
	li := LoggingInspector{Logger: log.New(&b, "", 0)}
	c.RequestInspector = li.WithInspection()

	if _, err := Prepare(r,
		c.WithInspection()); err != nil {
		t.Error(err)
	}

	if len(b.String()) <= 0 {
		t.Fatal("autorest: LoggingInspector#WithInspection did not record Request to the log")
	}
}

func TestLoggingInspectorWithInspectionRestoresBody(t *testing.T) {
	b := bytes.Buffer{}
	c := Client{}
	r := mocks.NewRequestWithContent("Content")
	li := LoggingInspector{Logger: log.New(&b, "", 0)}
	c.RequestInspector = li.WithInspection()

	Prepare(r,
		c.WithInspection())

	s, _ := ioutil.ReadAll(r.Body)
	if len(s) <= 0 {
		t.Fatal("autorest: LoggingInspector#WithInspection did not restore the Request body")
	}
}

func TestLoggingInspectorByInspecting(t *testing.T) {
	b := bytes.Buffer{}
	c := Client{}
	li := LoggingInspector{Logger: log.New(&b, "", 0)}
	c.ResponseInspector = li.ByInspecting()

	Respond(mocks.NewResponseWithContent("Content"),
		c.ByInspecting())

	if len(b.String()) <= 0 {
		t.Fatal("autorest: LoggingInspector#ByInspection did not record Response to the log")
	}
}

func TestLoggingInspectorByInspectingEmitsErrors(t *testing.T) {
	b := bytes.Buffer{}
	c := Client{}
	r := mocks.NewResponseWithContent("Content")
	li := LoggingInspector{Logger: log.New(&b, "", 0)}
	c.ResponseInspector = li.ByInspecting()

	if err := Respond(r,
		c.ByInspecting()); err != nil {
		t.Fatal(err)
	}

	if len(b.String()) <= 0 {
		t.Fatal("autorest: LoggingInspector#ByInspection did not record Response to the log")
	}
}

func TestLoggingInspectorByInspectingRestoresBody(t *testing.T) {
	b := bytes.Buffer{}
	c := Client{}
	r := mocks.NewResponseWithContent("Content")
	li := LoggingInspector{Logger: log.New(&b, "", 0)}
	c.ResponseInspector = li.ByInspecting()

	Respond(r,
		c.ByInspecting())

	s, _ := ioutil.ReadAll(r.Body)
	if len(s) <= 0 {
		t.Fatal("autorest: LoggingInspector#ByInspecting did not restore the Response body")
	}
}

func TestNewClientWithUserAgent(t *testing.T) {
	ua := "UserAgent"
	c := NewClientWithUserAgent(ua)
	completeUA := fmt.Sprintf("%s %s", UserAgent(), ua)

	if c.UserAgent != completeUA {
		t.Fatalf("autorest: NewClientWithUserAgent failed to set the UserAgent -- expected %s, received %s",
			completeUA, c.UserAgent)
	}
}

func TestAddToUserAgent(t *testing.T) {
	ua := "UserAgent"
	c := NewClientWithUserAgent(ua)
	ext := "extension"
	err := c.AddToUserAgent(ext)
	if err != nil {
		t.Fatalf("autorest: AddToUserAgent returned error -- expected nil, received %s", err)
	}
	completeUA := fmt.Sprintf("%s %s %s", UserAgent(), ua, ext)

	if c.UserAgent != completeUA {
		t.Fatalf("autorest: AddToUserAgent failed to add an extension to the UserAgent -- expected %s, received %s",
			completeUA, c.UserAgent)
	}

	err = c.AddToUserAgent("")
	if err == nil {
		t.Fatalf("autorest: AddToUserAgent didn't return error -- expected %s, received nil",
			fmt.Errorf("Extension was empty, User Agent stayed as %s", c.UserAgent))
	}
	if c.UserAgent != completeUA {
		t.Fatalf("autorest: AddToUserAgent failed to not add an empty extension to the UserAgent -- expected %s, received %s",
			completeUA, c.UserAgent)
	}
}

func TestClientSenderReturnsHttpClientByDefault(t *testing.T) {
	c := Client{}

	if fmt.Sprintf("%T", c.sender()) != "*http.Client" {
		t.Fatal("autorest: Client#sender failed to return http.Client by default")
	}
}

func TestClientSenderReturnsSetSender(t *testing.T) {
	c := Client{}

	s := mocks.NewSender()
	c.Sender = s

	if c.sender() != s {
		t.Fatal("autorest: Client#sender failed to return set Sender")
	}
}

func TestClientDoInvokesSender(t *testing.T) {
	c := Client{}

	s := mocks.NewSender()
	c.Sender = s

	c.Do(&http.Request{})
	if s.Attempts() != 1 {
		t.Fatal("autorest: Client#Do failed to invoke the Sender")
	}
}

func TestClientDoSetsUserAgent(t *testing.T) {
	ua := "UserAgent"
	c := Client{UserAgent: ua}
	r := mocks.NewRequest()
	s := mocks.NewSender()
	c.Sender = s

	c.Do(r)

	if r.UserAgent() != ua {
		t.Fatalf("autorest: Client#Do failed to correctly set User-Agent header: %s=%s",
			http.CanonicalHeaderKey(headerUserAgent), r.UserAgent())
	}
}

func TestClientDoSetsAuthorization(t *testing.T) {
	r := mocks.NewRequest()
	s := mocks.NewSender()
	c := Client{Authorizer: mockAuthorizer{}, Sender: s}

	c.Do(r)
	if len(r.Header.Get(http.CanonicalHeaderKey(headerAuthorization))) <= 0 {
		t.Fatalf("autorest: Client#Send failed to set Authorization header -- %s=%s",
			http.CanonicalHeaderKey(headerAuthorization),
			r.Header.Get(http.CanonicalHeaderKey(headerAuthorization)))
	}
}

func TestClientDoInvokesRequestInspector(t *testing.T) {
	r := mocks.NewRequest()
	s := mocks.NewSender()
	i := &mockInspector{}
	c := Client{RequestInspector: i.WithInspection(), Sender: s}

	c.Do(r)
	if !i.wasInvoked {
		t.Fatal("autorest: Client#Send failed to invoke the RequestInspector")
	}
}

func TestClientDoInvokesResponseInspector(t *testing.T) {
	r := mocks.NewRequest()
	s := mocks.NewSender()
	i := &mockInspector{}
	c := Client{ResponseInspector: i.ByInspecting(), Sender: s}

	c.Do(r)
	if !i.wasInvoked {
		t.Fatal("autorest: Client#Send failed to invoke the ResponseInspector")
	}
}

func TestClientDoReturnsErrorIfPrepareFails(t *testing.T) {
	c := Client{}
	s := mocks.NewSender()
	c.Authorizer = mockFailingAuthorizer{}
	c.Sender = s

	_, err := c.Do(&http.Request{})
	if err == nil {
		t.Fatalf("autorest: Client#Do failed to return an error when Prepare failed")
	}
}

func TestClientDoDoesNotSendIfPrepareFails(t *testing.T) {
	c := Client{}
	s := mocks.NewSender()
	c.Authorizer = mockFailingAuthorizer{}
	c.Sender = s

	c.Do(&http.Request{})
	if s.Attempts() > 0 {
		t.Fatal("autorest: Client#Do failed to invoke the Sender")
	}
}

func TestClientAuthorizerReturnsNullAuthorizerByDefault(t *testing.T) {
	c := Client{}

	if fmt.Sprintf("%T", c.authorizer()) != "autorest.NullAuthorizer" {
		t.Fatal("autorest: Client#authorizer failed to return the NullAuthorizer by default")
	}
}

func TestClientAuthorizerReturnsSetAuthorizer(t *testing.T) {
	c := Client{}
	c.Authorizer = mockAuthorizer{}

	if fmt.Sprintf("%T", c.authorizer()) != "autorest.mockAuthorizer" {
		t.Fatal("autorest: Client#authorizer failed to return the set Authorizer")
	}
}

func TestClientWithAuthorizer(t *testing.T) {
	c := Client{}
	c.Authorizer = mockAuthorizer{}

	req, _ := Prepare(&http.Request{},
		c.WithAuthorization())

	if req.Header.Get(headerAuthorization) == "" {
		t.Fatal("autorest: Client#WithAuthorizer failed to return the WithAuthorizer from the active Authorizer")
	}
}

func TestClientWithInspection(t *testing.T) {
	c := Client{}
	r := &mockInspector{}
	c.RequestInspector = r.WithInspection()

	Prepare(&http.Request{},
		c.WithInspection())

	if !r.wasInvoked {
		t.Fatal("autorest: Client#WithInspection failed to invoke RequestInspector")
	}
}

func TestClientWithInspectionSetsDefault(t *testing.T) {
	c := Client{}

	r1 := &http.Request{}
	r2, _ := Prepare(r1,
		c.WithInspection())

	if !reflect.DeepEqual(r1, r2) {
		t.Fatal("autorest: Client#WithInspection failed to provide a default RequestInspector")
	}
}

func TestClientByInspecting(t *testing.T) {
	c := Client{}
	r := &mockInspector{}
	c.ResponseInspector = r.ByInspecting()

	Respond(&http.Response{},
		c.ByInspecting())

	if !r.wasInvoked {
		t.Fatal("autorest: Client#ByInspecting failed to invoke ResponseInspector")
	}
}

func TestClientByInspectingSetsDefault(t *testing.T) {
	c := Client{}

	r := &http.Response{}
	Respond(r,
		c.ByInspecting())

	if !reflect.DeepEqual(r, &http.Response{}) {
		t.Fatal("autorest: Client#ByInspecting failed to provide a default ResponseInspector")
	}
}

func TestClientTracing(t *testing.T) {
	c := Client{}

	httpClient, ok := c.sender().(*http.Client)
	if !ok {
		t.Fatal("autorest: Client#sender failed to return http.Client by default")
	}
	if httpClient.Transport != tracing.Transport {
		t.Fatal("autorest: Client.Sender Default transport is not the tracing transport")
	}
}

func TestCookies(t *testing.T) {
	second := "second"
	expected := http.Cookie{
		Name:  "tastes",
		Value: "delicious",
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.SetCookie(w, &expected)
		b, err := ioutil.ReadAll(r.Body)
		if err != nil {
			t.Fatalf("autorest: ioutil.ReadAll failed reading request body: %s", err)
		}
		if string(b) == second {
			cookie, err := r.Cookie(expected.Name)
			if err != nil {
				t.Fatalf("autorest: r.Cookie could not get request cookie: %s", err)
			}
			if cookie == nil {
				t.Fatalf("autorest: got nil cookie, expecting %v", expected)
			}
			if cookie.Value != expected.Value {
				t.Fatalf("autorest: got cookie value '%s', expecting '%s'", cookie.Value, expected.Name)
			}
		}
	}))
	defer server.Close()

	client := NewClientWithUserAgent("")
	_, err := SendWithSender(client, mocks.NewRequestForURL(server.URL))
	if err != nil {
		t.Fatalf("autorest: first request failed: %s", err)
	}

	r2, err := http.NewRequest(http.MethodGet, server.URL, mocks.NewBody(second))
	if err != nil {
		t.Fatalf("autorest: failed creating second request: %s", err)
	}

	_, err = SendWithSender(client, r2)
	if err != nil {
		t.Fatalf("autorest: second request failed: %s", err)
	}
}

func randomString(n int) string {
	const chars = "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789"
	r := rand.New(rand.NewSource(time.Now().UTC().UnixNano()))
	s := make([]byte, n)
	for i := range s {
		s[i] = chars[r.Intn(len(chars))]
	}
	return string(s)
}
