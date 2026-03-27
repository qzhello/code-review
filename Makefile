BINARY := cr
VERSION := 0.1.0

.PHONY: build test clean install

build:
	go build -o $(BINARY) -ldflags "-s -w" .

test:
	go test ./... -v

clean:
	rm -f $(BINARY)

install: build
	mv $(BINARY) $(GOPATH)/bin/$(BINARY)

lint:
	golangci-lint run ./...
