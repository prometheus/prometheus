# Using Promtool Health Check with Authentication

When your Prometheus server requires authentication (basic auth, TLS, etc.), you can use the `--http.config.file` flag to pass the necessary credentials to `promtool check healthy` and `promtool check ready` commands.

## Basic Authentication Example

### 1. Create an HTTP config file with basic auth

Create a file named `promtool-auth.yml`:

```yaml
basic_auth:
  username: alice
  password: verylongpassword
```

### 2. Use it with promtool health check

```bash
promtool check healthy --http.config.file=promtool-auth.yml --url=https://prometheus.example.com:9090
```

## TLS and Basic Authentication Example

If your Prometheus server requires both TLS and basic authentication:

```yaml
basic_auth:
  username: alice
  password: verylongpassword

tls_config:
  ca_file: /path/to/ca.crt
  cert_file: /path/to/client.crt
  key_file: /path/to/client.key
  insecure_skip_verify: false
```

## Docker Usage

When running Prometheus in Docker with health checks that require authentication:

```yaml
# docker-compose.yml
prometheus:
  image: prom/prometheus:latest
  ports:
    - "9090:9090"
  healthcheck:
    test: ["CMD", "promtool", "check", "healthy", "--http.config.file=/etc/prometheus/promtool-auth.yml", "--url=http://localhost:9090"]
    interval: 10s
    timeout: 5s
    retries: 3
  volumes:
    - ./prometheus.yml:/etc/prometheus/prometheus.yml
    - ./promtool-auth.yml:/etc/prometheus/promtool-auth.yml
    - ./web-config.yml:/etc/prometheus/web-config.yml
```

With `web-config.yml` requiring authentication:

```yaml
basic_auth_users:
  alice: $2y$10$...  # bcrypt hashed password
```

## Security Considerations

- **Plaintext Passwords**: The HTTP config file contains plaintext passwords. Ensure proper file permissions (e.g., `chmod 600 promtool-auth.yml`).
- **Password Files**: Consider using `password_file` or `password_ref` instead of inline plaintext passwords:

```yaml
basic_auth:
  username: alice
  password_file: /etc/prometheus/prometheus-password.txt
```

## Troubleshooting

If you get a 401 Unauthorized error:

1. Verify credentials in your config file match those configured in Prometheus
2. Check the HTTP config file path is correct and readable
3. Ensure the `--url` matches the Prometheus server address

Example debugging:

```bash
# Enable verbose output to see what's happening
promtool check healthy --http.config.file=promtool-auth.yml --url=https://prometheus.example.com:9090 -v
```
