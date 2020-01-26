package main

import (
	"fmt"
	"net/http"
	"os"
	"strings"
	"testing"

	"github.com/pact-foundation/pact-go/dsl"
)

func TestPactStringsvcUppercase(t *testing.T) {
	if os.Getenv("WRITE_PACTS") == "" {
		t.Skip("skipping Pact contracts; set WRITE_PACTS environment variable to enable")
	}

	pact := dsl.Pact{
		Consumer: "addsvc",
		Provider: "stringsvc",
	}
	defer pact.Teardown()

	pact.AddInteraction().
		UponReceiving("stringsvc uppercase").
		WithRequest(dsl.Request{
			Headers: dsl.MapMatcher{"Content-Type": dsl.String("application/json; charset=utf-8")},
			Method:  "POST",
			Path:    dsl.String("/uppercase"),
			Body:    `{"s":"foo"}`,
		}).
		WillRespondWith(dsl.Response{
			Status:  200,
			Headers: dsl.MapMatcher{"Content-Type": dsl.String("application/json; charset=utf-8")},
			Body:    `{"v":"FOO"}`,
		})

	if err := pact.Verify(func() error {
		u := fmt.Sprintf("http://localhost:%d/uppercase", pact.Server.Port)
		req, err := http.NewRequest("POST", u, strings.NewReader(`{"s":"foo"}`))
		if err != nil {
			return err
		}
		req.Header.Set("Content-Type", "application/json; charset=utf-8")
		if _, err = http.DefaultClient.Do(req); err != nil {
			return err
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}

	pact.WritePact()
}
