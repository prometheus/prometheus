# goji/httpauth [![GoDoc](https://godoc.org/github.com/goji/httpauth?status.svg)](https://godoc.org/github.com/goji/httpauth) [![Build Status](https://travis-ci.org/goji/httpauth.svg)](https://travis-ci.org/goji/httpauth)

`httpauth` currently provides [HTTP Basic Authentication middleware](http://tools.ietf.org/html/rfc2617) for Go. It is compatible with Go's own `net/http`, [goji](https://goji.io), Gin & anything that speaks the `http.Handler` interface.

## Example

`httpauth` provides a `SimpleBasicAuth` function to get you up and running. Particularly ideal for development servers.

Note that HTTP Basic Authentication credentials are sent over the wire "in the clear" (read: plaintext!) and therefore should not be considered a robust way to secure a HTTP server. If you're after that, you'll need to use SSL/TLS ("HTTPS") at a minimum.

### Install It

```sh
$ go get github.com/goji/httpauth
```

### Goji v2

#### Simple Usage

The fastest and simplest way to get started using `httpauth` is to use the
`SimpleBasicAuth` function.

```go

package main

import(
    "net/http"

    "goji.io"
)

func main() {
    mux := goji.NewMux()

    mux.Use(httpauth.SimpleBasicAuth("dave", "somepassword"))
    mux.Use(SomeOtherMiddleware)

    // YourHandler now requires HTTP Basic Auth
    mux.Handle(pat.Get("/some-route"), YourHandler))

    log.Fatal(http.ListenAndServe("localhost:8000", mux))
}
```

#### Advanced Usage

For more control over the process, pass a `AuthOptions` struct to `BasicAuth` instead. This allows you to:

* Configure the authentication realm.
* Provide your own UnauthorizedHandler (anything that satisfies `http.Handler`) so you can return a better looking 401 page.
* Define a custom authentication function, which is discussed in the next section.

```go

func main() {

    authOpts := httpauth.AuthOptions{
        Realm: "DevCo",
        User: "dave",
        Password: "plaintext!",
        UnauthorizedHandler: myUnauthorizedHandler,
    }

    mux := goji.NewMux()

    mux.Use(BasicAuth(authOpts))
    mux.Use(SomeOtherMiddleware)

    mux.Handle(pat.Get("/some-route"), YourHandler))

    log.Fatal(http.ListenAndServe("localhost:8000", mux))
}
```

#### Custom Authentication Function

`httpauth` will accept a custom authentication function.
Normally, you would not set `AuthOptions.User` nor `AuthOptions.Password` in this scenario.
You would instead validate the given credentials against an external system such as a database.
The contrived example below is for demonstration purposes only.

```go
func main() {

    authOpts := httpauth.AuthOptions{
        Realm: "DevCo",
        AuthFunc: myAuthFunc,
        UnauthorizedHandler: myUnauthorizedHandler,
    }

    mux := goji.NewMux()

    mux.Use(BasicAuth(authOpts))
    mux.Use(SomeOtherMiddleware)

    mux.Handle(pat.Get("/some-route"), YourHandler))

    log.Fatal(http.ListenAndServe("localhost:8000", mux))
}

// myAuthFunc is not secure.  It checks to see if the password is simply
// the username repeated three times.
func myAuthFunc(user, pass string, r *http.Request) bool {
    return pass == strings.Repeat(user, 3)
}
```

### gorilla/mux

Since it's all `http.Handler`, `httpauth` works with [gorilla/mux](https://github.com/gorilla/mux) (and most other routers) as well:

```go
package main

import (
	"net/http"

	"github.com/goji/httpauth"
	"github.com/gorilla/mux"
)

func main() {
	r := mux.NewRouter()

	r.HandleFunc("/", YourHandler)
	http.Handle("/", httpauth.SimpleBasicAuth("dave", "somepassword")(r))

	http.ListenAndServe(":7000", nil)
}

func YourHandler(w http.ResponseWriter, r *http.Request) {
	w.Write([]byte("Gorilla!\n"))
}
```

### net/http

If you're using vanilla `net/http`:

```go
package main

import(
	"net/http"

	"github.com/goji/httpauth"
)

func main() {
	http.Handle("/", httpauth.SimpleBasicAuth("dave", "somepassword")(http.HandlerFunc(YourHandler)))
	http.ListenAndServe(":7000", nil)
}
```

## Contributing

Send a pull request! Note that features on the (informal) roadmap include HTTP Digest Auth.

## License

MIT Licensed. See the LICENSE file for details.
