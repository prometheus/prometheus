# Sarama tools

This folder contains applications that are useful for exploration of your Kafka cluster, or instrumentation.
Some of these tools mirror tools that ship with Kafka, but these tools won't require installing the JVM to function.

- [kafka-console-producer](./kafka-console-producer): a command line tool to produce a single message to your Kafka custer.
- [kafka-console-partitionconsumer](./kafka-console-partitionconsumer): (deprecated) a command line tool to consume a single partition of a topic on your Kafka cluster.
- [kafka-console-consumer](./kafka-console-consumer): a command line tool to consume arbitrary partitions of a topic on your Kafka cluster.

To install all tools, run `go get github.com/Shopify/sarama/tools/...`
