.PHONY: all deps build run-server run-client run-client-gui lint test clean

BIN = binaries

all: build

deps:
	go mod download
	go mod tidy

build:
	go build -o $(BIN)/server ./cmd/server
	go build -o $(BIN)/cli    ./cmd/cli
	go build -o $(BIN)/gui    ./cmd/gui

run-server:
	go run ./cmd/server

run-client:
	go run ./cmd/cli

run-client-gui:
	go run ./cmd/gui

lint:
	gofmt -l .
	go vet ./...

test:
	go test ./...

clean:
	rm -rf $(BIN)
	go clean