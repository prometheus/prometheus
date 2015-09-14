# kafka-console-producer

A simple command line tool to produce a single message to Kafka.

### Installation

    go get github.com/Shopify/sarama/tools/kafka-console-producer


### Usage

    # Minimum invocation
    kafka-console-producer -topic=test -value=value -brokers=kafka1:9092

    # It will pick up a KAFKA_PEERS environment variable
    export KAFKA_PEERS=kafka1:9092,kafka2:9092,kafka3:9092
    kafka-console-producer -topic=test -value=value

    # It will read the value from stdin by using pipes
    echo "hello world" | kafka-console-producer -topic=test

    # Specify a key:
    echo "hello world" | kafka-console-producer -topic=test -key=key

    # Partitioning: by default, kafka-console-producer will partition as follows:
    # - manual partitioning if a -partition is provided
    # - hash partitioning by key if a -key is provided
    # - random partioning otherwise.
    #
    # You can override this using the -partitioner argument:
    echo "hello world" | kafka-console-producer -topic=test -key=key -partitioner=random

    # Display all command line options
    kafka-console-producer -help
