FROM golang:1.12

## Warm apt cache
RUN apt-get update

# Install swagger-codegen
ENV SWAGGER_CODEGEN_VERSION=2.2.2
RUN apt-get install -y openjdk-8-jre wget && \
    wget http://repo1.maven.org/maven2/io/swagger/swagger-codegen-cli/${SWAGGER_CODEGEN_VERSION}/swagger-codegen-cli-${SWAGGER_CODEGEN_VERSION}.jar \
    -O /usr/local/bin/swagger-codegen-cli.jar && \
    apt-get remove -y wget
ENV SWAGGER_CODEGEN="java -jar /usr/local/bin/swagger-codegen-cli.jar"

# Install protoc
ENV PROTOC_VERSION=3.7.0
RUN apt-get install -y wget unzip && \
    wget https://github.com/google/protobuf/releases/download/v${PROTOC_VERSION}/protoc-${PROTOC_VERSION}-linux-x86_64.zip \
    -O /protoc-${PROTOC_VERSION}-linux-x86_64.zip && \
    unzip /protoc-${PROTOC_VERSION}-linux-x86_64.zip -d /usr/local/ && \
    rm -f /protoc-${PROTOC_VERSION}-linux-x86_64.zip && \
    apt-get remove -y unzip wget

# Install node
ENV NODE_VERSION=v10.15.2
RUN apt-get install -y wget bzip2 && \
    wget -qO- https://raw.githubusercontent.com/creationix/nvm/v0.33.11/install.sh | bash && \
    apt-get remove -y wget

# Clean up
RUN apt-get autoremove -y && \
    rm -rf /var/lib/apt/lists/*
