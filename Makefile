BINARY := hive
CMD := ./cmd/hive
OUT_DIR := bin
CACHE_DIR ?= .cache
GOCACHE ?= $(abspath $(CACHE_DIR)/go-build)
GOMODCACHE ?= $(abspath $(CACHE_DIR)/go-mod)
GO := go

export GOCACHE
export GOMODCACHE

.DEFAULT_GOAL := help

.PHONY: help cache-dirs build run serve swarm test test-unit test-integration vet fmt clean distclean

help:
	@echo ""
	@echo "The Hive CE — Make targets"
	@echo ""
	@echo "  make build              Build the CE binary into $(OUT_DIR)/$(BINARY)"
	@echo "  make run                Build and run the local binary"
	@echo "  make serve              Run CE directly with go run"
	@echo "  make swarm              Launch local swarm script"
	@echo ""
	@echo "  make test               Run the full test suite"
	@echo "  make test-unit          Run unit tests only"
	@echo "  make test-integration   Run integration tests only"
	@echo "  make vet                Run go vet"
	@echo "  make fmt                Run gofmt on Go sources"
	@echo ""
	@echo "  make clean              Remove build output"
	@echo "  make distclean          Remove build output and local caches"
	@echo ""

cache-dirs:
	@mkdir -p $(OUT_DIR) $(GOCACHE) $(GOMODCACHE)

build: cache-dirs
	$(GO) build -o $(OUT_DIR)/$(BINARY) $(CMD)

run: build
	./$(OUT_DIR)/$(BINARY)

serve: cache-dirs
	$(GO) run $(CMD)

swarm: build
	./scripts/swarm.sh

test: cache-dirs
	HIVE_RUN_INTEGRATION=1 HIVE_TEST_LOG=quiet $(GO) test ./...

test-unit: cache-dirs
	HIVE_TEST_LOG=quiet $(GO) test ./internal/... ./cmd/hive

test-integration: cache-dirs
	HIVE_RUN_INTEGRATION=1 HIVE_TEST_LOG=quiet $(GO) test ./cmd/hive -run '^TestEndToEnd$$'

vet: cache-dirs
	$(GO) vet ./...

fmt:
	@files="$$(find . -type f -name '*.go' -not -path './.git/*' -not -path './$(OUT_DIR)/*' -not -path './$(CACHE_DIR)/*')"; \
	if [ -n "$$files" ]; then gofmt -w $$files; fi

clean:
	rm -rf $(OUT_DIR)

distclean: clean
	rm -rf $(CACHE_DIR)
