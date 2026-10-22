require 'minitest/autorun'
require 'yaml'

class ReleaseConfigurationTest < Minitest::Test
  ROOT = File.expand_path('../..', __dir__)

  def setup
    @unix = YAML.safe_load(File.read(File.join(ROOT, '.goreleaser.yaml')))
    @windows = YAML.safe_load(File.read(File.join(ROOT, '.goreleaser-windows.yaml')))
  end

  def validate_packages(unix, windows)
    raise 'deprecated formula publisher' if unix.key?('brews')
    cask = unix.fetch('homebrew_casks').fetch(0)
    raise 'cask destination' unless cask.fetch('directory') == 'Casks' &&
                                    cask.fetch('repository').values_at('owner', 'name', 'branch') ==
                                      %w[whiteducksoftware homebrew-tap main]
    raise 'Azure CLI formula dependency' unless cask.fetch('dependencies') == [{ 'formula' => 'azure-cli' }]
    raise 'cask binaries' unless cask.fetch('binaries') == ['azctx']
    raise 'unsigned cask needs declarative quarantine removal' unless
      cask.fetch('custom_block').include?('postflight_steps do') &&
      cask.fetch('custom_block').include?('run "/usr/bin/xattr", args: ["-dr", "com.apple.quarantine",') &&
      !cask.key?('hooks')
    raise 'cask prerelease upload' unless cask.fetch('skip_upload') == 'auto'
    raise 'Linux archives' unless unix.fetch('archives').all? { |archive| archive.fetch('formats') == ['tar.gz'] }
    choco = windows.fetch('chocolateys').fetch(0)
    raise 'Chocolatey ownership' unless choco.values_at('authors', 'owners') == ['white duck GmbH'] * 2
    raise 'Chocolatey dependency floor' unless choco.fetch('dependencies') ==
                                               [{ 'id' => 'azure-cli', 'version' => '2.61.0' }]
    winget = windows.fetch('winget').fetch(0)
    raise 'winget identity' unless winget.fetch('package_identifier') == 'whiteducksoftware.azctx'
    raise 'winget dependency floor' unless winget.fetch('dependencies') ==
                                            [{ 'package_identifier' => 'Microsoft.AzureCLI', 'minimum_version' => '2.61.0' }]
    raise 'winget prerelease upload' unless winget.fetch('skip_upload') == 'auto'
    repo = winget.fetch('repository')
    raise 'manual upstream PR only' unless repo.values_at('owner', 'name', 'branch') ==
                                          ['whiteducksoftware', 'winget-pkgs', 'azctx-{{ .Version }}'] &&
                                          repo.fetch('pull_request').fetch('enabled') == false
    [choco, winget].each do |package|
      raise 'release-year copyright' unless package.fetch('copyright') == 'Copyright {{ .Now.Format "2006" }} white duck GmbH'
    end
    [unix, windows].each do |config|
      raise 'prerelease classification' unless config.fetch('release').fetch('prerelease') == 'auto'
      raise 'latest must be explicitly controlled' unless config.fetch('release').fetch('make_latest') ==
                                                        '{{ .Env.RELEASE_MAKE_LATEST }}'
    end
    raise 'long-lived publisher token' if [unix, windows].to_s.include?('AUTH_GITHUB')
  end

  def test_package_contract
    validate_packages(@unix, @windows)
  end

  def release_workflow
    YAML.safe_load(File.read(File.join(ROOT, '.github/workflows/goreleaser.yaml')))
  end

  def validate_workflow(workflow)
    raise 'workflow permissions' unless workflow.fetch('permissions') == {}
    raise 'release jobs' unless workflow.fetch('jobs').keys.sort == %w[goreleaser-linux goreleaser-windows homebrew]
    workflow.fetch('jobs').each do |name, job|
      raise 'protected environment' unless job.fetch('environment') == 'release'
      validate_step_integrity(job.fetch('steps'))
      if name == 'homebrew'
        validate_homebrew_job(job)
        next
      end
      raise 'release permissions' unless job.fetch('permissions') ==
                                         { 'contents' => 'write', 'id-token' => 'write', 'attestations' => 'write' }
      raise 'latest default' unless job.fetch('env').fetch('RELEASE_MAKE_LATEST') ==
                                    "${{ vars.RELEASE_MAKE_LATEST || 'false' }}"
      steps = job.fetch('steps')
      harden = steps.first
      raise 'runner hardening' unless harden.fetch('uses').start_with?('step-security/harden-runner@')
      if name == 'goreleaser-linux'
        raise 'Linux block egress' unless harden.fetch('with').fetch('egress-policy') == 'block'
        endpoints = harden.fetch('with').fetch('allowed-endpoints').split
        %w[github.com:443 api.github.com:443 proxy.golang.org:443 sum.golang.org:443].each do |endpoint|
          raise "missing endpoint #{endpoint}" unless endpoints.include?(endpoint)
        end
        raise 'unbounded egress' if endpoints.include?('*:443')
      else
        raise 'do not claim unsupported Windows blocking' unless harden.fetch('with').fetch('egress-policy') == 'audit' &&
                                                               !harden.fetch('with').key?('allowed-endpoints')
      end
      publisher = steps.find { |step| step.fetch('uses', '').start_with?('goreleaser/goreleaser-action@') }
      if name == 'goreleaser-windows'
        validate_app_token(steps, 'winget-pkgs')
        raise 'token handoff' unless publisher.fetch('env').fetch('WINGET_TOKEN') == '${{ steps.publisher-token.outputs.token }}'
        raise 'Chocolatey prerelease skip' unless publisher.fetch('with').fetch('args').include?(
          "${{ contains(github.ref_name, '-') && '--skip=chocolatey' || '' }}"
        )
        raise 'Chocolatey secret handoff' unless publisher.fetch('env').fetch('CHOCO_API_KEY').include?('secrets.CHOCO_API_KEY')
      else
        raise 'defer cask upload until native formatting' unless publisher.fetch('with').fetch('args').include?(
          '--config .release-check/release.yaml'
        ) && steps.any? { |step| step.fetch('run', '').include?('ruby .github/scripts/release_snapshot.rb defer-homebrew') }
        raise 'only the cask can be transferred' unless steps.any? do |step|
          step.fetch('uses', '').start_with?('actions/upload-artifact@') &&
            step.fetch('with').fetch('path') == 'dist/homebrew/Casks/azctx.rb'
        end
      end

      attest = steps.find { |step| step.fetch('uses', '').start_with?('actions/attest@') }
      pattern = name == 'goreleaser-linux' ? 'dist/*.tar.gz' : 'dist/*.zip'
      raise 'all archive attestations' unless attest.fetch('with').fetch('subject-path') == pattern
      raise 'no registry permission needed' unless attest.fetch('with').fetch('create-storage-record') == false
      raise 'Syft must remain' unless steps.any? { |step| step.fetch('uses', '').start_with?('anchore/sbom-action/download-syft@') }
    end
    raise 'long-lived token' if workflow.to_s.include?('AUTH_GITHUB')
  end

  def validate_step_integrity(steps)
    steps.each do |step|
      next unless step.key?('uses')

      raise 'unpinned action' unless step['uses'].match?(/@[a-f0-9]{40}$/)
      if step['uses'].start_with?('actions/checkout@')
        raise 'persisted credentials' unless step.fetch('with').fetch('persist-credentials') == false
      elsif step['uses'].start_with?('actions/setup-go@')
        raise 'release cache write' unless step.fetch('with').fetch('cache') == false
      end
    end
  end

  def validate_app_token(steps, repository)
    token = steps.find { |step| step.fetch('uses', '').start_with?('actions/create-github-app-token@') }
    raise 'App token scope' unless token.fetch('with').fetch('repositories') == repository &&
                                   token.fetch('with').fetch('owner') == 'whiteducksoftware' &&
                                   token.fetch('with').fetch('permission-contents') == 'write'
    raise 'App credential interface' unless token.fetch('with').fetch('client-id') == '${{ vars.RELEASE_APP_CLIENT_ID }}' &&
                                            token.fetch('with').fetch('private-key') == '${{ secrets.RELEASE_APP_PRIVATE_KEY }}'
    raise 'token must be revoked' if token.fetch('with').fetch('skip-token-revoke', false)
  end

  def validate_homebrew_job(job)
    raise 'native Homebrew formatting' unless job.fetch('runs-on') == 'macos-latest'
    raise 'minimal cask permissions' unless job.fetch('permissions') == { 'contents' => 'read' }
    raise 'no prerelease cask publishing' unless job.fetch('if').include?("!contains(github.ref_name, '-')")
    steps = job.fetch('steps')
    raise 'do not claim unsupported macOS blocking' unless steps.first.fetch('with').fetch('egress-policy') == 'audit' &&
                                                          !steps.first.fetch('with').key?('allowed-endpoints')
    validate_app_token(steps, 'homebrew-tap')
    format = steps.index { |step| step.fetch('run', '').include?('brew style --fix') &&
                                  step.fetch('run', '').include?('brew style .release-check/cask/azctx.rb') }
    token = steps.index { |step| step.fetch('uses', '').start_with?('actions/create-github-app-token@') }
    publish = steps.index { |step| step.fetch('run', '').include?('publish_cask.py') }
    raise 'format before token before publish' unless format && token && publish && format < token && token < publish
    raise 'publish token handoff' unless steps.fetch(publish).fetch('env').fetch('GH_TOKEN') ==
                                       '${{ steps.publisher-token.outputs.token }}'
  end

  def test_release_workflow_contract
    validate_workflow(release_workflow)
  end

  def test_deferred_release_configuration_changes_only_cask_upload
    assert system(RbConfig.ruby, File.join(ROOT, '.github/scripts/release_snapshot.rb'), 'defer-homebrew')
    rendered = YAML.safe_load(File.read(File.join(ROOT, '.release-check/release.yaml')))
    expected = Marshal.load(Marshal.dump(@unix))
    expected.fetch('homebrew_casks').each { |cask| cask['skip_upload'] = true }
    assert_equal expected, rendered
  end

  def test_native_snapshot_acceptance_is_configured_without_credentials
    workflow = YAML.safe_load(File.read(File.join(ROOT, '.github/workflows/release-check.yaml')))
    assert_equal({}, workflow.fetch('permissions'))
    job = workflow.fetch('jobs').fetch('snapshot')
    assert_equal({ 'contents' => 'read' }, job.fetch('permissions'))
    matrix = job.fetch('strategy').fetch('matrix')
    assert_equal %w[ubuntu-latest macos-latest windows-latest], matrix.fetch('os')
    assert_equal %w[stable rc], matrix.fetch('channel')
    assert_equal 'windows', matrix.fetch('include').find { |item| item['os'] == 'windows-latest' }.fetch('mode')
    steps = job.fetch('steps')
    assert steps.any? { |step| step.fetch('run', '').include?('ruby .github/scripts/release_snapshot.rb') }
    assert steps.any? { |step| step.fetch('if', '').include?("runner.os == 'macOS'") &&
                              step.fetch('run', '').include?('brew style --fix') &&
                              step.fetch('run', '').include?('brew style "$cask"') }
    assert steps.any? { |step| step.fetch('if', '').include?("runner.os == 'Windows'") &&
                              step.fetch('run', '').include?('ZipFile') &&
                              step.fetch('run', '').include?("GetEntry('azctx.nuspec')") }
    assert steps.any? { |step| step.fetch('if', '') == "runner.os == 'Windows'" &&
                              step.fetch('run', '').include?('check_winget.ps1') }
    refute_match(/secrets\.|publisher-token|publish_cask\.py/, workflow.to_s)
  end

  def test_workflow_policy_rejects_regressions
    mutations = [
      ->(w) { w['permissions'] = { 'contents' => 'write' } },
      ->(w) { w['jobs']['goreleaser-linux']['environment'] = 'unprotected' },
      ->(w) { w['jobs']['goreleaser-windows']['permissions']['packages'] = 'write' },
      ->(w) { w['jobs']['goreleaser-linux']['steps'][0]['with']['egress-policy'] = 'audit' },
      ->(w) { w['jobs']['goreleaser-linux']['steps'][0]['with']['allowed-endpoints'] = '*:443' },
      ->(w) { w['jobs']['goreleaser-linux']['steps'].delete_if { |s| s.fetch('uses', '').start_with?('actions/attest@') } },
      ->(w) { w['jobs']['homebrew']['if'] = 'true' },
      ->(w) { w['jobs']['homebrew']['steps'].delete_if { |s| s.fetch('run', '').include?('brew style --fix') } },
      ->(w) do
        step = w['jobs']['goreleaser-windows']['steps'].find { |s| s.fetch('uses', '').start_with?('actions/create-github-app-token@') }
        step['with']['repositories'] = 'azctx,winget-pkgs,homebrew-tap'
      end,
      ->(w) do
        step = w['jobs']['goreleaser-windows']['steps'].find { |s| s.fetch('uses', '').start_with?('goreleaser/goreleaser-action@') }
        step['with']['args'] = 'release --clean --config .goreleaser-windows.yaml'
      end
    ]
    mutations.each do |mutate|
      copy = Marshal.load(Marshal.dump(release_workflow))
      mutate.call(copy)
      assert_raises(StandardError) { validate_workflow(copy) }
    end
  end

  def test_package_policy_rejects_regressions
    mutations = [
      ->(u, _) { u['brews'] = [] },
      ->(u, _) { u['homebrew_casks'][0]['skip_upload'] = 'false' },
      ->(u, _) { u['homebrew_casks'][0]['hooks'] = { 'post' => { 'install' => 'system_command "xattr"' } } },
      ->(u, _) { u['homebrew_casks'][0]['directory'] = 'Formula' },
      ->(u, _) { u['archives'][0]['formats'] = ['deb'] },
      ->(_, w) { w['chocolateys'][0]['dependencies'][0]['version'] = '2.50.0' },
      ->(_, w) { w['winget'][0]['dependencies'][0]['minimum_version'] = '2.50.0' },
      ->(_, w) { w['winget'][0]['repository']['pull_request']['enabled'] = true },
      ->(_, w) { w['release']['prerelease'] = 'false' },
      ->(_, w) { w['release']['make_latest'] = 'true' }
    ]
    mutations.each do |mutate|
      unix, windows = Marshal.load(Marshal.dump([@unix, @windows]))
      mutate.call(unix, windows)
      assert_raises(RuntimeError) { validate_packages(unix, windows) }
    end
  end

  def test_conventional_changelog_classification
    examples = {
      'feat!: remove legacy picker' => 'Breaking changes',
      'fix(auth)!: remove broker fallback' => 'Breaking changes',
      'docs(upgrade)!: require current Homebrew' => 'Breaking changes',
      'feat(auth): add broker support' => 'Features',
      'fix: handle cancellation' => 'Fixes',
      'perf(match): avoid allocations' => 'Performance',
      'refactor: simplify parsing' => 'Refactoring',
      'docs: explain login' => 'Documentation',
      'test: cover interruption' => 'Tests',
      'build: pin Go' => 'Build',
      'ci: harden publishing' => 'CI',
      'chore: update metadata' => 'Chores',
      'unclassified change' => 'Other'
    }
    [@unix, @windows].each do |config|
      groups = config.fetch('changelog').fetch('groups')
      assert_equal 'Breaking changes', groups.min_by { |group| group.fetch('order') }.fetch('title')
      examples.each do |message, title|
        group = groups.find { |item| Regexp.new(item.fetch('regexp', '')).match?(message) }
        assert_equal title, group && group['title'], message
      end
      assert_empty config.fetch('changelog').fetch('filters', {}).fetch('exclude', []),
                   'do not hide breaking changes in documentation or other commit types'
    end
  end
end
