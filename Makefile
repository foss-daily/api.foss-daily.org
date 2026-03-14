.PHONY: build install clean

build:
	CGO_ENABLED=0 GOARCH=amd64 go build -ldflags="-s -w" -trimpath -o api .

install: build
	rm -rf /usr/local/bin/api && mv api /usr/local/bin

clean:
	rm -f api
