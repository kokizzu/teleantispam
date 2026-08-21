BINARY := teleantispam
GO ?= go
VERSION ?= dev
COMMIT ?= $(shell git rev-parse --short HEAD 2>/dev/null || echo none)
LDFLAGS := -s -w -X main.version=$(VERSION) -X main.commit=$(COMMIT)

.PHONY: fmt test build run tidy clean check-admin retry-failed-moderation deploy install-remote-env remote-status

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

check-admin: build
	BINARY="bin/$(BINARY)" TELEANTISPAM_ADMIN_CHECK_STATUS_PATH="$${TELEANTISPAM_ADMIN_CHECK_STATUS_PATH:-tmp/gophers-admin-status.json}" bash scripts/check-admin.sh

retry-failed-moderation: build
	BINARY="bin/$(BINARY)" bash scripts/retry-failed-moderation.sh

deploy: build
	bash scripts/deploy.sh

install-remote-env:
	bash scripts/install-remote-env.sh

remote-status:
	bash scripts/remote-status.sh
