.PHONY: build lint test check golden agent-check

build:
	go build .

lint:
	golangci-lint run --config .golangci.yml

test: agent-check
	go test -race -cover ./...

check: lint test

agent-check:
	python3 -B -m unittest discover -s plugins/azctx/tests -p 'test_*.py' -v

golden:
	go test ./internal/ui -count=1 -update
