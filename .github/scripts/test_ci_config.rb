require 'yaml'
require 'minitest/autorun'

ROOT = File.expand_path('../..', __dir__)
Dir.chdir(ROOT)

module CIConfigPolicy
  module_function

  def assert(condition, message)
    raise message unless condition
  end

  def action_pin(reference, source, path)
    return [reference, nil, nil] if reference.start_with?('./')

    action, sha = reference.split('@', 2)
    definition = /^\s*(?:-\s+)?uses: #{Regexp.escape(reference)}/
    comments = source.lines.grep(/#{definition}(?:\s|$)/).map do |line|
      line[/#{definition} # (v\d+\.\d+\.\d+)\s*$/, 1]
    end.uniq
    assert(sha&.match?(/\A[0-9a-f]{40}\z/) && comments.length == 1 && comments.first,
           "#{path}: #{action} needs an immutable SHA and exact stable release tag")
    [action, sha, comments.first]
  end
end

include CIConfigPolicy

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
action_references = {}
documents.select { |path, _| path.start_with?('.github/workflows/') }.each do |path, workflow|
  source = File.read(path)
  workflow.fetch('jobs').each_value do |job|
    job.fetch('steps').each do |step|
      next unless step.key?('uses')

      reference = step.fetch('uses')
      action, sha, tag = action_pin(reference, source, path)
      action_references[reference] = [action, sha, tag]
      next if sha.nil? # Local actions use the tested repository checkout.

      pin = [sha, tag]
      assert(!action_pins.key?(action) || action_pins[action] == pin,
             "#{action} SHA/tag pins must agree across every workflow")
      action_pins[action] = pin
    end
  end
end

ci = documents.fetch('.github/workflows/ci.yaml')
release = documents.fetch('.github/workflows/goreleaser.yaml')
rules = documents.fetch('.github/zizmor.yml').fetch('rules')

maintained_actions = {
  'step-security/harden-runner' => [[2, 16, 0], 'f4a75cfd619ee5ce8d5b864b0d183aff3c69b55a'],
  'actions/checkout' => [[5, 0, 0], '11bd71901bbe5b1630ceea73d27597364c9af683'],
  'actions/setup-go' => [[6, 0, 0], 'f111f3307d8850f501ac008e886eec1fd1932a34'],
  'goreleaser/goreleaser-action' => [[7, 3, 0], '90a3faa9d0182683851fbfa97ca1a2cb983bfca3']
}
%w[ci.yaml scorecard.yaml].each do |file|
  path = ".github/workflows/#{file}"
  workflow = documents.fetch(path)
  assert(workflow.fetch('permissions') == {}, "#{file} must deny workflow-level permissions")
  workflow.fetch('jobs').each do |name, job|
    steps = job.fetch('steps')
    first = steps.first
    assert(first.fetch('uses', '').start_with?('step-security/harden-runner@') &&
           first.fetch('with').fetch('egress-policy') == 'audit',
           "#{file}/#{name} must start with harden-runner in audit mode")
    steps.each do |step|
      action, sha, tag = action_references[step['uses']]
      next unless maintained_actions.key?(action)

      minimum, retired_sha = maintained_actions.fetch(action)
      assert(sha != retired_sha,
             "#{file}/#{name} must replace the retired #{action} SHA with an immutable pin")
      version = tag.delete_prefix('v').split('.').map(&:to_i)
      assert((version <=> minimum) >= 0, "#{action} must use a patched, Node24-native release")
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
       ci.fetch('jobs').fetch('test').fetch('steps').count { |step| step['run'] == 'make test' } == 1,
       'all three native platforms must run correctness, binary E2E and completion acceptance once via make test')
assert(ci.fetch('jobs').values.none? { |job| job.fetch('steps').any? { |step| step.fetch('run', '').match?(/\bmake\s+e2e\b/) } },
       'normal CI must not rerun the binary E2E subset already included in make test')
assert(makefile.include?("e2e:\n\tgo test ./cmd -run '^TestBinary' -count=1"),
       'make e2e must remain available as a standalone developer subset')

module CIConfigPolicy
  def self.validation_artifacts(ci)
    native_test = ci.fetch('jobs').fetch('test')
    native_steps = native_test.fetch('steps')
    assert(native_test.fetch('permissions') == { 'contents' => 'read' },
           'validation artifacts must reuse native tests without publishing permissions')
    checkouts = native_steps.select { |step| step.fetch('uses', '').start_with?('actions/checkout@') }
    assert(checkouts.length == 1, 'native validation must use one tested checkout')
    checkout = checkouts.first
    options = checkout.fetch('with')
    assert(options['persist-credentials'] == false &&
           [nil, '${{ github.sha }}'].include?(options['ref']) &&
           [nil, '${{ github.repository }}'].include?(options['repository']) &&
           [nil, '.'].include?(options['path']),
           'validation must build the tested repository/revision without persisting credentials')
    tests = native_steps.select { |step| ['make test', 'make check'].include?(step['run']) }
    assert(tests.length == 1 && !tests.first.key?('if') && !tests.first.fetch('continue-on-error', false),
           'native tests must pass before building validation artifacts')
    uploads = native_steps.select { |step| step.fetch('uses', '').start_with?('actions/upload-artifact@') }
    coverage = uploads.find { |step| step['name'] == 'Publish coverage report' }
    coverage_options = {
      'name' => 'coverage-${{ matrix.os }}', 'path' => 'coverage.out',
      'if-no-files-found' => 'error', 'retention-days' => 7
    }
    assert(coverage && coverage_options.all? { |key, value| coverage.fetch('with')[key] == value },
           'native tests must retain the per-OS coverage profile, retention and missing-file failure')
    builds = native_steps.select { |step| step['run'] == 'python3 -B .github/scripts/build_validation_artifact.py' }
    assert(builds.length == 1 && builds.first['id'] == 'validation-artifact' &&
           builds.first['shell'] == 'bash' && !builds.first.key?('if') &&
           !builds.first.fetch('continue-on-error', false),
           'each native test must build and smoke-test one validation binary, failing closed')
    binary_upload = uploads.find { |step| step['name'] == 'Upload native validation artifact' }
    binary_options = {
      'name' => '${{ steps.validation-artifact.outputs.artifact-name }}',
      'path' => '${{ steps.validation-artifact.outputs.archive-path }}',
      'if-no-files-found' => 'error', 'retention-days' => 14
    }
    assert(binary_upload && binary_upload.fetch('uses') == coverage.fetch('uses') &&
           binary_options.all? { |key, value| binary_upload.fetch('with')[key] == value } &&
           !binary_upload.key?('if') && !binary_upload.fetch('continue-on-error', false),
           'upload only the exact binary archive with the shared SHA pin, bounded retention and missing-file failure')
    assert(native_steps.index(checkout) < native_steps.index(tests.first) &&
           native_steps.index(tests.first) < native_steps.index(coverage) &&
           native_steps.index(tests.first) < native_steps.index(builds.first) &&
           native_steps.index(builds.first) < native_steps.index(binary_upload),
           'validation must check out, test, build and then upload the tested revision')
  end
end

CIConfigPolicy.validation_artifacts(ci)

puts 'PASS: YAML; Dependabot coverage/groups; all-workflow SHA/exact-tag pins; audit/checkout protections; publisher-disabled snapshots; no release scanner exceptions; toolchain diagnostics; scanners; explicit strict linters/assertions; three 60s fuzzers; reproducible build flags; hook coverage; instrumented correctness, weighted coverage and native completion acceptance; native validation archives and unchanged coverage uploads'

class CIConfigPolicyTest < Minitest::Test
  def native_ci
    YAML.safe_load(File.read('.github/workflows/ci.yaml'), aliases: false)
  end

  def test_artifact_policy_allows_unrelated_jobs_checkout_options_and_uploads
    ci = native_ci
    ci.fetch('jobs')['unrelated'] = {}
    steps = ci.fetch('jobs').fetch('test').fetch('steps')
    checkout = steps.find { |step| step.fetch('uses', '').start_with?('actions/checkout@') }
    checkout.fetch('with').merge!('fetch-depth' => 0, 'show-progress' => false, 'ref' => '${{ github.sha }}')
    upload = steps.find { |step| step['name'] == 'Upload native validation artifact' }
    upload.fetch('with')['compression-level'] = 0
    steps << { 'uses' => upload.fetch('uses'), 'with' => { 'name' => 'unrelated', 'path' => 'other.txt' } }
    CIConfigPolicy.validation_artifacts(ci)
  end

  def test_artifact_policy_rejects_unsafe_revision_permissions_options_and_order
    mutations = [
      ->(job, _) { job['permissions']['contents'] = 'write' },
      ->(_, steps) { steps.find { |s| s['name'] == 'Checkout' }['with']['persist-credentials'] = true },
      ->(_, steps) { steps.find { |s| s['name'] == 'Checkout' }['with']['ref'] = '${{ github.event.pull_request.head.sha }}' },
      ->(_, steps) { steps.find { |s| s['name'] == 'Checkout' }['with']['repository'] = 'other/repo' },
      ->(_, steps) { steps.find { |s| s['name'] == 'Checkout' }['with']['path'] = 'other' },
      ->(_, steps) { steps.find { |s| s['run'] == 'make test' }['continue-on-error'] = true },
      ->(_, steps) { steps.unshift(steps.delete(steps.find { |s| s['id'] == 'validation-artifact' })) },
      ->(_, steps) { steps.find { |s| s['id'] == 'validation-artifact' }['if'] = 'always()' },
      ->(_, steps) { steps.find { |s| s['id'] == 'validation-artifact' }['continue-on-error'] = true },
      ->(_, steps) { steps.find { |s| s['name'] == 'Publish coverage report' }['with']['path'] = 'other.out' },
      ->(_, steps) { steps.find { |s| s['name'] == 'Publish coverage report' }['with']['retention-days'] = 30 },
      ->(_, steps) { steps.unshift(steps.delete(steps.find { |s| s['name'] == 'Upload native validation artifact' })) },
      ->(_, steps) { steps.find { |s| s['name'] == 'Upload native validation artifact' }['with']['path'] = 'dist/**' },
      ->(_, steps) { steps.find { |s| s['name'] == 'Upload native validation artifact' }['with']['retention-days'] = 90 },
      ->(_, steps) { steps.find { |s| s['name'] == 'Upload native validation artifact' }['with']['if-no-files-found'] = 'warn' },
      ->(_, steps) { steps.find { |s| s['name'] == 'Upload native validation artifact' }['continue-on-error'] = true }
    ]
    mutations.each_with_index do |mutate, index|
      ci = native_ci
      job = ci.fetch('jobs').fetch('test')
      mutate.call(job, job.fetch('steps'))
      assert_raises(RuntimeError, "mutation #{index}") { CIConfigPolicy.validation_artifacts(ci) }
    end
  end

  def test_action_pin_parses_remote_pins_and_allows_checkout_local_actions
    reference = "actions/example@#{'a' * 40}"
    assert_equal ['actions/example', 'a' * 40, 'v1.2.3'],
                 CIConfigPolicy.action_pin(reference, "  uses: #{reference} # v1.2.3\n", 'fixture')
    assert_equal ['actions/example', 'a' * 40, 'v1.2.3'],
                 CIConfigPolicy.action_pin(reference, "- uses: #{reference} # v1.2.3\n", 'fixture')
    assert_equal ['./.github/actions/example', nil, nil],
                 CIConfigPolicy.action_pin('./.github/actions/example', '', 'fixture')
  end

  def test_action_pin_rejects_mutable_unannotated_or_conflicting_pins
    reference = "actions/example@#{'a' * 40}"
    [
      ['actions/example@v1', 'uses: actions/example@v1 # v1.2.3'],
      ["actions/example@#{'A' * 40}", "uses: actions/example@#{'A' * 40} # v1.2.3"],
      [reference, "uses: #{reference}"],
      [reference, "uses: #{reference} # v1"],
      [reference, "uses: #{reference} # v1.2.3-rc.1"],
      [reference, "uses: #{reference} # v1.2.3\nuses: #{reference}\n"],
      [reference, "uses: #{reference} # v1.2.3\nuses: #{reference} # v1.2.4\n"]
    ].each do |uses, source|
      assert_raises(RuntimeError, source) { CIConfigPolicy.action_pin(uses, source, 'fixture') }
    end
  end
end
