FROM gitpod/workspace-full@sha256:45638cd7eaeb35fc64e8c55a0aa65eaba41b82853133da4f5b537e0dd218b9f8

ENV CUSTOM_NODE_VERSION=16
ENV CUSTOM_GO_VERSION=1.19
ENV GOPATH=$HOME/go-packages
ENV GOROOT=$HOME/go
ENV PATH=$GOROOT/bin:$GOPATH/bin:$PATH

RUN bash -c ". .nvm/nvm.sh && nvm install ${CUSTOM_NODE_VERSION} && nvm use ${CUSTOM_NODE_VERSION} && nvm alias default ${CUSTOM_NODE_VERSION}"

RUN echo "nvm use default &>/dev/null" >> ~/.bashrc.d/51-nvm-fix
RUN curl -fsSL https://dl.google.com/go/go${GO_VERSION}.linux-amd64.tar.gz | tar xzs \
    && printf '%s\n' 'export GOPATH=/workspace/go' \
                      'export PATH=$GOPATH/bin:$PATH' > $HOME/.bashrc.d/300-go

