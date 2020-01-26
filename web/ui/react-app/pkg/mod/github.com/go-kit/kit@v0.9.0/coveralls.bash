#!/usr/bin/env bash

if ! type -P gover
then
	echo gover missing: go get github.com/modocache/gover
	exit 1
fi

if ! type -P goveralls
then
	echo goveralls missing: go get github.com/mattn/goveralls
	exit 1
fi

if [[ "$COVERALLS_TOKEN" == "" ]]
then
	echo COVERALLS_TOKEN not set
	exit 1
fi

go list ./... | grep -v '/examples/' | cut -d'/' -f 4- | while read d
do
	cd $d
	go test -covermode count -coverprofile coverage.coverprofile
	cd -
done

gover
goveralls -coverprofile gover.coverprofile -service travis-ci -repotoken $COVERALLS_TOKEN
find . -name '*.coverprofile' -delete

