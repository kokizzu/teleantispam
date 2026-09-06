BINARY := teleantispam
GO ?= go
VERSION ?= dev
COMMIT ?= $(shell git rev-parse --short HEAD 2>/dev/null || echo none)
LDFLAGS := -s -w -X main.version=$(VERSION) -X main.commit=$(COMMIT)

.PHONY: fmt test test-ops build run tidy clean check-admin retry-failed-moderation remote-retry-failed-moderation manual-moderate remote-manual-moderate plan-remote-manual-moderation apply-remote-manual-moderation deploy install-remote-env remote-status inspect-worktree inspect-telegram-message remote-find-message plan-release apply-release

fmt:
	GO="$(GO)" bash scripts/fmt.sh

test:
	GO="$(GO)" bash scripts/test.sh

test-ops:
	python3 -m unittest tests/test_ops.py

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

remote-retry-failed-moderation:
	bash scripts/remote-retry-failed-moderation.sh

manual-moderate: build
	BINARY="bin/$(BINARY)" TELEANTISPAM_MANUAL_CHAT_ID="$(TELEANTISPAM_MANUAL_CHAT_ID)" TELEANTISPAM_MANUAL_USER_ID="$(TELEANTISPAM_MANUAL_USER_ID)" TELEANTISPAM_MANUAL_MESSAGE_IDS="$(TELEANTISPAM_MANUAL_MESSAGE_IDS)" TELEANTISPAM_MANUAL_REASON="$(TELEANTISPAM_MANUAL_REASON)" TELEANTISPAM_MANUAL_MESSAGE_SAMPLE="$(TELEANTISPAM_MANUAL_MESSAGE_SAMPLE)" bash scripts/manual-moderate.sh

remote-manual-moderate:
	bash scripts/remote-manual-moderate.sh apply

plan-remote-manual-moderation:
	bash scripts/remote-manual-moderate.sh plan

apply-remote-manual-moderation:
	bash scripts/remote-manual-moderate.sh apply

deploy: build
	bash scripts/deploy.sh

install-remote-env:
	bash scripts/install-remote-env.sh

remote-status:
	bash scripts/remote-status.sh

inspect-worktree:
	bash scripts/inspect-worktree.sh

inspect-telegram-message:
	python3 scripts/inspect-telegram-message.py

remote-find-message:
	bash scripts/remote-find-message.sh

plan-release:
	python3 scripts/release_changes.py plan

apply-release:
	python3 scripts/release_changes.py apply
