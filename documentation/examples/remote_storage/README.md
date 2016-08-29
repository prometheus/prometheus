## Generic Remote Storage Example

This is a simple example of how to write a server to
receive samples from the remote storage output.

To use it:

```
go build
remote_storage
```

...and then run Prometheus as:

```
./prometheus -storage.remote.address=localhost:1234
```
