require 'yaml'
require 'json'

ROOT = File.expand_path('../..', __dir__)
Dir.chdir(ROOT)

def assert(condition, message)
  raise message unless condition
end

documents = Dir.glob('.github/**/*.{yaml,yml}').to_h do |path|
  document = YAML.safe_load(File.read(path), aliases: false)
  assert(document.is_a?(Hash), "#{path} must be a YAML mapping")
  [path, document]
end

dependabot = documents.fetch('.github/dependabot.yaml')
assert(dependabot.fetch('version') == 2, 'Dependabot must use the supported configuration schema')
updates = dependabot.fetch('updates')
assert(updates.map { |update| update.fetch('package-ecosystem') }.sort == %w[github-actions gomod],
       'Dependabot must cover the real Go module and workflow manifests, not invented ecosystems')
updates.each do |update|
  ecosystem = update.fetch('package-ecosystem')
  assert(update.fetch('directory') == '/' && update.fetch('schedule').fetch('interval') == 'weekly',
         "#{ecosystem} must maintain the root manifests weekly")
  assert(update.fetch('allow') == [{ 'dependency-type' => 'all' }],
         "#{ecosystem} must explicitly include direct and indirect dependencies")
  assert(update.fetch('open-pull-requests-limit') >= 10 && !update.key?('cooldown') &&
         !update.key?('ignore') && !update.key?('exclude-paths'),
         "#{ecosystem} must not starve updates or exclude major releases")
  groups = update.fetch('groups').values
  assert(groups.length == 1 && groups.first.fetch('patterns') == ['*'] &&
         groups.first.fetch('update-types').sort == %w[minor patch],
         "#{ecosystem} must group minor/patch updates and leave majors individually reviewable")
end

action_pins = {}
tools = JSON.parse(File.read('.github/tool-pins.json')).fetch('tools')
go_pin = tools.find { |tool| tool['name'] == 'go' }
assert(go_pin && go_pin.fetch('version') == File.read('go.mod')[/^toolchain (go\S+)$/, 1] &&
       go_pin.fetch('release_feed') == 'https://go.dev/dl/?mode=json' &&
       go_pin.fetch('sources').any? { |source| source['path'] == 'go.mod' },
       'the preferred Go runtime must be covered separately from the historical minimum')
tool_actions = tools.select { |tool| tool.key?('action') }.to_h { |tool| [tool.fetch('action'), tool] }
tool_installs = tools.select { |tool| tool.key?('go-install') }.to_h { |tool| [tool.fetch('go-install'), tool] }
documents.select { |path, _| path.start_with?('.github/workflows/') }.each do |path, workflow|
  source = File.read(path)
  workflow.fetch('jobs').each_value do |job|
    job.fetch('steps').each do |step|
      step.fetch('run', '').scan(/\bgo install ([^\s@]+)@([^\s]+)/).each do |package, version|
        tool = tool_installs.fetch(package)
        assert(tool.fetch('version') == version && tool.fetch('sources').any? { |entry| entry['path'] == path },
               "#{path}: installed #{package} must be covered by the real pin inventory")
      end
      next unless step.key?('uses')

      action, sha = step.fetch('uses').split('@', 2)
      comment = source.match(/^\s*uses: #{Regexp.escape(step.fetch('uses'))} # (v\d+\.\d+\.\d+)\s*$/)
      assert(sha&.match?(/\A[0-9a-f]{40}\z/) && comment,
             "#{path}: #{action} needs an immutable SHA and exact stable release tag")
      pin = [sha, comment[1]]
      assert(!action_pins.key?(action) || action_pins[action] == pin,
             "#{action} SHA/tag pins must agree across every workflow")
      action_pins[action] = pin
      step.fetch('with', {}).each do |input, version|
        next unless input == 'version' || input.end_with?('-version')

        tool = tool_actions.fetch(action)
        assert(tool.fetch('input') == input && tool.fetch('version') == version &&
               tool.fetch('sources').any? { |entry| entry['path'] == path },
               "#{path}: #{action} #{input} must be covered by the real pin inventory")
      end
    end
  end
end

ci = documents.fetch('.github/workflows/ci.yaml')
freshness = documents.fetch('.github/workflows/dependency-freshness.yaml')
triggers = freshness.fetch('on') { freshness.fetch(true) }
assert(triggers.keys.sort == %w[schedule workflow_dispatch] &&
       triggers.fetch('schedule') == [{ 'cron' => '31 7 * * 1' }],
       'online freshness must be scheduled weekly or manually invoked, never an online PR gate')
assert(freshness.fetch('permissions') == {} && freshness.fetch('jobs').keys == ['tool-pins'],
       'freshness must deny workflow permissions and have only the read-only tool check')
freshness_job = freshness.fetch('jobs').fetch('tool-pins')
assert(freshness_job.fetch('permissions') == { 'contents' => 'read' } &&
       freshness_job.fetch('timeout-minutes') <= 15 && !freshness_job.key?('continue-on-error'),
       'freshness needs only bounded read access and must fail on errors or available updates')
freshness_steps = freshness_job.fetch('steps')
assert(freshness_steps.length == 3 &&
       freshness_steps[0].fetch('uses').start_with?('step-security/harden-runner@') &&
       freshness_steps[0].fetch('with') == { 'egress-policy' => 'audit' } &&
       freshness_steps[1].fetch('uses').start_with?('actions/checkout@') &&
       freshness_steps[1].fetch('with') == { 'persist-credentials' => false } &&
       freshness_steps[2].fetch('run') == 'python3 -B .github/scripts/check_tool_pins.py --online' &&
       freshness_steps[2].fetch('env') == { 'GH_TOKEN' => '${{ github.token }}' } &&
       freshness_steps.none? { |step| step.key?('if') || step.key?('continue-on-error') },
       'freshness must only harden, check out without credentials and report upstream tool updates')
assert(File.read('Makefile').include?("\tpython3 -B .github/scripts/check_tool_pins.py\n"),
       'ordinary CI must check inventory source consistency offline')
release = documents.fetch('.github/workflows/goreleaser.yaml')
rules = documents.fetch('.github/zizmor.yml').fetch('rules')

maintained_actions = {
  'step-security/harden-runner' => [[2, 16, 0], 'f4a75cfd619ee5ce8d5b864b0d183aff3c69b55a'],
  'actions/checkout' => [[5, 0, 0], '11bd71901bbe5b1630ceea73d27597364c9af683'],
  'actions/setup-go' => [[6, 0, 0], 'f111f3307d8850f501ac008e886eec1fd1932a34'],
  'goreleaser/goreleaser-action' => [[7, 3, 0], '90a3faa9d0182683851fbfa97ca1a2cb983bfca3']
}
shared_pins = {}
%w[ci.yaml scorecard.yaml].each do |file|
  path = ".github/workflows/#{file}"
  source = File.read(path)
  workflow = documents.fetch(path)
  assert(workflow.fetch('permissions') == {}, "#{file} must deny workflow-level permissions")
  workflow.fetch('jobs').each do |name, job|
    steps = job.fetch('steps')
    first = steps.first
    assert(first.fetch('uses', '').start_with?('step-security/harden-runner@') &&
           first.fetch('with').fetch('egress-policy') == 'audit',
           "#{file}/#{name} must start with harden-runner in audit mode")
    steps.each do |step|
      action, sha = step.fetch('uses', '').split('@', 2)
      next unless maintained_actions.key?(action)

      minimum, retired_sha = maintained_actions.fetch(action)
      assert(sha.match?(/\A[0-9a-f]{40}\z/) && sha != retired_sha,
             "#{file}/#{name} must replace the retired #{action} SHA with an immutable pin")
      comment = source.match(/^\s*uses: #{Regexp.escape(step.fetch('uses'))} # v(\d+\.\d+\.\d+)$/)
      assert(!comment.nil?, "#{action} must have an exact release-version comment")
      version = comment[1].split('.').map(&:to_i)
      assert((version <=> minimum) >= 0, "#{action} must use a patched, Node24-native release")
      assert(!shared_pins.key?(action) || shared_pins[action] == sha,
             "#{action} pins must agree across CI and Scorecard")
      shared_pins[action] = sha
      if action == 'actions/checkout'
        assert(step.fetch('with').fetch('persist-credentials') == false,
               "#{file}/#{name} must not persist checkout credentials")
      end
    end
  end
end

%w[ci-linux ci-windows].each do |name|
  steps = ci.fetch('jobs').fetch(name).fetch('steps')
  setup = steps.select { |step| step.fetch('uses', '').start_with?('actions/setup-go@') }
  assert(setup.length == 1, "#{name} must set up Go once")
  assert(setup.first.fetch('with').fetch('cache', true) == false,
         "#{name} must explicitly disable setup-go caching")
  publisher = steps.find { |step| step.fetch('uses', '').start_with?('goreleaser/goreleaser-action@') }
  args = publisher.fetch('with').fetch('args')
  assert(args.include?('--snapshot') &&
         args.include?('--skip publish,announce,archive,sbom,homebrew,chocolatey,winget'),
         "#{name} must remain a publisher-disabled snapshot")
end

assert(rules == {}, 'release hardening must remove all scanner-debt exceptions')
assert(release.fetch('permissions') == {}, 'release must deny workflow-level permissions')
release.fetch('jobs').each do |name, job|
  job.fetch('steps').each do |step|
    if step.fetch('uses', '').start_with?('actions/setup-go@')
      assert(step.fetch('with').fetch('cache') == false, "#{name} must not write release caches")
    elsif step.fetch('uses', '').start_with?('actions/checkout@')
      assert(step.fetch('with').fetch('persist-credentials') == false, "#{name} must not persist credentials")
    end
  end
end

assert([nil, 'auto'].include?(ci.fetch('env', {})['GOTOOLCHAIN']),
       'CI must permit automatic selection of the go.mod toolchain')
ci.fetch('jobs').each do |name, job|
  steps = job.fetch('steps')
  assert([nil, 'auto'].include?(job.fetch('env', {})['GOTOOLCHAIN']),
         "#{name} must permit automatic toolchain selection")
  setup_index = steps.index { |step| step.fetch('uses', '').start_with?('actions/setup-go@') }
  assert(!setup_index.nil?, "#{name} must set up Go")
  setup = steps.fetch(setup_index).fetch('with')
  assert(setup['go-version-file'] == 'go.mod' && !setup.key?('go-version'),
         "#{name} must obtain its Go version from go.mod, not a global version override")
  sanity = steps.fetch(setup_index + 1)
  assert(sanity['name'] == 'Check active Go toolchain' && sanity['shell'] == 'bash',
         "#{name} must check the active toolchain immediately after setup on every platform")
  run = sanity.fetch('run')
  assert(run.include?('go version') && run.include?('go env GOTOOLCHAIN') &&
         run.include?('toolchain') && run.include?('go.mod'),
         "#{name} must log the active Go version, selection mode and preferred toolchain")
  steps.each do |step|
    assert([nil, 'auto'].include?(step.fetch('env', {})['GOTOOLCHAIN']),
           "#{name} steps must permit automatic toolchain selection")
  end
end

go_mod = File.read('go.mod')
assert(go_mod.match?(/^go 1\.26(?:\.0)?$/), 'the minimum Go version must remain 1.26')
assert(go_mod.match?(/^toolchain go1\.27\.3$/),
       'the preferred toolchain must include the GO-2026-6809 fix in Go 1.27.3')
lint_steps = ci.fetch('jobs').fetch('lint').fetch('steps')
assert(lint_steps.any? { |step| step['run'] == 'make ci-check' },
       'CI must run these configuration regressions')
audit = lint_steps.find { |step| step.fetch('uses', '').start_with?('zizmorcore/zizmor-action@') }
assert(audit.fetch('with').fetch('inputs') == '.github/workflows' &&
       audit.fetch('with').fetch('config') == '.github/zizmor.yml' &&
       !audit.fetch('with').key?('min-confidence') &&
       !audit.fetch('with').key?('min-severity'),
       'zizmor must scan every workflow without hiding findings by confidence or severity')

lint = YAML.safe_load(File.read('.golangci.yml')).fetch('linters')
%w[errcheck govet revive gocritic nilerr errorlint].each do |name|
  assert(lint.fetch('enable').include?(name), "#{name} must be explicitly enabled")
end
assert(lint.fetch('settings').fetch('errorlint').fetch('asserts') == true,
       'errorlint must check assertions')
assert(lint.fetch('settings').fetch('errcheck').fetch('check-type-assertions') == true,
       'errcheck must check type assertion failures')
%w[.goreleaser.yaml .goreleaser-windows.yaml].each do |path|
  config = YAML.safe_load(File.read(path))
  config.fetch('builds').each do |build|
    assert(build.fetch('env').include?('CGO_ENABLED=0') &&
           build.fetch('flags').include?('-trimpath') &&
           build.fetch('mod_timestamp') == '{{ .CommitTimestamp }}',
           "#{path} must build private-path-free static binaries with a commit timestamp")
    assert(build.fetch('ldflags').any? { |flag| flag.include?('BuildDate={{.CommitDate}}') },
           "#{path} must not embed the wall-clock build date")
  end
end
security_steps = ci.fetch('jobs').fetch('security').fetch('steps')
assert(security_steps.any? { |step| step['run'] == 'make fuzz' },
       'CI must exercise all three fuzzers')
makefile = File.read('Makefile')
%w[Profile Config Project].zip(%w[profile config project]).each do |name, package|
  assert(makefile.include?("go test ./internal/#{package} -run '^$$' -fuzz '^Fuzz#{name}$$' -fuzztime=60s"),
         "#{name} must get its own 60-second fuzz run")
end
assert(makefile.include?("CGO_ENABLED=0 go build -trimpath ."), 'make build must use the hardened build flags')
assert(makefile.include?("test: agent-check"), 'make check/test must retain hook coverage')
test_recipe = makefile[/^test:.*\n((?:\t.*\n)+)/, 1]
assert(test_recipe.include?("go test -race -coverpkg=./... -coverprofile=coverage.out ./...") &&
       test_recipe.include?("python3 -B .github/scripts/check_coverage.py < coverage.out") &&
       test_recipe.include?("$(MAKE) completion-perf"),
       'make test must retain full instrumentation, the weighted coverage gate and native completion acceptance')
assert(makefile.include?("GOFLAGS= go test -race=false -cover=false -tags=completionperf " \
                        "-run '^TestCompletion500SubscriptionsIsOfflineAndFast$$' -count=1 -v ./cmd"),
       'completion acceptance must run once, uncached and without inherited instrumentation')
assert(ci.fetch('jobs').fetch('test').fetch('strategy').fetch('matrix').fetch('os').sort ==
       %w[macos-latest ubuntu-latest windows-latest] &&
       ci.fetch('jobs').fetch('test').fetch('steps').any? { |step| step['run'] == 'make test' },
       'all three native platforms must run correctness and completion performance acceptance')

puts 'PASS: YAML; Dependabot coverage/groups; all-workflow SHA/exact-tag pins; embedded tool inventory; read-only freshness workflow; audit/checkout protections; publisher-disabled snapshots; no release scanner exceptions; toolchain diagnostics; scanners; explicit strict linters/assertions; three 60s fuzzers; reproducible build flags; hook coverage; instrumented correctness, weighted coverage and native completion acceptance'
