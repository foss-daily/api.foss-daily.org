SUDO := $(shell command -v doas 2>/dev/null || command -v sudo 2>/dev/null)

.PHONY: build install clean

build:
	CGO_ENABLED=0 GOARCH=amd64 go build -ldflags="-s -w -X main.version=$(shell git rev-parse --short HEAD)" -trimpath -o api .

install: build
	$(SUDO) rm -f /usr/local/bin/api && $(SUDO) mv api /usr/local/bin

clean:
	rm -f api

test:
	go run mvdan.cc/gofumpt@v0.9.2 -l -w .
	go run github.com/golangci/golangci-lint/v2/cmd/golangci-lint@v2.11.2 run
	go run golang.org/x/vuln/cmd/govulncheck@v1 ./...
