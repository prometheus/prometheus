# kafka-console-consumer

A simple command line tool to consume partitions of a topic and print the
messages on the standard output.

### Installation

    go get github.com/Shopify/sarama/tools/kafka-console-consumer

### Usage

    # Minimum invocation
    kafka-console-consumer -topic=test -brokers=kafka1:9092

    # It will pick up a KAFKA_PEERS environment variable
    export KAFKA_PEERS=kafka1:9092,kafka2:9092,kafka3:9092
    kafka-console-consumer -topic=test

    # You can specify the offset you want to start at. It can be either
    # `oldest`, `newest`. The default is `newest`.
    kafka-console-consumer -topic=test -offset=oldest
    kafka-console-consumer -topic=test -offset=newest

    # You can specify the partition(s) you want to consume as a comma-separated
    # list. The default is `all`.
    kafka-console-consumer -topic=test -partitions=1,2,3

    # Display all command line options
    kafka-console-consumer -help
