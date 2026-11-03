# storage/remote: expose remote write failure reasons

## Summary

Remote write retry and failure counters currently combine several different
failure modes. This makes it difficult to determine whether a remote receiver
is rate limiting Prometheus, returning server errors, or unreachable due to a
network problem.

This change adds a bounded `reason` label to the samples, exemplars, and
histograms retry and failure counters.

## Changes

- Classify retryable errors as `rate_limited`, `server_error`, or
  `network_error`.
- Classify non-retried HTTP 4xx responses as `client_error`.
- Classify partial Remote Write 2.0 responses as `partial_write`.
- Classify data discarded during hard shutdown as `shutdown`.
- Propagate retry reasons through the queue manager into retry metrics.
- Add focused assertions for HTTP error classification and labeled retry
  metrics.

The reason values are fixed and bounded, so the label does not introduce
unbounded cardinality.

## Testing

```text
go test ./storage/remote
git diff --check
```

The HTTP tests use local `httptest` servers. Queue-manager tests use injected
write-client errors and do not contact an external remote-write receiver.

Related: #19870

```release-notes
[ENHANCEMENT] Remote write retry and failure metrics now expose a bounded reason label.
```
