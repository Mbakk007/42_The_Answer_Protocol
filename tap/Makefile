.PHONY: all build run-server run-client run-client-gui lint clean

all: build

build:
	go build -o bin/server ./server/
	go build -o bin/client-cli ./client-cli/
	go build -o bin/client-gui ./client-gui/

run-server:
	go run ./server/main.go

run-client:
	go run ./client-cli/main.go

run-client-gui:
	go run ./client-gui/main.go

lint:
	go vet ./...

clean:
	rm -rf bin/