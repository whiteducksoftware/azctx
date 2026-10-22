require 'minitest/autorun'
require 'yaml'

class IntegrationWorkflowTest < Minitest::Test
  ROOT = File.expand_path('../..', __dir__)

  def setup
    @workflow = YAML.safe_load(File.read(File.join(ROOT, '.github/workflows/integration.yaml')))
    @ci = YAML.safe_load(File.read(File.join(ROOT, '.github/workflows/ci.yaml')))
  end

  def validate(workflow)
    raise 'workflow permissions' unless workflow.fetch('permissions') == {}
    triggers = workflow.fetch('on', workflow[true])
    raise 'unsafe triggers' unless triggers.keys.sort == %w[schedule workflow_dispatch]
    raise 'dispatch inputs' unless triggers['workflow_dispatch'].nil?
    jobs = workflow.fetch('jobs')
    raise 'job surface' unless jobs.keys.sort == %w[azure notify prerequisites]
    jobs.each do |name, job|
      raise 'job timeout' unless job.fetch('timeout-minutes').between?(1, 30)
      raise 'Node actions must run on host' if job.key?('container')
      first = job.fetch('steps').first
      raise 'runner hardening' unless first.fetch('uses').start_with?('step-security/harden-runner@')
      job.fetch('steps').each do |step|
        next unless step.key?('uses')

        raise 'unpinned action' unless step['uses'].match?(/@[a-f0-9]{40}$/)
        if step['uses'].start_with?('actions/checkout@')
          raise 'checkout credentials' unless step.fetch('with')['persist-credentials'] == false
        end
      end
      permissions = job.fetch('permissions')
      raise 'OIDC scope' if name != 'azure' && permissions.key?('id-token')
    end
    preflight = jobs.fetch('prerequisites')
    condition = preflight.fetch('if')
    raise 'trusted source only' unless condition.include?("github.repository == 'whiteducksoftware/azctx'") &&
                                             condition.include?("github.ref == 'refs/heads/main'")
    azure = jobs.fetch('azure')
    raise 'explicit prerequisite gate' unless azure.fetch('needs') == 'prerequisites' &&
                                              azure.fetch('if').include?("needs.prerequisites.outputs.ready == 'true'")
    raise 'minimal OIDC permissions' unless azure.fetch('permissions') == { 'contents' => 'read', 'id-token' => 'write' }
    raise 'protected environment' unless azure.fetch('environment') == 'azure-integration'
    matrix = azure.fetch('strategy').fetch('matrix').fetch('include')
    raise 'both CLI versions' unless matrix.map { |item| item['cli'] } == ['2.61.0', 'latest']
    raise 'old image digest pin' unless matrix.first.fetch('image').match?(%r{\Amcr.microsoft.com/azure-cli:2.61.0@sha256:[a-f0-9]{64}\z})
    raise 'latest image source' unless matrix.last['image'] == 'mcr.microsoft.com/azure-cli:latest'
    login = azure.fetch('steps').find { |step| step.fetch('uses', '').start_with?('azure/login@') }
    %w[client-id tenant-id subscription-id].zip(%w[CLIENT_ID TENANT_ID SUBSCRIPTION_ID]).each do |input, variable|
      raise 'repository variables only' unless login.fetch('with')[input] == "${{ vars.AZCTX_TEST_#{variable} }}"
    end
    raise 'no secret credential input' if login.fetch('with').key?('creds')
    runs = azure.fetch('steps').map { |step| step['run'] }.compact.join("\n")
    raise 'container CLI must resolve immutable digest' unless runs.include?('docker image inspect') &&
                                                             runs.include?('AZCTX_AZURE_CLI_IMAGE=$resolved')
    raise 'host action must resolve the same shim' unless runs.include?('az-container.sh .integration/bin/az') &&
                                                        runs.include?('.integration/bin" >> "$GITHUB_PATH"')
    raise 'real command checks required' unless runs.include?('azure_integration.py check --binary ./azctx')
    raise 'no Azure artifact upload' if azure.fetch('steps').any? { |step| step.fetch('uses', '').start_with?('actions/upload-artifact@') }
    notify = jobs.fetch('notify')
    raise 'notify on any failed prerequisite/matrix' unless notify.fetch('if').include?('always()') &&
                                                          notify['if'].include?("needs.azure.result == 'failure'") &&
                                                          notify['if'].include?("needs.prerequisites.result == 'failure'")
    raise 'separate issue permission' unless notify.fetch('permissions') == { 'issues' => 'write' }
    notify_source = notify.fetch('steps').map { |step| step['run'] }.compact.join("\n")
    raise 'owner notification' unless notify_source.include?('gh issue create') &&
                                     notify_source.include?('@nmeisenzahl @hutstep @swolg')
    source = workflow.to_s
    raise 'no untrusted dispatch/PR inputs' if source.match?(/github\.event\.(inputs|pull_request)|pull_request_target|secrets\./)
  end

  def test_workflow_contract
    validate(@workflow)
    steps = @ci.fetch('jobs').fetch('test').fetch('steps')
    assert_equal %w[ubuntu-latest macos-latest windows-latest], @ci['jobs']['test']['strategy']['matrix']['os']
    assert steps.any? { |step| step['run'] == 'make e2e' }, 'native E2E must run on all three OSes'
    assert steps.any? { |step| step.fetch('uses', '').start_with?('actions/upload-artifact@') &&
                                step.fetch('with')['path'] == 'coverage.out' }, 'publish coverage per OS'
    lint = @ci.fetch('jobs').fetch('lint').fetch('steps')
    assert lint.any? { |step| step['run'] == 'make ci-check' }, 'CI must run workflow negative tests'
  end

  def test_policy_rejects_unsafe_mutations
    mutations = [
      ->(w) { w['permissions'] = { 'id-token' => 'write' } },
      ->(w) { w['on']['pull_request_target'] = {} },
      ->(w) { w['on']['workflow_dispatch'] = { 'inputs' => { 'image' => {} } } },
      ->(w) { w['jobs']['notify']['permissions']['id-token'] = 'write' },
      ->(w) { w['jobs']['azure']['container'] = 'mcr.microsoft.com/azure-cli:2.61.0' },
      ->(w) { w['jobs']['prerequisites']['if'] = 'true' },
      ->(w) { w['jobs']['azure']['if'] = 'always()' },
      ->(w) { w['jobs']['azure']['strategy']['matrix']['include'][0]['image'] = 'mcr.microsoft.com/azure-cli:2.61.0' },
      ->(w) { w['jobs']['azure']['steps'][0]['uses'] = 'step-security/harden-runner@main' },
      ->(w) { w['jobs']['azure']['steps'].delete_if { |step| step['name'] == 'Check real profile, switching, env and guard' } },
      ->(w) { w['jobs']['azure']['steps'].delete_if { |step| step['name'] == 'Prepare isolated container CLI' } },
      ->(w) { w['jobs']['notify']['if'] = 'success()' }
    ]
    mutations.each do |mutate|
      copy = Marshal.load(Marshal.dump(@workflow))
      mutate.call(copy)
      assert_raises(RuntimeError) { validate(copy) }
    end
  end
end
