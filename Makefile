.PHONY: build lint test check golden agent-check fuzz e2e ci-check completion-perf docs docs-check live-test

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
	go run ./internal/cmd/gendocs

docs-check:
	go run ./internal/cmd/gendocs -check

agent-check:
	python3 -B -m unittest discover -s plugins/azctx/tests -p 'test_*.py' -v

ci-check:
	python3 -B -m unittest discover -s .github/scripts -p 'test_*.py' -v
	python3 -B -m unittest discover -s scripts -p 'test_*.py' -v
	ruby .github/scripts/test_ci_config.rb
	ruby .github/scripts/test_live_azure_policy.rb
	ruby .github/scripts/test_release_config.rb
	ruby .github/scripts/test_publish_winget.rb
	ruby .github/scripts/test_docs_config.rb

# Manual maintainer check against a dedicated test tenant; never run in CI.
# Example: make live-test LIVE_TEST_ARGS='--opt-in-live-azure --tenant T --subscription S1 --subscription S2'
live-test: build
	python3 -B scripts/live-azure-test.py --binary ./azctx$(shell go env GOEXE) $(LIVE_TEST_ARGS)

e2e:
	go test ./cmd -run '^TestBinary' -count=1

golden:
	go test ./internal/ui -count=1 -update

fuzz:
	go test ./internal/profile -run '^$$' -fuzz '^FuzzProfile$$' -fuzztime=60s
	go test ./internal/config -run '^$$' -fuzz '^FuzzConfig$$' -fuzztime=60s
	go test ./internal/project -run '^$$' -fuzz '^FuzzProject$$' -fuzztime=60s
