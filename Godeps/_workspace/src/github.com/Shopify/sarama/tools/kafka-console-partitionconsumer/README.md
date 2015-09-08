# kafka-console-partitionconsumer

NOTE: this tool is deprecated in favour of the more general and more powerful
`kafka-console-consumer`.

A simple command line tool to consume a partition of a topic and print the messages
on the standard output.

### Installation

    go get github.com/Shopify/sarama/tools/kafka-console-partitionconsumer

### Usage

    # Minimum invocation
    kafka-console-partitionconsumer -topic=test -partition=4 -brokers=kafka1:9092

    # It will pick up a KAFKA_PEERS environment variable
    export KAFKA_PEERS=kafka1:9092,kafka2:9092,kafka3:9092
    kafka-console-partitionconsumer -topic=test -partition=4

    # You can specify the offset you want to start at. It can be either
    # `oldest`, `newest`, or a specific offset number
    kafka-console-partitionconsumer -topic=test -partition=3 -offset=oldest
    kafka-console-partitionconsumer -topic=test -partition=2 -offset=1337

    # Display all command line options
    kafka-console-partitionconsumer -help
