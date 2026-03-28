BINARY   = webui
GOARCH  ?= $(shell go env GOARCH)
GOOS    ?= $(shell go env GOOS)

build:
	CGO_ENABLED=0 GOOS=$(GOOS) GOARCH=$(GOARCH) \
	go build -ldflags="-s -w" -o $(BINARY) .

dev: build
	go run . --listen :10000 --session-key /tmp/webui-session.key --insecure-tls $(ARGS)

clean:
	rm -f $(BINARY)

.PHONY: build dev clean
