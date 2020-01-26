#! /bin/bash

set -e

JEKYLL_VERSION=3.5
BUNDLE_DIR="/tmp/grpc-gateway-bundle"

if [ ! -d "${BUNDLE_DIR}" ]; then
  mkdir "${BUNDLE_DIR}"

  # Run this to update the Gemsfile.lock
  docker run --rm \
    --volume="${PWD}:/srv/jekyll" \
    -e "JEKYLL_UID=$(id -u)" \
    -e "JEKYLL_GID=$(id -g)" \
    --volume="/tmp/grpc-gateway-bundle:/usr/local/bundle" \
    -it "jekyll/builder:${JEKYLL_VERSION}" \
    bundle update
fi

docker run --rm \
  --volume="${PWD}:/srv/jekyll" \
  -p 35729:35729 -p 4000:4000 \
  -e "JEKYLL_UID=$(id -u)" \
  -e "JEKYLL_GID=$(id -g)" \
  --volume="/tmp/grpc-gateway-bundle:/usr/local/bundle" \
  -it "jekyll/builder:${JEKYLL_VERSION}" \
  jekyll serve
