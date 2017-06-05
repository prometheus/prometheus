## Remote Write Adapter Example

This is a simple example of how to write a server to
receive samples from the remote storage output.

To use it:

```
go build
./example_write_adapter
```

...and then add the following to your `prometheus.yml`:

```yaml
remote_write:
  url: "http://localhost:1234/receive"
```

Then start Prometheus:

```
./prometheus
```
