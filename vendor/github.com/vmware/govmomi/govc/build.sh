#!/bin/bash -e

git_version=$(git describe --dirty)
 if [[ $git_version == *-dirty ]] ; then
  echo 'Working tree is dirty.'
  echo 'NOTE: This script is meant for building govc releases via release.sh'
  echo 'To build govc from source see: https://github.com/vmware/govmomi/blob/master/govc/README.md#source'
  exit 1
fi

ldflags="-X github.com/vmware/govmomi/govc/version.gitVersion=${git_version}"

BUILD_OS=${BUILD_OS:-darwin linux windows freebsd}
BUILD_ARCH=${BUILD_ARCH:-386 amd64}

for os in ${BUILD_OS}; do
  export GOOS="${os}"
  for arch in ${BUILD_ARCH}; do
    export GOARCH="${arch}"

    out="govc_${os}_${arch}"
    if [ "${os}" == "windows" ]; then
      out="${out}.exe"
    fi

    set -x
    go build \
      -o="${out}" \
      -pkgdir="./_pkg" \
      -compiler='gc' \
      -ldflags="${ldflags}" \
      github.com/vmware/govmomi/govc &
    set +x
  done
done

wait
