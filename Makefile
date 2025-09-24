## Simple Makefile for swagger2mcp repo

.PHONY: help build test e2e e2e-online fmt tidy run generate-go-sample generate-npm-sample clean clean-samples

BIN_DIR ?= bin
OUT_GO ?= tmp/out-go
OUT_NPM ?= tmp/out-npm

help:
	@echo "Targets: build test e2e e2e-online fmt tidy run generate-go-sample generate-npm-sample clean clean-samples"

build:
	@mkdir -p $(BIN_DIR)
	go build -o $(BIN_DIR)/swagger2mcp ./cmd/swagger2mcp

test:
	go test ./...

e2e:
	go test ./internal/e2e -run E2E -v

e2e-online:
	SWAGGER2MCP_E2E_ONLINE=1 go test ./internal/e2e -run E2E -v

fmt:
	go fmt ./...

tidy:
	go mod tidy

run:
	go run ./cmd/swagger2mcp --help

generate-go-sample:
	go run ./cmd/swagger2mcp generate --input testdata/swagger.yaml --lang go --out $(OUT_GO) --force

generate-npm-sample:
	go run ./cmd/swagger2mcp generate --input testdata/swagger.yaml --lang npm --out $(OUT_NPM) --force

clean:
	rm -rf $(BIN_DIR)

clean-samples:
	rm -rf $(OUT_GO) $(OUT_NPM)

