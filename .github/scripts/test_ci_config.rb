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
assert(go_mod.match?(/^go 1\.26$/), 'the minimum Go version must remain 1.26')
assert(go_mod.match?(/^toolchain go1\.27\.3$/),
       'the preferred toolchain must include the GO-2026-6809 fix in Go 1.27.3')
lint_steps = ci.fetch('jobs').fetch('lint').fetch('steps')
assert(lint_steps.any? { |step| step['run'] == 'ruby .github/scripts/test_ci_config.rb' },
       'CI must run these configuration regressions')
audit = lint_steps.find { |step| step.fetch('uses', '').start_with?('zizmorcore/zizmor-action@') }
assert(audit.fetch('with').fetch('inputs') == '.github/workflows' &&
       audit.fetch('with').fetch('config') == '.github/zizmor.yml' &&
       !audit.fetch('with').key?('min-confidence') &&
       !audit.fetch('with').key?('min-severity'),
       'zizmor must scan every workflow without hiding findings by confidence or severity')

puts 'PASS: YAML parses; snapshot caches/publishers disabled; exact release exceptions; automatic toolchain diagnostics; Go 1.26 floor / 1.27.3 security patch; scanning remains enabled'
