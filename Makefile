.PHONY: build test lint golden server repl

build:
	go build -o bin/pluto ./cmd/pluto
	go build -o bin/userserver ./examples/userserver

test:
	go test -race ./...

lint:
	golangci-lint run ./...

golden:
	go test ./internal/adapter/cli/ -update

server: build
	./bin/userserver -addr localhost:8080 -proto testdata/proto

repl: build
	./bin/pluto --schema testdata/proto --target http://localhost:8080 repl
