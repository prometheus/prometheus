#!/usr/bin/env sh

# See also https://thrift.apache.org/tutorial/go.
#
# An old version can be obtained via `brew install thrift`.
# For the latest, here's the annoying dance:
#
#   brew install automake bison pkg-config openssl
#   ln -s /usr/local/opt/openssl/include/openssl /usr/local/include # if it isn't already
#   git clone git@github.com:apache/thrift
#   ./bootstrap.sh
#   bash
#   export PATH=/usr/local/Cellar/bison/*/bin:$PATH
#   ./configure ./configure  --without-qt4 --without-qt5 --without-c_glib --without-csharp --without-java --without-erlang --without-nodejs --without-lua --without-python --without-perl --without-php --without-php_extension --without-dart --without-ruby --without-haskell --without-rs --without-cl --without-haxe --without-dotnetcore --without-d
#   make
#   sudo make install

thrift -r --gen "go:package_prefix=github.com/go-kit/kit/examples/addsvc/thrift/gen-go/,thrift_import=github.com/apache/thrift/lib/go/thrift" addsvc.thrift
