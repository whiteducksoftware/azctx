.PHONY: build lint test check golden

build:
	go build .

lint:
	golangci-lint run --config .golangci.yml

test:
	go test -race -cover ./...

check: lint test

golden:
	go test ./internal/ui -count=1 -update
