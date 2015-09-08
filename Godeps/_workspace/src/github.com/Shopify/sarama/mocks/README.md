# sarama/mocks

The `mocks` subpackage includes mock implementations that implement the interfaces of the major sarama types.
You can use them to test your sarama applications using dependency injection.

The following mock objects are available:

- [Consumer](https://godoc.org/github.com/Shopify/sarama/mocks#Consumer), which will create [PartitionConsumer](https://godoc.org/github.com/Shopify/sarama/mocks#PartitionConsumer) mocks.
- [AsyncProducer](https://godoc.org/github.com/Shopify/sarama/mocks#AsyncProducer)
- [SyncProducer](https://godoc.org/github.com/Shopify/sarama/mocks#SyncProducer)

The mocks allow you to set expectations on them. When you close the mocks, the expectations will be verified,
and the results will be reported to the `*testing.T` object you provided when creating the mock.
