FROM gitpod/workspace-full

ENV CUSTOM_NODE_VERSION=16

RUN bash -c ". .nvm/nvm.sh && nvm install ${CUSTOM_NODE_VERSION} && nvm use ${CUSTOM_NODE_VERSION} && nvm alias default ${CUSTOM_NODE_VERSION}"

RUN echo "nvm use default &>/dev/null" >> ~/.bashrc.d/51-nvm-fix
