# Locate the Go toolchain.
# This project expects Go at /home/kjsst/.local/go/bin/go (common in this workspace).
# We prefer an absolute path so builds work even if "go" is not in PATH (e.g. running as root).
GO := $(shell \
  command -v go 2>/dev/null || \
  ( [ -x /home/kjsst/.local/go/bin/go ] && echo /home/kjsst/.local/go/bin/go ) || \
  ( [ -x $(HOME)/.local/go/bin/go ] && echo $(HOME)/.local/go/bin/go ) || \
  echo go \
)

GOFLAGS ?= -trimpath
BIN_DIR := bin

# Make sure the correct Go directory is in PATH (helps other tools and sub-processes)
ifneq ($(findstring /home/kjsst/.local/go/bin,$(GO)),)
  export PATH := /home/kjsst/.local/go/bin:$(PATH)
endif

BINS := conductor dashboard h2-thrasher quic-burner l7-abuser slowloris ws-flood sync-runtime vector-bench

.PHONY: all build-all test tidy clean lab-up lab-down sync-runtime bench bench-all list-vectors

all: build-all

build-all: $(addprefix $(BIN_DIR)/,$(BINS))

# Proper dependency: rebuild if ANY .go file or go.mod changes.
# The old rule only watched cmd/*/main.go and ignored internal packages.
GO_SRCS := $(shell find . -name '*.go' 2>/dev/null)
$(BIN_DIR)/%: $(GO_SRCS) go.mod go.sum
	@mkdir -p $(BIN_DIR)
	$(GO) build $(GOFLAGS) -o $@ ./cmd/$*

test:
	$(GO) test ./...

tidy:
	$(GO) mod tidy

clean:
	rm -rf $(BIN_DIR)

sync-runtime:
	./$(BIN_DIR)/sync-runtime

lab-up:
	docker compose --profile attacker --profile dashboard --profile vectors --profile monitoring up -d --build

lab-up-auto:
	docker compose --profile attacker --profile auto --profile vectors --profile monitoring up -d --build

lab-down:
	./deploy/scripts/lab-down.sh

list-vectors:
	./$(BIN_DIR)/vector-bench -list

bench: build-all
	./deploy/scripts/bench-vectors.sh

bench-all: bench

bench-one: build-all
	./$(BIN_DIR)/vector-bench -vector $(VECTOR) -target $(TARGET) -duration $(DURATION)