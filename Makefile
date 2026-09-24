.PHONY: all deps build server cli gui lint test clean

BIN = binaries

all: build

deps:
	go mod download
	go mod tidy

build:
	go build -o $(BIN)/server ./cmd/server
	go build -o $(BIN)/cli    ./cmd/cli
	go build -o $(BIN)/gui    ./cmd/gui

server:
	go run ./cmd/server

cli:
	go run ./cmd/cli

gui:
	go run ./cmd/gui

lint:
	gofmt -l .
	go vet ./...

test:
	go test ./...

clean:
	rm -rf $(BIN)
	go clean