BINARY := teleantispam
GO ?= go
VERSION ?= dev
COMMIT ?= $(shell git rev-parse --short HEAD 2>/dev/null || echo none)
LDFLAGS := -s -w -X main.version=$(VERSION) -X main.commit=$(COMMIT)

.PHONY: fmt test build run tidy clean deploy remote-status

fmt:
	GO="$(GO)" bash scripts/fmt.sh

test:
	GO="$(GO)" bash scripts/test.sh

build:
	GO="$(GO)" BINARY="$(BINARY)" LDFLAGS="$(LDFLAGS)" bash scripts/build.sh

run:
	GO="$(GO)" bash scripts/run.sh

tidy:
	GO="$(GO)" bash scripts/tidy.sh

clean:
	bash scripts/clean.sh

deploy: build
	bash scripts/deploy.sh

remote-status:
	bash scripts/remote-status.sh
