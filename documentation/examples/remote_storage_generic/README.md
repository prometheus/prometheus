## Generic Remote Storage Example

This is a simple example of how to write a server to
recieve samples from the generic remote storage output.

To use it:

```
go build
remote_storage_generic
```

and then run Prometheus as:

```
./prometheus -storage.remote.generic-url http://localhost:1234/remote  
```
