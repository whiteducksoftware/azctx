require 'yaml'

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

ci = documents.fetch('.github/workflows/ci.yaml')
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

expected_ignores = {
  'artipacked' => %w[goreleaser.yaml:17 goreleaser.yaml:48],
  'excessive-permissions' => %w[goreleaser.yaml:9 goreleaser.yaml:10],
  'cache-poisoning' => %w[goreleaser.yaml:25 goreleaser.yaml:56]
}
assert(rules.keys.sort == expected_ignores.keys.sort, 'only release-debt audit exceptions are permitted')
expected_ignores.each do |rule, locations|
  assert(rules.fetch(rule) == { 'ignore' => locations },
         "#{rule} must have only its exact release-workflow exceptions")
end

release_lines = File.readlines('.github/workflows/goreleaser.yaml')
expected_ignores.fetch('cache-poisoning').each do |location|
  line = location.split(':').last.to_i
  assert(release_lines.fetch(line - 1).include?('- name: Set up Go'),
         "#{location} must still identify the release setup-go step")
end
assert(release.fetch('jobs').values.all? do |job|
  job.fetch('steps').any? { |step| step.fetch('uses', '').start_with?('goreleaser/goreleaser-action@') }
end, 'cache exceptions apply only to the existing release jobs')

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

puts 'PASS: YAML; shared action pins; audit/checkout protections; publisher-disabled snapshots; exact release exceptions; toolchain diagnostics; scanners; explicit strict linters/assertions; three 60s fuzzers; reproducible build flags; hook coverage'
