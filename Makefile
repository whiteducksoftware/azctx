.PHONY: build lint test check golden agent-check fuzz

build:
	CGO_ENABLED=0 go build -trimpath .

lint:
	golangci-lint run --config .golangci.yml

test: agent-check
	go test -race -cover ./...

check: lint test

agent-check:
	python3 -B -m unittest discover -s plugins/azctx/tests -p 'test_*.py' -v

golden:
	go test ./internal/ui -count=1 -update

fuzz:
	go test ./internal/profile -run '^$$' -fuzz '^FuzzProfile$$' -fuzztime=60s
	go test ./internal/config -run '^$$' -fuzz '^FuzzConfig$$' -fuzztime=60s
	go test ./internal/project -run '^$$' -fuzz '^FuzzProject$$' -fuzztime=60s
