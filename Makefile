
    BIN_DIR := bin
    NAME := opx
    VERSION ?= $(shell svu current 2>/dev/null || echo "dev")
    COMMIT ?= $(shell git rev-parse --short HEAD 2>/dev/null || echo "unknown")
    DATE ?= $(shell date -u +"%Y-%m-%dT%H:%M:%SZ")
    
    LDFLAGS := -s -w -X main.version=$(VERSION) -X main.commit=$(COMMIT) -X main.date=$(DATE)

    all: build

    build:
	mkdir -p $(BIN_DIR)
	GO111MODULE=on go build -ldflags="$(LDFLAGS)" -o $(BIN_DIR)/opx-authd ./cmd/opx-authd
	GO111MODULE=on go build -ldflags="$(LDFLAGS)" -o $(BIN_DIR)/opx ./cmd/opx

    run:
	./bin/opx-authd --verbose

    clean:
	rm -rf $(BIN_DIR)
