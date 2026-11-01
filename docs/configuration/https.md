---
title: HTTPS and authentication
sort_rank: 7
---

Prometheus supports basic authentication and TLS.
This is **experimental** and might change in the future.

To specify which web configuration file to load, use the `--web.config.file` flag.

The file is written in [YAML format](https://en.wikipedia.org/wiki/YAML),
defined by the scheme described below.
Brackets indicate that a parameter is optional. For non-list parameters the
value is set to the specified default.

The file is read upon every http request, such as any change in the
configuration and the certificates is picked up immediately.

Generic placeholders are defined as follows:

* `<boolean>`: a boolean that can take the values `true` or `false`
* `<filename>`: a valid path in the current working directory
* `<secret>`: a regular string that is a secret, such as a password
* `<string>`: a regular string
* `<int>`: a regular integer
* `<duration>`: a duration as accepted by Go's [time.ParseDuration](https://pkg.go.dev/time#ParseDuration), such as `1s`, `500ms` or `1m30s`; unlike durations in the main configuration file, the units `d`, `w` and `y` are not supported

A valid example file can be found [here](/documentation/examples/web-config.yml).

```yaml
tls_server_config:
  # Certificate for server to use to authenticate to client.
  # Expected to be passed as a PEM encoded sequence of bytes as a string.
  #
  # NOTE: If passing the cert inline, cert_file should not be specified below.
  [ cert: <string> ]

  # Key for server to use to authenticate to client.
  # Expected to be passed as a PEM encoded sequence of bytes as a string.
  #
  # NOTE: If passing the key inline, key_file should not be specified below.
  [ key: <secret> ]

  # CA certificate for client certificate authentication to the server.
  # Expected to be passed as a PEM encoded sequence of bytes as a string.
  #
  # NOTE: If passing the client_ca inline, client_ca_file should not be specified below.
  [ client_ca: <string> ]

  # Certificate and key files for server to use to authenticate to client.
  cert_file: <filename>
  key_file: <filename>

  # Server policy for client authentication. Maps to ClientAuth Policies.
  # For more detail on clientAuth options:
  # https://golang.org/pkg/crypto/tls/#ClientAuthType
  #
  # NOTE: If you want to enable client authentication, you need to use
  # RequireAndVerifyClientCert. Other values are insecure.
  [ client_auth_type: <string> | default = "NoClientCert" ]

  # CA certificate for client certificate authentication to the server.
  [ client_ca_file: <filename> ]

  # Verify that the client certificate has a Subject Alternate Name (SAN)
  # which is an exact match to an entry in this list, else terminate the
  # connection. SAN match can be one or multiple of the following: DNS,
  # IP, e-mail, or URI address from https://pkg.go.dev/crypto/x509#Certificate.
  [ client_allowed_sans:
    [ - <string> ] ]

  # Minimum TLS version that is acceptable.
  [ min_version: <string> | default = "TLS12" ]

  # Maximum TLS version that is acceptable.
  [ max_version: <string> | default = "TLS13" ]

  # List of supported cipher suites for TLS versions up to TLS 1.2. If empty,
  # Go default cipher suites are used. Available cipher suites are documented
  # in the go documentation:
  # https://golang.org/pkg/crypto/tls/#pkg-constants
  #
  # Note that only the cipher returned by the following function are supported:
  # https://pkg.go.dev/crypto/tls#CipherSuites
  [ cipher_suites:
    [ - <string> ] ]

  # Deprecated: this setting is accepted and ignored.
  #
  # It used to control whether the server followed its own cipher suite
  # preference, as expressed in the order of elements in cipher_suites, or the
  # client's. Go has selected the cipher suite itself since Go 1.17, taking
  # into account inferred client hardware, server hardware and security, and
  # the underlying setting has had no effect since then.
  [ prefer_server_cipher_suites: <boolean> | default = true ]

  # Elliptic curves that will be used in an ECDHE handshake, in preference
  # order. Available curves are documented in the go documentation:
  # https://golang.org/pkg/crypto/tls/#CurveID
  [ curve_preferences:
    [ - <string> ] ]

http_server_config:
  # Enable HTTP/2 support. Note that HTTP/2 is only supported with TLS.
  # This can not be changed on the fly.
  [ http2: <boolean> | default = true ]
  # List of headers that can be added to HTTP responses.
  [ headers:
    # Set the Content-Security-Policy header to HTTP responses.
    # Unset if blank.
    [ Content-Security-Policy: <string> ]
    # Set the X-Frame-Options header to HTTP responses.
    # Unset if blank. Accepted values are deny and sameorigin.
    # https://developer.mozilla.org/en-US/docs/Web/HTTP/Headers/X-Frame-Options
    [ X-Frame-Options: <string> ]
    # Set the X-Content-Type-Options header to HTTP responses.
    # Unset if blank. Accepted value is nosniff.
    # https://developer.mozilla.org/en-US/docs/Web/HTTP/Headers/X-Content-Type-Options
    [ X-Content-Type-Options: <string> ]
    # Set the X-XSS-Protection header to all responses.
    # Unset if blank.
    # https://developer.mozilla.org/en-US/docs/Web/HTTP/Headers/X-XSS-Protection
    [ X-XSS-Protection: <string> ]
    # Set the Strict-Transport-Security header to HTTP responses.
    # Unset if blank.
    # Please make sure that you use this with care as this header might force
    # browsers to load Prometheus and the other applications hosted on the same
    # domain and subdomains over HTTPS.
    # https://developer.mozilla.org/en-US/docs/Web/HTTP/Headers/Strict-Transport-Security
    [ Strict-Transport-Security: <string> ] ]

# Usernames and hashed passwords that have full access to the web
# server via basic authentication. If empty, no basic authentication is
# required. Passwords are hashed with bcrypt.
basic_auth_users:
  [ <string>: <secret> ... ]

# Rate limiting of requests to the web server, using a token bucket.
# Requests over the limit are rejected with HTTP 429 Too Many Requests.
# A single limit applies to all requests from all clients, and is checked
# before authentication.
# This can not be changed on the fly.
[ rate_limit:
  # Average time between requests: one request is allowed per interval,
  # with bursts up to burst. Set to 0 to disable the rate limiter.
  [ interval: <duration> | default = 0 ]
  # Number of requests permitted in a burst. Must be at least 1 when the
  # rate limiter is enabled, otherwise all requests are rejected.
  [ burst: <int> | default = 0 ] ]
```

