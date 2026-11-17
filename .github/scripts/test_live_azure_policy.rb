require 'minitest/autorun'
require 'yaml'

# Live Azure checks are a manual maintainer action (scripts/live-azure-test.py).
# No workflow may sign in to Azure, use the former integration environment or
# run the live script; release attestation keeps its own OIDC permission.
class LiveAzurePolicyTest < Minitest::Test
  ROOT = File.expand_path('../..', __dir__)
  RELEASE_OIDC_JOBS = %w[goreleaser-linux windows-publish].freeze

  def setup
    @workflows = Dir.glob(File.join(ROOT, '.github/workflows/*.{yaml,yml}')).sort.to_h do |path|
      [File.basename(path), YAML.safe_load(File.read(path), aliases: false)]
    end
  end

  def steps(job)
    job.fetch('steps', [])
  end

  def validate(workflows)
    raise 'nightly Azure integration workflow must stay removed' if workflows.key?('integration.yaml')

    workflows.each do |file, workflow|
      source = workflow.to_s
      raise "#{file}: Azure integration environment" if source.include?('azure-integration')
      raise "#{file}: Azure test variables" if source.match?(/AZCTX_(TEST|INTEGRATION)_/)
      raise "#{file}: live script" if source.match?(/live-azure-test|make live-test|az-container/)
      raise "#{file}: workflow-level OIDC" if workflow.fetch('permissions', {}).is_a?(Hash) &&
                                               workflow.fetch('permissions', {}).key?('id-token')

      workflow.fetch('jobs').each do |name, job|
        raise "#{file}/#{name}: Azure sign-in action" if steps(job).any? { |step| step.fetch('uses', '').match?(%r{\Aazure/login@}i) }
        raise "#{file}/#{name}: Azure CLI sign-in" if steps(job).any? { |step| step.fetch('run', '').match?(/\baz\s+login\b/) }

        permissions = job.fetch('permissions', {})
        next unless permissions.is_a?(Hash) && permissions.key?('id-token')
        raise "#{file}/#{name}: OIDC outside release attestation" unless file == 'goreleaser.yaml' &&
                                                                         RELEASE_OIDC_JOBS.include?(name) &&
                                                                         job.fetch('environment') == 'release' &&
                                                                         permissions['attestations'] == 'write'
      end
    end
  end

  def test_no_workflow_has_live_azure_access
    validate(@workflows)
  end

  def test_release_attestation_oidc_is_retained
    release = @workflows.fetch('goreleaser.yaml').fetch('jobs')
    RELEASE_OIDC_JOBS.each do |name|
      assert_equal 'write', release.fetch(name).fetch('permissions').fetch('id-token'), name
    end
  end

  def test_offline_e2e_and_coverage_run_natively_on_three_platforms
    ci = @workflows.fetch('ci.yaml')
    test = ci.fetch('jobs').fetch('test')
    assert_equal %w[ubuntu-latest macos-latest windows-latest], test.fetch('strategy').fetch('matrix').fetch('os')
    assert(steps(test).any? { |step| step['run'] == 'make e2e' }, 'native E2E must run on all three OSes')
    assert(steps(test).any? { |step| step.fetch('uses', '').start_with?('actions/upload-artifact@') &&
                                     step.fetch('with')['path'] == 'coverage.out' }, 'publish coverage per OS')
    lint = ci.fetch('jobs').fetch('lint')
    assert(steps(lint).any? { |step| step['run'] == 'make ci-check' }, 'CI must run these policy regressions')
  end

  def test_makefile_keeps_live_test_manual_and_its_offline_tests_in_ci_check
    makefile = File.read(File.join(ROOT, 'Makefile'))
    ci_check = makefile[/^ci-check:\n((?:\t.*\n)+)/, 1]
    assert_includes ci_check, "python3 -B -m unittest discover -s scripts -p 'test_*.py' -v"
    assert_includes ci_check, 'ruby .github/scripts/test_live_azure_policy.rb'
    %w[check test e2e ci-check].each do |target|
      recipe = makefile[/^#{target}:.*\n((?:\t.*\n)*)/, 0]
      refute_match(/live-test|live-azure-test/, recipe, target)
    end
  end

  def test_policy_rejects_live_azure_ci_mutations
    login = { 'uses' => 'azure/login@a641126d1b8aa4d1fa005f4f92df94a3a4c4c906' }
    mutations = [
      ->(w) { w['integration.yaml'] = { 'jobs' => {} } },
      ->(w) { w['ci.yaml']['jobs']['test']['environment'] = 'azure-integration' },
      ->(w) { w['ci.yaml']['jobs']['test']['permissions'] = { 'contents' => 'read', 'id-token' => 'write' } },
      ->(w) { w['ci.yaml']['permissions'] = { 'id-token' => 'write' } },
      ->(w) { w['ci.yaml']['jobs']['test']['steps'] << login },
      ->(w) { w['ci.yaml']['jobs']['test']['steps'] << { 'run' => 'az login --identity' } },
      ->(w) { w['ci.yaml']['jobs']['test']['steps'] << { 'run' => 'make live-test' } },
      ->(w) { w['ci.yaml']['jobs']['test']['steps'] << { 'run' => 'python3 scripts/live-azure-test.py' } },
      ->(w) { w['ci.yaml']['env'] = { 'TENANT' => '${{ vars.AZCTX_TEST_TENANT_ID }}' } },
      ->(w) { w['scorecard.yaml']['jobs'].values.first['permissions']['id-token'] = 'write' },
      ->(w) { w['goreleaser.yaml']['jobs']['goreleaser-linux']['steps'] << login },
      ->(w) { w['goreleaser.yaml']['jobs']['goreleaser-linux']['environment'] = 'azure-integration' }
    ]
    mutations.each_with_index do |mutate, index|
      copy = Marshal.load(Marshal.dump(@workflows))
      mutate.call(copy)
      assert_raises(RuntimeError, "mutation #{index}") { validate(copy) }
    end
  end
end
