---
title: HTTP configuration for promtool
sort_rank: 6
---

Promtool is a versatile CLI tool for Prometheus that supports validation, debugging, querying, unit testing, tsdb management, pushing data, and experimental PromQL editing.

Prometheus supports basic authentication and TLS. Since promtool needs to connect to Prometheus, we need to provide the authentication details. To specify those authentication details, use the `--http.config.file` for all requests that need to communicate with Prometheus.
For instance, if you would like to check whether your local Prometheus server is healthy, you would use:
```bash
promtool check healthy --url=http://localhost:9090 --http.config.file=http-config-file.yml
```

The file is written in [YAML format](https://en.wikipedia.org/wiki/YAML), defined by the schema described below.
Brackets indicate that a parameter is optional. For non-list parameters the value is set to the specified default.

The file is read upon every http request, such as any change in the
configuration and the certificates is picked up immediately.

Generic placeholders are defined as follows:

* `<bool>`: a boolean that can take the values `true` or `false`
* `<filename>`: a valid path to a file
* `<secret>`: a regular string that is a secret, such as a password
* `<string>`: a regular string

A valid example file can be found [here](/documentation/examples/promtool-http-config-file.yml).

```yaml
# Note that `basic_auth` and `authorization` options are mutually exclusive.

# Sets the `Authorization` header with the configured username and password.
# `username_ref` and `password_ref`refer to the name of the secret within the secret manager.
# `password`, `password_file` and `password_ref` are mutually exclusive.
basic_auth:
  [ username: <string> ]
  [ username_file: <filename> ]
  [ username_ref: <string> ]
  [ password: <secret> ]
  [ password_file: <string> ]
  [ password_ref: <string> ]

# Optional the `Authorization` header configuration.
authorization:
  # Sets the authentication type.
  [ type: <string> | default: Bearer ]
  # Sets the credentials. It is mutually exclusive with
  # `credentials_file`.
  [ credentials: <secret> ]
  # Sets the credentials with the credentials read from the configured file.
  # It is mutually exclusive with `credentials`.
  [ credentials_file: <filename> ]
  [ credentials_ref: <string> ]

# Optional OAuth 2.0 configuration.
# Cannot be used at the same time as basic_auth or authorization.
oauth2:
  [ <oauth2> ]

tls_config:
  [ <tls_config> ]

[ follow_redirects: <bool> | default: true ]

# Whether to enable HTTP2.
[ enable_http2: <bool> | default: true ]

# Optional proxy URL.
[ proxy_url: <string> ]
# Comma-separated string that can contain IPs, CIDR notation, domain names
# that should be excluded from proxying. IP and domain names can
# contain port numbers.
[ no_proxy: <string> ]
[ proxy_from_environment: <bool> ]
[ proxy_connect_header:
  [ <string>: [ <secret>, ... ] ] ]

# `http_headers` specifies a set of headers that will be injected into each request.
http_headers:
  [ <string>: <header> ]
```

## \<oauth2\>
OAuth 2.0 authentication using the client credentials grant type.
```yaml
# `client_id` and `client_secret` are used to authenticate your
# application with the authorization server in order to get
# an access token.
# `client_secret`, `client_secret_file` and `client_secret_ref` are mutually exclusive.
client_id: <string>
[ client_secret: <secret> ]
[ client_secret_file: <filename> ]
[ client_secret_ref: <string> ]

# `scopes` specify the reason for the resource access.
scopes:
  [ - <string> ...]

# The URL to fetch the token from.
token_url: <string>

# Optional parameters to append to the token URL.
[ endpoint_params:
    <string>: <string> ... ]

# Configures the token request's TLS settings.
tls_config:
  [ <tls_config> ]

# Optional proxy URL.
[ proxy_url: <string> ]
# Comma-separated string that can contain IPs, CIDR notation, domain names
# that should be excluded from proxying. IP and domain names can
# contain port numbers.
[ no_proxy: <string> ]
[ proxy_from_environment: <bool> ]
[ proxy_connect_header:
  [ <string>: [ <secret>, ... ] ] ]
```

## <tls_config>
```yaml
# For the following configurations, use either `ca`, `cert` and `key` or `ca_file`, `cert_file` and `key_file` or use `ca_ref`, `cert_ref` or `key_ref`.
# Text of the CA certificate to use for the server.
[ ca: <string> ]
# CA certificate to validate the server certificate with.
[ ca_file: <filename> ]
# `ca_ref` is the name of the secret within the secret manager to use as the CA cert.
[ ca_ref: <string> ]

# Text of the client cert file for the server.
[ cert: <string> ]
# Certificate file for client certificate authentication.
[ cert_file: <filename> ]
# `cert_ref` is the name of the secret within the secret manager to use as the client certificate.
[ cert_ref: <string> ]

# Text of the client key file for the server.
[ key: <secret> ]
# Key file for client certificate authentication.
[ key_file: <filename> ]
# `key_ref` is the name of the secret within the secret manager to use as the client key.
[ key_ref: <string> ]

# ServerName extension to indicate the name of the server.
# http://tools.ietf.org/html/rfc4366#section-3.1
[ server_name: <string> ]

# Disable validation of the server certificate.
[ insecure_skip_verify: <bool> ]

# Minimum acceptable TLS version. Accepted values: TLS10 (TLS 1.0), TLS11 (TLS
# 1.1), TLS12 (TLS 1.2), TLS13 (TLS 1.3).
# If unset, promtool will use Go default minimum version, which is TLS 1.2.
# See MinVersion in https://pkg.go.dev/crypto/tls#Config.
[ min_version: <string> ]
# Maximum acceptable TLS version. Accepted values: TLS10 (TLS 1.0), TLS11 (TLS
# 1.1), TLS12 (TLS 1.2), TLS13 (TLS 1.3).
# If unset, promtool will use Go default maximum version, which is TLS 1.3.
# See MaxVersion in https://pkg.go.dev/crypto/tls#Config.
[ max_version: <string> ]
```

## \<header\>
`header` represents the configuration for a single HTTP header.
```yaml
[ values:
  [ - <string> ... ] ]

[ secrets:
  [ - <secret> ... ] ]

[ files:
  [ - <filename> ... ] ]
```
