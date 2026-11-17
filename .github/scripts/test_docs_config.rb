require 'minitest/autorun'
require 'yaml'

class DocsConfigTest < Minitest::Test
  def validate(workflow)
    steps = workflow.fetch('jobs').fetch('lint').fetch('steps')
    gate = steps.select { |step| step['run'] == 'make docs-check' }
    raise 'required non-rewriting docs freshness gate is missing' unless gate.length == 1

    step = gate.first
    raise 'docs freshness cannot be conditional or ignored' if step.key?('if') || step.key?('continue-on-error')
    job = workflow.fetch('jobs').fetch('lint')
    raise 'docs lint job cannot be conditional or ignored' if job.key?('if') || job.key?('continue-on-error')
    setup = steps.index { |entry| entry.fetch('uses', '').start_with?('actions/setup-go@') }
    raise 'docs freshness requires Go setup' unless setup && setup < steps.index(step)
  end

  def workflow
    YAML.safe_load(File.read(File.expand_path('../workflows/ci.yaml', __dir__)))
  end

  def test_required_gate
    validate(workflow)
  end

  def test_gate_downgrades_are_rejected
    [
      ->(job, step) { job['steps'].delete(step) },
      ->(_job, step) { step['run'] = 'make docs' },
      ->(_job, step) { step['continue-on-error'] = true },
      ->(_job, step) { step['if'] = 'false' },
      ->(job, _step) { job['if'] = 'false' },
      ->(job, _step) { job['continue-on-error'] = true },
      ->(job, step) { job['steps'].delete(step); job['steps'].unshift(step) }
    ].each do |mutation|
      copy = workflow
      job = copy.fetch('jobs').fetch('lint')
      gate = job.fetch('steps').find { |step| step['run'] == 'make docs-check' }
      mutation.call(job, gate)
      assert_raises(RuntimeError) { validate(copy) }
    end
  end
end
