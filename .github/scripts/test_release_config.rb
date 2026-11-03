require 'minitest/autorun'
require 'yaml'
require 'open3'

class ReleaseConfigurationTest < Minitest::Test
  ROOT = File.expand_path('../..', __dir__)
  WINDOWS_TRANSFER = "dist/*.zip\ndist/*_Windows_checksums.txt\ndist/*.sbom.json\ndist/winget/**/*.yaml\n".freeze
  ARCHIVE_PERMISSIONS = { 'contents' => 'write', 'id-token' => 'write', 'attestations' => 'write' }.freeze

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
    raise 'release jobs' unless workflow.fetch('jobs').keys.sort ==
                                %w[goreleaser-linux goreleaser-windows homebrew homebrew-publish release-readiness windows-publish]
    workflow.fetch('jobs').each do |name, job|
      raise 'release failures must not be bypassed' if job.key?('continue-on-error') ||
                                                     job.fetch('steps').any? { |step| step.key?('continue-on-error') }
      unless %w[homebrew homebrew-publish].include?(name)
        raise 'upstream successful jobs only' unless job.fetch('if') == "github.repository == 'whiteducksoftware/azctx'"
      end
      validate_step_integrity(job.fetch('steps'))
      if name == 'release-readiness'
        validate_release_readiness(job)
        next
      end
      if name == 'homebrew'
        validate_homebrew_job(job)
        next
      end
      raise 'protected environment' unless job.fetch('environment') == 'release'
      if name == 'homebrew-publish'
        validate_homebrew_publisher(job)
        next
      end
      if name == 'goreleaser-windows'
        validate_windows_native(job)
        next
      end
      if name == 'windows-publish'
        validate_windows_publisher(job)
        next
      end
      raise 'release permissions' unless job.fetch('permissions') == ARCHIVE_PERMISSIONS
      raise 'no release before enforced native egress' unless job.fetch('needs') == 'release-readiness'
      raise 'latest default' unless job.fetch('env').fetch('RELEASE_MAKE_LATEST') ==
                                    "${{ vars.RELEASE_MAKE_LATEST || 'false' }}"
      steps = job.fetch('steps')
      harden = steps.first
      raise 'runner hardening' unless harden.fetch('uses').start_with?('step-security/harden-runner@')
      raise 'Linux block egress' unless harden.fetch('with').fetch('egress-policy') == 'block'
      endpoints = harden.fetch('with').fetch('allowed-endpoints').split
      %w[github.com:443 api.github.com:443 proxy.golang.org:443 sum.golang.org:443].each do |endpoint|
        raise "missing endpoint #{endpoint}" unless endpoints.include?(endpoint)
      end
      raise 'unbounded egress' if endpoints.include?('*:443')
      publisher = steps.find { |step| step.fetch('uses', '').start_with?('goreleaser/goreleaser-action@') }
      raise 'defer cask upload until native formatting' unless publisher.fetch('with').fetch('args').include?(
        '--config .release-check/release.yaml'
      ) && steps.any? { |step| step.fetch('run', '').include?('ruby .github/scripts/release_snapshot.rb defer-homebrew') }
      validate_upload(steps, 'homebrew-cask', 'dist/homebrew/Casks/azctx.rb')
      upload = steps.find { |step| step.fetch('uses', '').start_with?('actions/upload-artifact@') }
      raise 'stable-only raw cask transfer' unless upload.fetch('if') == "${{ !contains(github.ref_name, '-') }}"

      attest = steps.find { |step| step.fetch('uses', '').start_with?('actions/attest@') }
      raise 'all archive attestations' unless attest.fetch('with').fetch('subject-path') == 'dist/*.tar.gz'
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
    tokens = steps.select { |step| step.fetch('uses', '').start_with?('actions/create-github-app-token@') }
    raise 'one minimal automatically revoked App token' unless tokens.length == 1 &&
      tokens.first.fetch('with') == {
        'client-id' => '${{ vars.RELEASE_APP_CLIENT_ID }}', 'private-key' => '${{ secrets.RELEASE_APP_PRIVATE_KEY }}',
        'owner' => 'whiteducksoftware', 'repositories' => repository, 'permission-contents' => 'write'
      }
  end

  def validate_homebrew_job(job)
    raise 'native Homebrew formatting' unless job.fetch('runs-on') == 'macos-latest'
    raise 'minimal cask permissions' unless job.fetch('permissions') == { 'contents' => 'read' }
    validate_cask_gate(job, 'goreleaser-linux')
    raise 'native formatter has no release environment' if job.key?('environment')
    raise 'native formatter has no publisher credentials or execution' if
      job.to_s.match?(/secrets\.|github\.token|create-github-app-token|publish_cask|GH_TOKEN|TOKEN|private-key/)
    steps = job.fetch('steps')
    raise 'do not claim unsupported macOS blocking' unless steps.first.fetch('with').fetch('egress-policy') == 'audit' &&
                                                          !steps.first.fetch('with').key?('allowed-endpoints')
    validate_download(steps, 'homebrew-cask', 'release-cask/Casks')
    format = steps.index { |step| step.fetch('run', '').include?('brew style --fix') &&
                                  step.fetch('run', '').include?('brew style release-cask/Casks/azctx.rb') }
    upload = steps.index { |step| step.fetch('uses', '').start_with?('actions/upload-artifact@') }
    raise 'normalize and verify before transfer' unless format && upload && format < upload
    validate_upload(steps, 'homebrew-cask-normalized', 'release-cask/Casks/azctx.rb')
  end

  def validate_cask_gate(job, producer)
    raise 'only the successful cask producer is needed' unless job.fetch('needs') == producer
    raise 'stable upstream cask jobs only' unless job.fetch('if') ==
      "${{ github.repository == 'whiteducksoftware/azctx' && !contains(github.ref_name, '-') }}"
  end

  def validate_upload(steps, name, path)
    uploads = steps.select { |step| step.fetch('uses', '').start_with?('actions/upload-artifact@') }
    raise 'only declared release files can be transferred' unless uploads.length == 1 &&
      uploads.first.fetch('with') == {
        'name' => name, 'path' => path, 'if-no-files-found' => 'error', 'retention-days' => 1
      }
  end

  def validate_download(steps, name, path = '.release-check/cask')
    downloads = steps.select { |step| step.fetch('uses', '').start_with?('actions/download-artifact@') }
    raise 'verified same-run artifact only' unless downloads.length == 1 &&
      downloads.first.fetch('with') == {
        'name' => name, 'path' => path, 'digest-mismatch' => 'error'
      }
  end

  def validate_windows_native(job)
    raise 'native Chocolatey tools only' unless job.fetch('runs-on') == 'windows-latest'
    raise 'native Windows cannot publish GitHub assets' unless job.fetch('permissions') == { 'contents' => 'read' }
    raise 'Windows depends only on the Linux release' unless job.fetch('needs') == 'goreleaser-linux'
    raise 'native credentials must not be job-scoped' unless job.fetch('env') == {
      'RELEASE_MAKE_LATEST' => "${{ vars.RELEASE_MAKE_LATEST || 'false' }}", 'SYFT_CHECK_FOR_APP_UPDATE' => 'false'
    }
    steps = job.fetch('steps')
    harden = steps.first
    raise 'do not claim unsupported Windows blocking' unless
      harden.fetch('uses').start_with?('step-security/harden-runner@') &&
      harden.fetch('with') == { 'egress-policy' => 'audit' }
    raise 'no portable publishers on native Windows' if job.to_s.match?(/WINGET_TOKEN|create-github-app-token|publish_winget|actions\/attest@/)
    build = steps.find { |step| step.fetch('uses', '').start_with?('goreleaser/goreleaser-action@') }
    raise 'native GoReleaser never publishes' unless build.fetch('with').fetch('args') ==
      "release --clean --skip=publish --config .goreleaser-windows.yaml ${{ contains(github.ref_name, '-') && '--skip=chocolatey' || '' }}" &&
      build.fetch('env') == { 'GITHUB_TOKEN' => '${{ github.token }}' }
    validate_upload(steps, 'windows-release-files', WINDOWS_TRANSFER)
    choco = steps.last
    raise 'only stable native Chocolatey push can receive its credential' unless
      choco.fetch('if') == "${{ !contains(github.ref_name, '-') }}" && choco.fetch('shell') == 'pwsh' &&
      choco.fetch('env') == { 'CHOCO_API_KEY' => '${{ secrets.CHOCO_API_KEY }}' } &&
      choco.fetch('run').include?('choco push $packages[0].FullName --source https://push.chocolatey.org/ --api-key $env:CHOCO_API_KEY') &&
      choco.fetch('run').include?('[string]::IsNullOrWhiteSpace($env:CHOCO_API_KEY)') &&
      choco.fetch('run').include?('$packages.Count -ne 1') &&
      choco.fetch('run').include?('if ($LASTEXITCODE -ne 0) { exit $LASTEXITCODE }')
    raise 'no credentials before the last native push step' if steps[0...-1].to_s.match?(/secrets\./)
  end

  def validate_windows_publisher(job)
    raise 'portable Windows publishers require supported Linux enforcement' unless job.fetch('runs-on') == 'ubuntu-latest'
    raise 'archive-only write permissions' unless job.fetch('permissions') == ARCHIVE_PERMISSIONS
    raise 'only native output is needed' unless job.fetch('needs') == 'goreleaser-windows'
    raise 'portable publisher credentials must not be job-scoped' unless job.fetch('env', {}) == {}
    steps = job.fetch('steps')
    raise 'portable Windows publishers start with block enforcement' unless
      steps.first.fetch('uses').start_with?('step-security/harden-runner@') &&
      steps.first.fetch('with').fetch('egress-policy') == 'block'
    expected = %w[github.com:443 api.github.com:443 uploads.github.com:443
                  *.actions.githubusercontent.com:443 *.blob.core.windows.net:443
                  fulcio.sigstore.dev:443 rekor.sigstore.dev:443
                  tuf-repo-cdn.sigstore.dev:443 tuf-repo.github.com:443 timestamp.github.com:443]
    raise 'minimal Windows publisher endpoints' unless steps.first.fetch('with').fetch('allowed-endpoints').split.sort == expected.sort
    validate_download(steps, 'windows-release-files', '.release-check/windows')
    validate_app_token(steps, 'winget-pkgs')
    token = steps.index { |step| step.fetch('uses', '').start_with?('actions/create-github-app-token@') }
    check = steps.index { |step| step.fetch('run', '') == 'ruby .github/scripts/publish_winget.rb --check' }
    publish = steps.index { |step| step.fetch('run', '') == 'ruby .github/scripts/publish_winget.rb' }
    raise 'verified manifests before token before publish' unless check && token && publish && check < token && token < publish
    raise 'stable-only winget credentials and publication' unless [check, token, publish].all? do |index|
      steps[index].fetch('if') == "${{ !contains(github.ref_name, '-') }}"
    end
    raise 'winget token remains step-scoped' unless steps[publish].fetch('env') == {
      'GH_TOKEN' => '${{ steps.publisher-token.outputs.token }}', 'RELEASE_TAG' => '${{ github.ref_name }}'
    }
    attest = steps.find { |step| step.fetch('uses', '').start_with?('actions/attest@') }
    raise 'all Windows archive attestations' unless attest.fetch('with') == {
      'subject-path' => '.release-check/windows/*.zip', 'create-storage-record' => false
    }
    upload = steps.find { |step| step.fetch('run', '').include?('gh release upload') }
    raise 'verified Windows archive and SBOM upload' unless upload.fetch('run') ==
      'gh release upload "$RELEASE_TAG" .release-check/windows/*.zip .release-check/windows/*_Windows_checksums.txt .release-check/windows/*.sbom.json' &&
      upload.fetch('env') == { 'GH_TOKEN' => '${{ github.token }}', 'RELEASE_TAG' => '${{ github.ref_name }}' }
    raise 'no toolchain rebuild or Chocolatey credentials in the portable publisher' if
      job.to_s.match?(/setup-go|goreleaser-action|CHOCO_API_KEY/)
  end

  def validate_release_readiness(job)
    raise 'readiness has no credentials or environment' unless job.fetch('permissions') == {} &&
                                                             !job.key?('environment') && !job.key?('env')
    steps = job.fetch('steps')
    raise 'explicit native egress release blocker' unless steps.length == 1 &&
      steps.first.fetch('shell') == 'bash' &&
      steps.first.fetch('run') ==
        "echo '::error::BLOCKED (#98): native Chocolatey publishing has audit-only egress on hosted Windows. " \
        "Native macOS style also has audit-only egress. " \
        "A reviewed, supported enforcement design and native acceptance are required before release.' >&2\nexit 1\n"
  end

  def test_native_windows_only_retains_unavoidable_chocolatey_publishing
    validate_windows_native(release_workflow.fetch('jobs').fetch('goreleaser-windows'))
    validate_windows_publisher(release_workflow.fetch('jobs').fetch('windows-publish'))
  end

  def test_release_readiness_is_actually_blocked_before_any_publisher
    workflow = release_workflow
    job = workflow.fetch('jobs').fetch('release-readiness')
    validate_release_readiness(job)
    output, status = Open3.capture2e('bash', '-c', job.fetch('steps').first.fetch('run'))
    refute status.success?
    assert_equal 1, status.exitstatus
    assert_includes output, 'BLOCKED (#98): native Chocolatey publishing has audit-only egress'
    validate_workflow(workflow)
  end

  def test_windows_and_readiness_policy_reject_egress_and_credential_regressions
    mutations = {
      'delete readiness blocker' => ->(w) { w['jobs'].delete('release-readiness') },
      'bypass readiness' => ->(w) { w['jobs']['goreleaser-linux']['needs'] = [] },
      'fake readiness success' => ->(w) { w['jobs']['release-readiness']['steps'][0]['run'].sub!('exit 1', 'exit 0') },
      'ignore readiness failure' => ->(w) { w['jobs']['release-readiness']['continue-on-error'] = true },
      'ignore readiness step failure' => ->(w) { w['jobs']['release-readiness']['steps'][0]['continue-on-error'] = true },
      'always publish despite readiness failure' => ->(w) { w['jobs']['goreleaser-linux']['if'] = 'always()' },
      'pretend native block support' => ->(w) { w['jobs']['goreleaser-windows']['steps'][0]['with']['egress-policy'] = 'block' },
      'native source writes' => ->(w) { w['jobs']['goreleaser-windows']['permissions']['contents'] = 'write' },
      'native winget token' => ->(w) { w['jobs']['goreleaser-windows']['env']['WINGET_TOKEN'] = 'synthetic' },
      'native implicit GoReleaser publishing' => ->(w) do
        build = w['jobs']['goreleaser-windows']['steps'].find { |s| s.fetch('uses', '').start_with?('goreleaser/goreleaser-action@') }
        build['with']['args'].sub!('--skip=publish ', '')
      end,
      'prerelease Chocolatey credential' => ->(w) { w['jobs']['goreleaser-windows']['steps'].last['if'] = 'true' },
      'implicit Chocolatey credential fallback' => ->(w) do
        w['jobs']['goreleaser-windows']['steps'].last['run'].sub!(/^.*IsNullOrWhiteSpace.*\n/, '')
      end,
      'credential precedes native transfer' => ->(w) do
        w['jobs']['goreleaser-windows']['steps'][1]['env'] = { 'GH_TOKEN' => '${{ secrets.RELEASE_APP_PRIVATE_KEY }}' }
      end,
      'upload native workspace' => ->(w) do
        upload = w['jobs']['goreleaser-windows']['steps'].find { |s| s.fetch('uses', '').start_with?('actions/upload-artifact@') }
        upload['with']['path'] = 'dist/'
      end,
      'Windows publisher audit only' => ->(w) { w['jobs']['windows-publish']['steps'][0]['with']['egress-policy'] = 'audit' },
      'Windows transfer digest only warns' => ->(w) do
        download = w['jobs']['windows-publish']['steps'].find { |s| s.fetch('uses', '').start_with?('actions/download-artifact@') }
        download['with']['digest-mismatch'] = 'warn'
      end,
      'Windows token before artifact verification' => ->(w) do
        steps = w['jobs']['windows-publish']['steps']
        token = steps.index { |s| s.fetch('uses', '').start_with?('actions/create-github-app-token@') }
        check = steps.index { |s| s.fetch('run', '').end_with?('publish_winget.rb --check') }
        steps[token], steps[check] = steps[check], steps[token]
      end,
      'prerelease winget token' => ->(w) do
        token = w['jobs']['windows-publish']['steps'].find { |s| s.fetch('uses', '').start_with?('actions/create-github-app-token@') }
        token['if'] = 'true'
      end
    }
    validate_workflow(release_workflow)
    mutations.each do |name, mutate|
      copy = Marshal.load(Marshal.dump(release_workflow))
      mutate.call(copy)
      assert_raises(RuntimeError, name) { validate_workflow(copy) }
    end
  end

  def validate_homebrew_publisher(job)
    raise 'cask publisher must support block enforcement' unless job.fetch('runs-on') == 'ubuntu-latest'
    raise 'minimal cask publisher permissions' unless job.fetch('permissions') == { 'contents' => 'read' }
    validate_cask_gate(job, 'homebrew')
    steps = job.fetch('steps')
    harden = steps.first
    raise 'cask publisher must start in block mode' unless
      harden.fetch('uses').start_with?('step-security/harden-runner@') &&
      harden.fetch('with').fetch('egress-policy') == 'block'
    raise 'cask publisher endpoints must not include build/package services' unless
      harden.fetch('with').fetch('allowed-endpoints').split.sort ==
      %w[github.com:443 api.github.com:443 *.actions.githubusercontent.com:443 *.blob.core.windows.net:443].sort
    validate_app_token(steps, 'homebrew-tap')
    validate_download(steps, 'homebrew-cask-normalized')
    download = steps.index { |step| step.fetch('uses', '').start_with?('actions/download-artifact@') }
    token = steps.index { |step| step.fetch('uses', '').start_with?('actions/create-github-app-token@') }
    publish = steps.index { |step| step.fetch('run', '').include?('publish_cask.py') }
    raise 'verify artifact before token before publish' unless download && token && publish && download < token && token < publish
    raise 'publish token handoff' unless steps.fetch(publish).fetch('env') == {
      'GH_TOKEN' => '${{ steps.publisher-token.outputs.token }}', 'RELEASE_TAG' => '${{ github.ref_name }}'
    }
    raise 'no formatter/build tooling near publisher credentials' if steps.any? do |step|
      step.fetch('run', '').match?(/\b(?:brew|go|goreleaser|syft)\b/) ||
        step.fetch('uses', '').match?(/setup-go|goreleaser-action|sbom-action/)
    end
    raise 'publisher credentials must remain step-scoped' if job.fetch('env', {}).to_s.match?(/TOKEN|secrets\./)
  end

  def test_release_workflow_contract
    validate_workflow(release_workflow)
  end

  def test_cask_boundary_policy_rejects_credential_and_transfer_regressions
    mutations = {
      'native release secrets' => ->(w) { w['jobs']['homebrew']['environment'] = 'release' },
      'native App token' => ->(w) do
        w['jobs']['homebrew']['steps'] << {
          'uses' => 'actions/create-github-app-token@64ab729a6294000f5aa4a4bc0b73d1f32bc45510'
        }
      end,
      'native write permission' => ->(w) { w['jobs']['homebrew']['permissions']['contents'] = 'write' },
      'native secret env' => ->(w) { w['jobs']['homebrew']['env']['GH_TOKEN'] = '${{ secrets.RELEASE_APP_PRIVATE_KEY }}' },
      'transfer workspace' => ->(w) do
        upload = w['jobs']['homebrew']['steps'].find { |s| s.fetch('uses', '').start_with?('actions/upload-artifact@') }
        upload['with']['path'] = '.'
      end,
      'unverified transfer' => ->(w) do
        download = w['jobs']['homebrew-publish']['steps'].find { |s| s.fetch('uses', '').start_with?('actions/download-artifact@') }
        download['with']['digest-mismatch'] = 'warn'
      end,
      'unverified raw cask transfer' => ->(w) do
        download = w['jobs']['homebrew']['steps'].find { |s| s.fetch('uses', '').start_with?('actions/download-artifact@') }
        download['with']['digest-mismatch'] = 'warn'
      end,
      'hidden normalized cask is not uploaded by default' => ->(w) do
        upload = w['jobs']['homebrew']['steps'].find { |s| s.fetch('uses', '').start_with?('actions/upload-artifact@') }
        upload['with']['path'] = '.release-check/cask/azctx.rb'
      end,
      'different workflow run' => ->(w) do
        download = w['jobs']['homebrew-publish']['steps'].find { |s| s.fetch('uses', '').start_with?('actions/download-artifact@') }
        download['with']['run-id'] = '12345'
      end,
      'publish raw unstyled cask' => ->(w) do
        download = w['jobs']['homebrew-publish']['steps'].find { |s| s.fetch('uses', '').start_with?('actions/download-artifact@') }
        download['with']['name'] = 'homebrew-cask'
      end,
      'native publisher' => ->(w) { w['jobs']['homebrew-publish']['runs-on'] = 'macos-latest' },
      'audit publisher' => ->(w) { w['jobs']['homebrew-publish']['steps'][0]['with']['egress-policy'] = 'audit' },
      'overwide publisher endpoints' => ->(w) do
        w['jobs']['homebrew-publish']['steps'][0]['with']['allowed-endpoints'] += ' proxy.golang.org:443'
      end,
      'overwide dependencies' => ->(w) { w['jobs']['homebrew-publish']['needs'] = %w[homebrew goreleaser-windows] },
      'overwide tap token' => ->(w) do
        token = w['jobs']['homebrew-publish']['steps'].find { |s| s.fetch('uses', '').start_with?('actions/create-github-app-token@') }
        token['with']['repositories'] = 'homebrew-tap,winget-pkgs'
      end,
      'prerelease token' => ->(w) { w['jobs']['homebrew-publish']['if'] = 'true' },
      'token before verification' => ->(w) do
        steps = w['jobs']['homebrew-publish']['steps']
        download = steps.index { |s| s.fetch('uses', '').start_with?('actions/download-artifact@') }
        token = steps.index { |s| s.fetch('uses', '').start_with?('actions/create-github-app-token@') }
        steps[download], steps[token] = steps[token], steps[download]
      end,
      'build tooling sees token' => ->(w) { w['jobs']['homebrew-publish']['steps'] << { 'run' => 'go version' } },
      'unscoped publisher token' => ->(w) do
        w['jobs']['homebrew-publish']['env'] = { 'GH_TOKEN' => '${{ steps.publisher-token.outputs.token }}' }
      end
    }
    validate_homebrew_job(release_workflow.fetch('jobs').fetch('homebrew'))
    validate_workflow(release_workflow)
    mutations.each do |name, mutate|
      copy = Marshal.load(Marshal.dump(release_workflow))
      mutate.call(copy)
      assert_raises(RuntimeError, name) { validate_workflow(copy) }
    end
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
    refute_match(/secrets\.|publisher-token|publish_cask\.py|publish_winget\.rb/, workflow.to_s)
  end

  def snapshot_workflow
    YAML.safe_load(File.read(File.join(ROOT, '.github/workflows/release-check.yaml')))
  end

  def validate_snapshot_tool_setup(workflow)
    steps = workflow.fetch('jobs').fetch('snapshot').fetch('steps')
    on_path = false
    steps.each do |step|
      if step.fetch('uses', '').start_with?('goreleaser/goreleaser-action@')
        raise 'snapshot GoReleaser pin' unless
          step.fetch('uses') == 'goreleaser/goreleaser-action@a4f02ab5355a589de60d917a659ce617ed19b3de' &&
          step.fetch('with').values_at('distribution', 'version') == %w[goreleaser v2.18.3]
        # The pinned action exports PATH only in install-only mode, not when running args.
        if [true, 'true'].include?(step.fetch('with')['install-only'])
          raise 'snapshot GoReleaser setup must be unconditional and required' if
            step.key?('if') || step.key?('continue-on-error')
          raise 'install-only ignores args' if step.fetch('with').key?('args')
          on_path = true
        end
      end
      run = step.fetch('run', '')
      next unless run.match?(/\bgoreleaser\s|ruby\s+\.github\/scripts\/release_snapshot\.rb\b/)

      raise "GoReleaser is not on PATH before: #{run.strip}" unless on_path
    end
    %w[.goreleaser.yaml .goreleaser-windows.yaml].each do |config|
      raise "missing snapshot configuration check: #{config}" unless steps.any? do |step|
        step.fetch('run', '') == "goreleaser check --config #{config}" && !step.key?('if')
      end
    end
    raise 'missing publisher-disabled snapshot generation' unless steps.any? do |step|
      step.fetch('run', '').include?('ruby .github/scripts/release_snapshot.rb "$SNAPSHOT_MODE" "$SNAPSHOT_CHANNEL"')
    end
  end

  def test_snapshot_goreleaser_is_on_path_before_every_shell_use
    validate_snapshot_tool_setup(snapshot_workflow)
    renamed = snapshot_workflow
    renamed.fetch('jobs').fetch('snapshot').fetch('steps').each { |step| step['name'] = 'not a setup contract' }
    validate_snapshot_tool_setup(renamed)
  end

  def test_snapshot_tool_setup_rejects_actual_input_and_order_regressions
    mutations = {
      'args invocation does not export PATH' => ->(steps, setup) do
        setup['with'].delete('install-only')
        setup['with']['args'] = 'check --config .goreleaser.yaml'
      end,
      'explicit false does not export PATH' => ->(steps, setup) { setup['with']['install-only'] = false },
      'missing setup' => ->(steps, setup) { steps.delete(setup) },
      'setup after first shell use' => ->(steps, setup) do
        steps.delete(setup)
        first_use = steps.index { |step| step.fetch('run', '').match?(/\bgoreleaser\s/) }
        steps.insert(first_use + 1, setup)
      end,
      'setup after generation' => ->(steps, setup) { steps.delete(setup); steps << setup },
      'generation before setup' => ->(steps, setup) do
        generation = steps.find { |step| step.fetch('run', '').include?('ruby .github/scripts/release_snapshot.rb') }
        steps.delete(generation)
        steps.insert(steps.index(setup), generation)
      end,
      'platform-conditional setup' => ->(steps, setup) { setup['if'] = "runner.os == 'Linux'" },
      'channel-conditional setup' => ->(steps, setup) { setup['if'] = "matrix.channel == 'stable'" },
      'ignored setup failure' => ->(steps, setup) { setup['continue-on-error'] = true },
      'check silently ignored in install-only mode' => ->(steps, setup) do
        setup['with']['args'] = 'check --config .goreleaser.yaml'
      end,
      'missing Unix check' => ->(steps, setup) do
        steps.delete_if { |step| step.fetch('run', '') == 'goreleaser check --config .goreleaser.yaml' }
      end,
      'missing Windows check' => ->(steps, setup) do
        steps.delete_if { |step| step.fetch('run', '') == 'goreleaser check --config .goreleaser-windows.yaml' }
      end
    }
    validate_snapshot_tool_setup(snapshot_workflow)
    mutations.each do |name, mutate|
      copy = snapshot_workflow
      steps = copy.fetch('jobs').fetch('snapshot').fetch('steps')
      setup = steps.find { |step| step.fetch('uses', '').start_with?('goreleaser/goreleaser-action@') }
      mutate.call(steps, setup)
      assert_raises(RuntimeError, name) { validate_snapshot_tool_setup(copy) }
    end
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
        step = w['jobs']['windows-publish']['steps'].find { |s| s.fetch('uses', '').start_with?('actions/create-github-app-token@') }
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
