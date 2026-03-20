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
  - url: "http://localhost:1234/receive"
    protobuf_message: "io.prometheus.write.v2.Request"
```

or for the eventually deprecated Remote Write 1.0 message:

```yaml
remote_write:
  - url: "http://localhost:1234/receive"
    protobuf_message: "prometheus.WriteRequest"
```

Then start Prometheus (in separate terminal):

```
./prometheus --enable-feature=metadata-wal-records
```
