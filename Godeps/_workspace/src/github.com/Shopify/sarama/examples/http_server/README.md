# HTTP server example

This HTTP server example shows you how to use the AsyncProducer and SyncProducer, and how to test them using mocks. The server simply sends the data of the HTTP request's query string to Kafka, and send a 200 result if that succeeds. For every request, it will send an access log entry to Kafka as well in the background.

If you need to know whether a message was successfully sent to the Kafka cluster before you can send your HTTP response, using the `SyncProducer` is probably the simplest way to achieve this. If you don't care, e.g. for the access log, using the `AsyncProducer` will let you fire and forget. You can send the HTTP response, while the message is being produced in the background.

One important thing to note is that both the `SyncProducer` and `AsyncProducer` are **thread-safe**. Go's `http.Server` handles requests concurrently in different goroutines, but you can use a single producer safely. This will actually achieve efficiency gains as the producer will be able to batch messages from concurrent requests together.
