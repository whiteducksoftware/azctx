.PHONY: build lint test check golden agent-check fuzz e2e ci-check

build:
	CGO_ENABLED=0 go build -trimpath .

lint:
	golangci-lint run --config .golangci.yml

test: agent-check
	go test -race -coverpkg=./... -coverprofile=coverage.out ./...
	python3 -B .github/scripts/check_coverage.py < coverage.out

check: lint test

agent-check:
	python3 -B -m unittest discover -s plugins/azctx/tests -p 'test_*.py' -v

ci-check:
	python3 -B -m unittest discover -s .github/scripts -p 'test_*.py' -v
	ruby .github/scripts/test_ci_config.rb
	ruby .github/scripts/test_integration_config.rb

e2e:
	go test ./cmd -run '^TestBinary' -count=1

golden:
	go test ./internal/ui -count=1 -update

fuzz:
	go test ./internal/profile -run '^$$' -fuzz '^FuzzProfile$$' -fuzztime=60s
	go test ./internal/config -run '^$$' -fuzz '^FuzzConfig$$' -fuzztime=60s
	go test ./internal/project -run '^$$' -fuzz '^FuzzProject$$' -fuzztime=60s
