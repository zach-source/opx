
    BIN_DIR := bin
    NAME := opx
    VERSION ?= 0.1.0

    all: build

    build:
	mkdir -p $(BIN_DIR)
	GO111MODULE=on go build -o $(BIN_DIR)/op-authd ./cmd/op-authd
	GO111MODULE=on go build -o $(BIN_DIR)/opx ./cmd/opx

    run:
	./bin/op-authd --verbose

    clean:
	rm -rf $(BIN_DIR)
