BINARY_NAME=pgwire-mock
BUILD_DIR=bin
GO_CMD=go

.PHONY: all build test clean run lint docker-build

all: build test

build:
	@mkdir -p $(BUILD_DIR)
	$(GO_CMD) build -v -o $(BUILD_DIR)/$(BINARY_NAME) ./cmd/pgwire-mock

test:
	$(GO_CMD) test -v -race -cover ./...

coverage:
	$(GO_CMD) test -coverprofile=coverage.txt -covermode=atomic ./...
	$(GO_CMD) tool cover -html=coverage.txt -o coverage.html

run: build
	./$(BUILD_DIR)/$(BINARY_NAME) --port 5432 --rules examples/rules_ecommerce.yaml -v

clean:
	@rm -rf $(BUILD_DIR) coverage.txt coverage.html

lint:
	$(GO_CMD) vet ./...

docker-build:
	docker build -t pgwire-mock:latest .
