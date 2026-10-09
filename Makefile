GO ?= go
VERSION ?= dev

.PHONY: test check build
test:
	$(GO) test -race -count=1 ./...
check:
	test -z "$$($(GO) fmt ./...)"
	$(GO) vet ./...
	$(GO) test -race -count=1 -coverprofile=coverage.out ./...
build:
	mkdir -p dist
	CGO_ENABLED=0 $(GO) build -buildvcs=false -trimpath -ldflags="-s -w -X main.version=$(VERSION)" -o dist/notify-relay ./cmd/notify-relay
