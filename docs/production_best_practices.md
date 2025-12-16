---
title: Production Best Practices
sort_rank: 3
---

# Production Best Practices

This guide provides recommendations and best practices for deploying Prometheus in production environments. Following these guidelines will help ensure reliable, scalable, and maintainable Prometheus deployments.

## High Availability

### Prometheus Server Redundancy

For critical production environments, deploy multiple Prometheus instances to ensure high availability:

- **Dual Prometheus Setup**: Run at least two Prometheus servers scraping the same targets independently. This provides redundancy in case one instance fails.
- **Geographic Distribution**: For global deployments, consider running Prometheus instances in different regions or data centers.
- **Independent Storage**: Each Prometheus instance should maintain its own local storage to ensure data availability even if one instance fails.

### Alertmanager High Availability

Configure multiple Alertmanager instances in a cluster:

```yaml
alerting:
  alertmanagers:
    - static_configs:
        - targets:
          - 'alertmanager1:9093'
          - 'alertmanager2:9093'
          - 'alertmanager3:9093'
```

This ensures that alerts continue to be processed even if one or more Alertmanager instances are unavailable.

## Performance and Scalability

### Resource Planning

Proper resource allocation is critical for production deployments:

- **CPU**: Allocate sufficient CPU cores (typically 4-8 cores for medium-sized deployments, 8+ for large-scale)
- **Memory**: Plan for at least 2GB of RAM per 1 million active time series. Monitor `prometheus_tsdb_head_series` to track series count
- **Disk I/O**: Use SSDs for storage. Prometheus is I/O intensive, especially during compaction operations
- **Network**: Ensure adequate network bandwidth for scraping targets and remote write operations

### Scrape Configuration Optimization

Optimize scrape intervals based on your requirements:

```yaml
global:
  scrape_interval: 15s  # Default for most metrics
  scrape_timeout: 10s   # Should be less than scrape_interval

scrape_configs:
  - job_name: 'critical-services'
    scrape_interval: 5s   # More frequent for critical services
    scrape_timeout: 3s

  - job_name: 'batch-jobs'
    scrape_interval: 60s  # Less frequent for batch workloads
```

**Best Practices:**
- Use shorter intervals (5-15s) for critical services requiring fast alerting
- Use longer intervals (30-60s) for less critical metrics to reduce load
- Ensure `scrape_timeout` is always less than `scrape_interval`
- Monitor scrape duration with `prometheus_target_interval_length_seconds`

### Recording Rules

Use recording rules to pre-compute expensive queries:

```yaml
groups:
  - name: cpu_usage
    interval: 30s
    rules:
      - record: job:node_cpu_usage:rate5m
        expr: |
          avg by (job, instance) (
            rate(node_cpu_seconds_total[5m])
          )
```

**Benefits:**
- Reduces query latency for dashboards
- Lowers CPU usage on Prometheus server
- Enables faster alert evaluation

### Remote Write Configuration

For large-scale deployments, use remote write to offload long-term storage:

```yaml
remote_write:
  - url: "https://remote-storage.example.com/api/v1/write"
    queue_config:
      max_samples_per_send: 1000
      batch_send_deadline: 5s
      max_retries: 3
      min_backoff: 30ms
      max_backoff: 100ms
    write_relabel_configs:
      - source_labels: [__name__]
        regex: 'expensive_metric.*'
        action: drop
```

**Configuration Tips:**
- Adjust `max_samples_per_send` based on network capacity
- Use `write_relabel_configs` to filter metrics before remote write
- Monitor `prometheus_remote_storage_succeeded_samples_total` and `prometheus_remote_storage_failed_samples_total`

## Storage Management

### Retention and Compaction

Configure appropriate retention periods:

```bash
# Command-line flags
--storage.tsdb.retention.time=30d
--storage.tsdb.retention.size=50GB
```

**Recommendations:**
- Set retention based on business requirements (typically 15-90 days for local storage)
- Use `--storage.tsdb.retention.size` to prevent disk space exhaustion
- Monitor disk usage with `prometheus_tsdb_storage_blocks_bytes`
- Plan for long-term storage using remote write to systems like Thanos, Cortex, or M3

### Storage Path Configuration

Use dedicated storage volumes:

```bash
--storage.tsdb.path=/var/lib/prometheus/data
```

**Best Practices:**
- Use dedicated disks/volumes for Prometheus data
- Ensure sufficient free space (recommend 3x your retention size)
- Use fast storage (SSDs) for optimal performance
- Monitor with `prometheus_tsdb_storage_blocks_bytes`

## Security

### Network Security

- **Firewall Rules**: Restrict access to Prometheus web UI (port 9090) to authorized networks only
- **TLS Configuration**: Use TLS for all external communications:

```yaml
global:
  external_labels:
    cluster: 'production'

scrape_configs:
  - job_name: 'secure-service'
    scheme: https
    tls_config:
      ca_file: /etc/prometheus/certs/ca.crt
      cert_file: /etc/prometheus/certs/client.crt
      key_file: /etc/prometheus/certs/client.key
      insecure_skip_verify: false
```

### Authentication and Authorization

- **Basic Auth**: Enable basic authentication for web UI access
- **Reverse Proxy**: Use a reverse proxy (nginx, Apache) with authentication in front of Prometheus
- **Service Accounts**: Use service accounts and mTLS for service-to-service communication in Kubernetes

### Secrets Management

Never hardcode credentials in configuration files:

```yaml
# Use environment variables
scrape_configs:
  - job_name: 'api'
    basic_auth:
      username: '${API_USERNAME}'
      password: '${API_PASSWORD}'
```

Use secret management systems (HashiCorp Vault, AWS Secrets Manager, Kubernetes Secrets) in production.

## Monitoring Prometheus Itself

### Self-Monitoring

Prometheus should monitor itself to ensure reliability:

```yaml
scrape_configs:
  - job_name: 'prometheus'
    static_configs:
      - targets: ['localhost:9090']
```

### Key Metrics to Monitor

Monitor these critical metrics:

- **Scrape Health**: `up` metric for all targets
- **Scrape Duration**: `prometheus_target_interval_length_seconds`
- **Storage**: `prometheus_tsdb_storage_blocks_bytes`, `prometheus_tsdb_head_series`
- **Memory**: `process_resident_memory_bytes`
- **Query Performance**: `prometheus_engine_query_duration_seconds`
- **Remote Write**: `prometheus_remote_storage_samples_total`, `prometheus_remote_storage_failed_samples_total`

### Alerting Rules for Prometheus

Create alerts to monitor Prometheus health:

```yaml
groups:
  - name: prometheus_alerts
    rules:
      - alert: PrometheusTargetDown
        expr: up == 0
        for: 5m
        labels:
          severity: critical
        annotations:
          summary: "Prometheus target {{ $labels.job }}/{{ $labels.instance }} is down"

      - alert: PrometheusHighMemoryUsage
        expr: process_resident_memory_bytes > 8 * 1024 * 1024 * 1024
        for: 5m
        labels:
          severity: warning
        annotations:
          summary: "Prometheus memory usage is high: {{ $value | humanize1024 }}"

      - alert: PrometheusStorageFull
        expr: (prometheus_tsdb_storage_blocks_bytes / prometheus_tsdb_storage_blocks_bytes:max) > 0.9
        for: 5m
        labels:
          severity: critical
        annotations:
          summary: "Prometheus storage is {{ $value | humanizePercentage }} full"
```

## Configuration Management

### Version Control

- Store all Prometheus configurations in version control (Git)
- Use configuration management tools (Ansible, Terraform, etc.) for deployment
- Implement configuration validation before deployment:

```bash
promtool check config prometheus.yml
promtool check rules recording_rules.yml
```

### Configuration Reloading

Use configuration reloading to avoid downtime:

```bash
# Send SIGHUP signal
kill -HUP <prometheus_pid>

# Or use HTTP endpoint (requires --web.enable-lifecycle flag)
curl -X POST http://localhost:9090/-/reload
```

**Best Practices:**
- Always validate configuration before reloading
- Test configuration changes in non-production first
- Monitor for configuration reload errors in logs

## Labeling Strategy

### Consistent Labeling

Use consistent label naming across all services:

```yaml
# Good: Consistent label structure
- job_name: 'web-server'
  relabel_configs:
    - source_labels: [__address__]
      target_label: instance
    - source_labels: [__meta_kubernetes_namespace]
      target_label: namespace
    - source_labels: [__meta_kubernetes_pod_name]
      target_label: pod
```

**Label Best Practices:**
- Use `job` to identify the type of service
- Use `instance` to identify specific targets
- Add environment labels (`env`, `region`, `cluster`) via `external_labels`
- Avoid high-cardinality labels (user IDs, request IDs) that create too many time series
- Use `relabel_configs` to standardize labels across different discovery mechanisms

### Label Cardinality

Monitor label cardinality to prevent performance issues:

```promql
# Count unique label combinations
count by (job) ({__name__=~".+"})
```

High cardinality can lead to:
- Increased memory usage
- Slower queries
- Storage bloat

## Service Discovery

### Dynamic Configuration

Use service discovery to automatically discover targets:

```yaml
scrape_configs:
  - job_name: 'kubernetes-pods'
    kubernetes_sd_configs:
      - role: pod
    relabel_configs:
      - source_labels: [__meta_kubernetes_pod_annotation_prometheus_io_scrape]
        action: keep
        regex: true
```

**Benefits:**
- Automatic target discovery
- Reduced manual configuration
- Better alignment with infrastructure changes

## Capacity Planning

### Series Cardinality Estimation

Estimate the number of time series:

```
Total Series = (Number of Targets) × (Metrics per Target) × (Label Combinations)
```

**Example:**
- 1000 targets
- 50 metrics per target
- 10 label combinations per metric
- Total: 1000 × 50 × 10 = 500,000 series

### Resource Sizing Guidelines

Based on series count:

| Series Count | CPU Cores | Memory | Storage (30d retention) |
|-------------|-----------|--------|------------------------|
| < 100K      | 2-4       | 4-8 GB | 10-20 GB               |
| 100K-500K   | 4-8       | 8-16 GB| 50-100 GB              |
| 500K-1M     | 8-16      | 16-32 GB| 200-500 GB            |
| > 1M        | 16+       | 32+ GB | 500+ GB (use remote write) |

## Backup and Disaster Recovery

### Configuration Backups

- Regularly backup Prometheus configuration files
- Store backups in version control and separate storage systems
- Document recovery procedures

### Data Backups

For critical deployments, consider:

- **Remote Write**: Continuous backup via remote write to long-term storage
- **Periodic Snapshots**: Use `promtool tsdb create-blocks-from` for point-in-time backups
- **Storage Replication**: Replicate storage volumes at the infrastructure level

## Troubleshooting

### Common Issues

1. **High Memory Usage**
   - Check series count: `prometheus_tsdb_head_series`
   - Review label cardinality
   - Consider reducing retention or using remote write

2. **Slow Queries**
   - Use recording rules for expensive queries
   - Optimize PromQL queries
   - Check query duration: `prometheus_engine_query_duration_seconds`

3. **Scrape Failures**
   - Check target availability
   - Verify network connectivity
   - Review scrape timeout settings
   - Check logs for detailed error messages

4. **Storage Issues**
   - Monitor disk space usage
   - Adjust retention settings
   - Consider remote write for long-term storage

### Debugging Tools

Use `promtool` for debugging:

```bash
# Check configuration
promtool check config prometheus.yml

# Check rules
promtool check rules *.rules.yml

# Query Prometheus
promtool query instant 'up'

# Debug queries
promtool query instant 'rate(http_requests_total[5m])'
```

## Conclusion

Following these best practices will help ensure a reliable, scalable, and maintainable Prometheus deployment. Regularly review and update your configuration based on changing requirements and Prometheus updates.

For additional resources:
- [Prometheus Configuration Documentation](configuration/configuration.md)
- [Prometheus Querying Guide](querying/basics.md)
- [Prometheus Storage Documentation](storage.md)
- [Prometheus Community](https://prometheus.io/community/)

