BINARY   := domovoi
VERSION  ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
LDFLAGS  := -s -w -X main.version=$(VERSION)
GOLANGCI := github.com/golangci/golangci-lint/v2/cmd/golangci-lint@v2.12.2
MDI_TTF  := $(HOME)/.cache/domovoi/materialdesignicons-webfont.ttf
MDI_URL  := https://raw.githubusercontent.com/Templarian/MaterialDesign-Webfont/master/fonts/materialdesignicons-webfont.ttf

.PHONY: build test test-integration lint generate icons check release clean

## build: the binary for this machine
build:
	CGO_ENABLED=0 go build -ldflags '$(LDFLAGS)' -o $(BINARY) ./cmd/domovoi

## test: unit tests with the race detector
test:
	go test -race ./...

## test-integration: unit and integration tests
test-integration:
	go test -race -tags integration ./...

## lint: golangci-lint at the pinned version
lint:
	go run $(GOLANGCI) run --build-tags integration ./...

## generate: mocks from every interfaces.go
generate:
	go generate ./...

## icons: regenerate the icon font subset from the upstream font (needs node and `npm install subset-font`)
icons: $(MDI_TTF)
	node tools/subset-icons.mjs $(MDI_TTF) internal/icon/mdi-subset.ttf $$(go run ./internal/icon/cmd/codepoints)

$(MDI_TTF):
	mkdir -p $(dir $@)
	curl -sSL -o $@ $(MDI_URL)

## check: everything CI runs
check: lint test-integration
	GOOS=linux GOARCH=arm64 CGO_ENABLED=0 go build ./...

## release: the Raspberry Pi binary
release:
	GOOS=linux GOARCH=arm64 CGO_ENABLED=0 go build -ldflags '$(LDFLAGS)' -o $(BINARY)-arm64 ./cmd/domovoi

clean:
	rm -f $(BINARY) $(BINARY)-arm64
