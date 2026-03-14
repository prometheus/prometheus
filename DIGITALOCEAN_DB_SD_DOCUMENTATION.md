# DigitalOcean Managed Database Service Discovery

## Overview

DigitalOcean Managed Database Service Discovery (`digitalocean_db_sd_configs`) allows Prometheus to automatically discover DigitalOcean managed database clusters as scrape targets. This complements the existing DigitalOcean Droplets service discovery by adding support for managed databases (PostgreSQL, MySQL, Redis, etc.).

## Configuration

Add the following to your `prometheus.yml`:

```yaml
scrape_configs:
  - job_name: 'digitalocean-databases'
    digitalocean_db_sd_configs:
      # Your DigitalOcean API token
      bearer_token: 'your-do-api-token'
      
      # Port to connect to databases (default: 5432 for PostgreSQL)
      port: 5432
      
      # How often to refresh the database list (default: 60s)
      refresh_interval: 60s
      
      # HTTP client configuration
      http_client_config:
        follow_redirects: true
        enable_http2: true
```

## Required Configuration

- `bearer_token`: Your DigitalOcean API token with read permissions for databases

## Optional Configuration

- `port`: Port to use for discovered databases (default: 5432)
- `refresh_interval`: How often to refresh the database list (default: 60s)
- `http_client_config`: HTTP client configuration for API requests

## Discovered Labels

Each discovered database cluster provides the following labels:

### Core Labels

| Label | Description | Example |
|--------|-------------|---------|
| `__meta_digitalocean_db_id` | Database cluster ID | `123456` |
| `__meta_digitalocean_db_name` | Database name | `production-pg` |
| `__meta_digitalocean_db_engine` | Database engine | `pg`, `mysql`, `redis` |
| `__meta_digitalocean_db_status` | Database status | `online`, `offline` |
| `__meta_digitalocean_db_region` | Database region | `nyc1` |
| `__meta_digitalocean_db_host` | Public connection host | `db-example-do-user-1234.db.ondigitalocean.com` |
| `__meta_digitalocean_db_private_host` | Private connection host | `private-db-example-do-user-1234.db.ondigitalocean.com` |

### Tag Labels

For each tag applied to the database, a label is created:

| Label | Description | Example |
|--------|-------------|---------|
| `__meta_digitalocean_db_tag_env` | Environment tag | `production` |
| `__meta_digitalocean_db_tag_team` | Team tag | `backend` |
| `__meta_digitalocean_db_tag_project` | Project tag | `user-service` |

### Address Label

| Label | Description | Example |
|--------|-------------|---------|
| `__address__` | Target address (host:port) | `db-example-do-user-1234.db.ondigitalocean.com:5432` |

## Usage Examples

### Basic Configuration

```yaml
scrape_configs:
  - job_name: 'digitalocean-databases'
    digitalocean_db_sd_configs:
      - bearer_token: 'dop_v1_1234567890abcdef1234567890abcdef'
```

### Multiple Database Types

```yaml
scrape_configs:
  - job_name: 'postgresql-databases'
    digitalocean_db_sd_configs:
      - bearer_token: 'dop_v1_1234567890abcdef1234567890abcdef'
        port: 5432
        refresh_interval: 30s

  - job_name: 'mysql-databases'
    digitalocean_db_sd_configs:
      - bearer_token: 'dop_v1_1234567890abcdef1234567890abcdef'
        port: 3306
        refresh_interval: 30s
```

### Using Private Hosts

If your Prometheus server runs in the same VPC as your databases, you can use private hosts:

```yaml
scrape_configs:
  - job_name: 'digitalocean-databases'
    digitalocean_db_sd_configs:
      - bearer_token: 'dop_v1_1234567890abcdef1234567890abcdef'
        port: 5432
    relabel_configs:
      - source_labels: [__address__]
        regex: '([^:]+):.*'
        target_label: __address__
        replacement: '${1}'
```

## API Rate Limits

DigitalOcean API has rate limits. The default refresh interval of 60 seconds helps avoid hitting API limits. If you need more frequent refreshes, ensure your token has appropriate permissions.

## Security Considerations

- Store your DigitalOcean API token securely (use environment variables or secret management)
- Use tokens with minimal required permissions (read-only for databases)
- Consider using VPC peering and private hosts when possible

## Troubleshooting

### Common Issues

1. **No databases discovered**: Check your API token has database read permissions
2. **Connection timeouts**: Verify network connectivity to DigitalOcean API
3. **Incorrect ports**: Ensure you're using the right port for your database type

### Debug Logging

Enable debug logging to see API requests:

```yaml
global:
  log_level: debug
```

## Differences from Droplets SD

| Feature | Droplets SD | Database SD |
|---------|---------------|-------------|
| Discovers | Droplets (VMs) | Managed Databases |
| Default Port | 80 | 5432 |
| Host Type | Public/Private IPs | Connection Hosts |
| Labels | droplet_* | db_* |

## Migration from File SD

If you're currently using `file_sd` with manually maintained database lists:

```yaml
# Before (file_sd)
scrape_configs:
  - job_name: 'databases'
    file_sd_configs:
      - files:
        - 'databases.json'

# After (digitalocean_db_sd)
scrape_configs:
  - job_name: 'databases'
    digitalocean_db_sd_configs:
      - bearer_token: 'your-api-token'
        port: 5432
```

This eliminates manual maintenance and automatically picks up new databases.
