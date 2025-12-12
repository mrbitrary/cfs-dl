BINARY_NAME=cfs-dl
BUILD_DIR=bin
CMD_PATH=cmd/cfs-dl/main.go

.PHONY: all build test coverage clean

all: build

build:
	mkdir -p $(BUILD_DIR)
	go build -o $(BUILD_DIR)/$(BINARY_NAME) $(CMD_PATH)

test:
	go test -v ./...

coverage:
	go test -coverprofile=coverage.out ./...
	go tool cover -func=coverage.out

clean:
	rm -rf $(BUILD_DIR) coverage.out
