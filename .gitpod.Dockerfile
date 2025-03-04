FROM gitpod/workspace-full

# Set Node.js version as an environment variable.
ENV CUSTOM_NODE_VERSION=16

# Install and use the specified Node.js version via nvm.
RUN bash -c ". .nvm/nvm.sh && nvm install ${CUSTOM_NODE_VERSION} && nvm use ${CUSTOM_NODE_VERSION} && nvm alias default ${CUSTOM_NODE_VERSION}"

# Ensure nvm uses the default Node.js version in all new shells.
RUN echo "nvm use default &>/dev/null" >> ~/.bashrc.d/51-nvm-fix

# Remove any existing Go installation in $HOME path.
RUN rm -rf $HOME/go $HOME/go-packages

# Export go environment variables.
RUN echo "export GOPATH=/workspace/go" >> ~/.bashrc.d/300-go && \
    echo "export GOBIN=\$GOPATH/bin" >> ~/.bashrc.d/300-go && \
    echo "export GOROOT=${HOME}/go" >> ~/.bashrc.d/300-go && \
    echo "export PATH=\$GOROOT/bin:\$GOBIN:\$PATH" >> ~/.bashrc

# Reload the environment variables to ensure go environment variables are
# available in subsequent commands.
RUN bash -c "source ~/.bashrc && source ~/.bashrc.d/300-go"

# Fetch the Go version dynamically from the Prometheus go.mod file and Install Go in $HOME path.
RUN export CUSTOM_GO_VERSION=$(curl -sSL "https://raw.githubusercontent.com/prometheus/prometheus/main/go.mod" | awk '/^go/{print $2".0"}') && \
    curl -fsSL "https://dl.google.com/go/go${CUSTOM_GO_VERSION}.linux-amd64.tar.gz" | \
    tar -xz -C $HOME

# Fetch the goyacc parser version dynamically from the Prometheus Makefile
# and install it globally in $GOBIN path.
RUN GOYACC_VERSION=$(curl -fsSL "https://raw.githubusercontent.com/prometheus/prometheus/main/Makefile" | awk -F'=' '/GOYACC_VERSION \?=/{gsub(/ /, "", $2); print $2}') && \
    go install "golang.org/x/tools/cmd/goyacc@${GOYACC_VERSION}"
