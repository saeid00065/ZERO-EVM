BINARY=zeroevm
BUILD_DIR=./build
MAIN=./cmd/zeroevm/main.go

.PHONY: build install init start clean test

## build - کامپایل کردن binary
build:
	@echo "Building $(BINARY)..."
	@go build -o $(BUILD_DIR)/$(BINARY) $(MAIN)
	@echo "✓ Built: $(BUILD_DIR)/$(BINARY)"

## install - نصب binary توی PATH
install:
	@go install $(MAIN)
	@echo "✓ Installed $(BINARY)"

## init - راه‌اندازی اولیه chain
init: build
	@chmod +x ./scripts/init-chain.sh
	@./scripts/init-chain.sh

## start - شروع node
start: build
	@$(BUILD_DIR)/$(BINARY) start \
		--json-rpc.enable \
		--json-rpc.address "0.0.0.0:8545" \
		--json-rpc.ws-address "0.0.0.0:8546" \
		--json-rpc.api "eth,net,web3,txpool,debug"

## clean - پاک کردن build
clean:
	@rm -rf $(BUILD_DIR)
	@echo "✓ Cleaned"

## test - اجرای تست‌ها
test:
	@go test ./... -v

## tidy - آپدیت dependencies
tidy:
	@go mod tidy

## help - نمایش دستورها
help:
	@echo "ZERO-EVM Commands:"
	@echo "  make build   - Build the binary"
	@echo "  make init    - Initialize chain (run once)"
	@echo "  make start   - Start the node"
	@echo "  make test    - Run tests"
	@echo "  make clean   - Clean build files"
