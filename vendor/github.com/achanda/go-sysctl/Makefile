PACKAGES = $(shell go list ./...)
VETARGS?=-asmdecl -atomic -bool -buildtags -copylocks -methods \
         -nilfunc -printf -rangeloops -shift -structtags -unsafeptr

all:	test

test:
	@go test ./...
