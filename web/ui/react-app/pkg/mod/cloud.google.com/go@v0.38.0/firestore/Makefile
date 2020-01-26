# Copy textproto files in this directory from the source of truth.

SRC=$(GOPATH)/src/github.com/GoogleCloudPlatform/google-cloud-common/testing/firestore

.PHONY: refresh-tests

refresh-tests:
	-rm genproto/*.pb.go
	cp $(SRC)/genproto/*.pb.go genproto
	-rm testdata/*.textproto
	cp $(SRC)/testdata/*.textproto testdata
	openssl dgst -sha1 $(SRC)/testdata/test-suite.binproto > testdata/VERSION

