BINARY := cr
VERSION := 0.1.0
GO := CGO_ENABLED=1 go
LDFLAGS := -ldflags "-s -w -X main.version=$(VERSION)"
DIST := dist

.PHONY: build test clean install release-local npm-local

build:
	$(GO) build -o $(BINARY) $(LDFLAGS) .

test:
	$(GO) test ./... -v

clean:
	rm -f $(BINARY)
	rm -rf $(DIST)

install: build
	cp $(BINARY) $(GOPATH)/bin/$(BINARY)

lint:
	golangci-lint run ./...

# Cross-compile for all platforms (for GitHub releases / npm)
release-local: clean
	mkdir -p $(DIST)
	GOOS=darwin GOARCH=amd64 $(GO) build -o $(DIST)/cr-darwin-amd64 $(LDFLAGS) .
	GOOS=darwin GOARCH=arm64 $(GO) build -o $(DIST)/cr-darwin-arm64 $(LDFLAGS) .
	GOOS=linux GOARCH=amd64 $(GO) build -o $(DIST)/cr-linux-amd64 $(LDFLAGS) .
	GOOS=linux GOARCH=arm64 $(GO) build -o $(DIST)/cr-linux-arm64 $(LDFLAGS) .
	GOOS=windows GOARCH=amd64 $(GO) build -o $(DIST)/cr-windows-amd64.exe $(LDFLAGS) .
	@echo "Built binaries in $(DIST)/"
	@ls -la $(DIST)/

# Build and copy binary into npm package for local testing
npm-local: build
	cp $(BINARY) npm/bin/$(BINARY)
	@echo "Binary copied to npm/bin/. Test with: cd npm && npm link"
