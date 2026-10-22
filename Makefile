.PHONY: build lint test check golden agent-check fuzz e2e ci-check completion-perf docs docs-check

build:
	CGO_ENABLED=0 go build -trimpath .

lint:
	golangci-lint run --config .golangci.yml

test: agent-check
	go test -race -coverpkg=./... -coverprofile=coverage.out ./...
	python3 -B .github/scripts/check_coverage.py < coverage.out
	$(MAKE) completion-perf

completion-perf:
	GOFLAGS= go test -race=false -cover=false -tags=completionperf -run '^TestCompletion500SubscriptionsIsOfflineAndFast$$' -count=1 -v ./cmd

check: lint test

docs:
	go run ./tools/gendocs

docs-check:
	go run ./tools/gendocs -check

agent-check:
	python3 -B -m unittest discover -s plugins/azctx/tests -p 'test_*.py' -v

ci-check:
	python3 -B -m unittest discover -s .github/scripts -p 'test_*.py' -v
	ruby .github/scripts/test_ci_config.rb
	ruby .github/scripts/test_integration_config.rb
	ruby .github/scripts/test_release_config.rb
	ruby .github/scripts/test_publish_winget.rb
	ruby .github/scripts/test_docs_config.rb

e2e:
	go test ./cmd -run '^TestBinary' -count=1

golden:
	go test ./internal/ui -count=1 -update

fuzz:
	go test ./internal/profile -run '^$$' -fuzz '^FuzzProfile$$' -fuzztime=60s
	go test ./internal/config -run '^$$' -fuzz '^FuzzConfig$$' -fuzztime=60s
	go test ./internal/project -run '^$$' -fuzz '^FuzzProject$$' -fuzztime=60s
