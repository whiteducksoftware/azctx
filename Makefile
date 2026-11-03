.PHONY: build lint test check

build:
	go build .

lint:
	golangci-lint run --config .golangci.yml

test:
	go test -race -cover ./...

check: lint test
