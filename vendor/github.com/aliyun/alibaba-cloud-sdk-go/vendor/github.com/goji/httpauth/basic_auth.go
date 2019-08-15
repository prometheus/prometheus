package httpauth

import (
	"bytes"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"fmt"
	"net/http"
	"strings"
)

type basicAuth struct {
	h    http.Handler
	opts AuthOptions
}

// AuthOptions stores the configuration for HTTP Basic Authentication.
//
// A http.Handler may also be passed to UnauthorizedHandler to override the
// default error handler if you wish to serve a custom template/response.
type AuthOptions struct {
	Realm               string
	User                string
	Password            string
	AuthFunc            func(string, string, *http.Request) bool
	UnauthorizedHandler http.Handler
}

// Satisfies the http.Handler interface for basicAuth.
func (b basicAuth) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	// Check if we have a user-provided error handler, else set a default
	if b.opts.UnauthorizedHandler == nil {
		b.opts.UnauthorizedHandler = http.HandlerFunc(defaultUnauthorizedHandler)
	}

	// Check that the provided details match
	if b.authenticate(r) == false {
		b.requestAuth(w, r)
		return
	}

	// Call the next handler on success.
	b.h.ServeHTTP(w, r)
}

// authenticate retrieves and then validates the user:password combination provided in
// the request header. Returns 'false' if the user has not successfully authenticated.
func (b *basicAuth) authenticate(r *http.Request) bool {
	const basicScheme string = "Basic "

	if r == nil {
		return false
	}

	// In simple mode, prevent authentication with empty credentials if User is
	// not set. Allow empty passwords to support non-password use-cases.
	if b.opts.AuthFunc == nil && b.opts.User == "" {
		return false
	}

	// Confirm the request is sending Basic Authentication credentials.
	auth := r.Header.Get("Authorization")
	if !strings.HasPrefix(auth, basicScheme) {
		return false
	}

	// Get the plain-text username and password from the request.
	// The first six characters are skipped - e.g. "Basic ".
	str, err := base64.StdEncoding.DecodeString(auth[len(basicScheme):])
	if err != nil {
		return false
	}

	// Split on the first ":" character only, with any subsequent colons assumed to be part
	// of the password. Note that the RFC2617 standard does not place any limitations on
	// allowable characters in the password.
	creds := bytes.SplitN(str, []byte(":"), 2)

	if len(creds) != 2 {
		return false
	}

	givenUser := string(creds[0])
	givenPass := string(creds[1])

	// Default to Simple mode if no AuthFunc is defined.
	if b.opts.AuthFunc == nil {
		b.opts.AuthFunc = b.simpleBasicAuthFunc
	}

	return b.opts.AuthFunc(givenUser, givenPass, r)
}

// simpleBasicAuthFunc authenticates the supplied username and password against
// the User and Password set in the Options struct.
func (b *basicAuth) simpleBasicAuthFunc(user, pass string, r *http.Request) bool {
	// Equalize lengths of supplied and required credentials
	// by hashing them
	givenUser := sha256.Sum256([]byte(user))
	givenPass := sha256.Sum256([]byte(pass))
	requiredUser := sha256.Sum256([]byte(b.opts.User))
	requiredPass := sha256.Sum256([]byte(b.opts.Password))

	// Compare the supplied credentials to those set in our options
	if subtle.ConstantTimeCompare(givenUser[:], requiredUser[:]) == 1 &&
		subtle.ConstantTimeCompare(givenPass[:], requiredPass[:]) == 1 {
		return true
	}

	return false
}

// Require authentication, and serve our error handler otherwise.
func (b *basicAuth) requestAuth(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("WWW-Authenticate", fmt.Sprintf(`Basic realm=%q`, b.opts.Realm))
	b.opts.UnauthorizedHandler.ServeHTTP(w, r)
}

// defaultUnauthorizedHandler provides a default HTTP 401 Unauthorized response.
func defaultUnauthorizedHandler(w http.ResponseWriter, r *http.Request) {
	http.Error(w, http.StatusText(http.StatusUnauthorized), http.StatusUnauthorized)
}

// BasicAuth provides HTTP middleware for protecting URIs with HTTP Basic Authentication
// as per RFC 2617. The server authenticates a user:password combination provided in the
// "Authorization" HTTP header.
//
// Example:
//
//     package main
//
//     import(
//            "net/http"
//            "github.com/zenazn/goji"
//            "github.com/goji/httpauth"
//     )
//
//     func main() {
//          basicOpts := httpauth.AuthOptions{
//                      Realm: "Restricted",
//                      User: "Dave",
//                      Password: "ClearText",
//                  }
//
//          goji.Use(httpauth.BasicAuth(basicOpts), SomeOtherMiddleware)
//          goji.Get("/thing", myHandler)
//  }
//
// Note: HTTP Basic Authentication credentials are sent in plain text, and therefore it does
// not make for a wholly secure authentication mechanism. You should serve your content over
// HTTPS to mitigate this, noting that "Basic Authentication" is meant to be just that: basic!
func BasicAuth(o AuthOptions) func(http.Handler) http.Handler {
	fn := func(h http.Handler) http.Handler {
		return basicAuth{h, o}
	}
	return fn
}

// SimpleBasicAuth is a convenience wrapper around BasicAuth. It takes a user and password, and
// returns a pre-configured BasicAuth handler using the "Restricted" realm and a default 401 handler.
//
// Example:
//
//     package main
//
//     import(
//            "net/http"
//            "github.com/zenazn/goji/web/httpauth"
//     )
//
//     func main() {
//
//          goji.Use(httpauth.SimpleBasicAuth("dave", "somepassword"), SomeOtherMiddleware)
//          goji.Get("/thing", myHandler)
//      }
//
func SimpleBasicAuth(user, password string) func(http.Handler) http.Handler {
	opts := AuthOptions{
		Realm:    "Restricted",
		User:     user,
		Password: password,
	}
	return BasicAuth(opts)
}
