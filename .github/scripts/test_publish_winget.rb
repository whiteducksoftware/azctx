require 'minitest/autorun'
require 'yaml'
require 'base64'
require 'fileutils'
require_relative 'publish_winget'

class PublishWingetTest < Minitest::Test
  def setup
    @checksums = { 'azctx_Windows_x86_64.zip' => 'a' * 64, 'azctx_Windows_arm64.zip' => 'b' * 64 }
    common = { 'PackageIdentifier' => 'whiteducksoftware.azctx', 'PackageVersion' => '2026.11.0',
               'ManifestVersion' => '1.12.0' }
    documents = {
      'whiteducksoftware.azctx.yaml' => common.merge('ManifestType' => 'version', 'DefaultLocale' => 'en-US'),
      'whiteducksoftware.azctx.locale.en-US.yaml' => common.merge('ManifestType' => 'defaultLocale', 'PackageLocale' => 'en-US'),
      'whiteducksoftware.azctx.installer.yaml' => common.merge(
        'ManifestType' => 'installer',
        'Dependencies' => { 'PackageDependencies' => [{ 'PackageIdentifier' => 'Microsoft.AzureCLI', 'MinimumVersion' => '2.61.0' }] },
        'Installers' => @checksums.map do |name, sha|
          { 'Architecture' => name.include?('arm64') ? 'arm64' : 'x64',
            'InstallerUrl' => "https://github.com/whiteducksoftware/azctx/releases/download/2026.11.0/#{name}",
            'InstallerSha256' => sha }
        end
      )
    }
    @files = documents.transform_values { |document| YAML.dump(document) }
  end

  def publish(responses, tag: '2026.11.0', files: @files)
    calls = []
    transport = lambda do |method, path, data|
      calls << [method, path, data]
      raise 'unexpected request' if responses.empty?

      responses.shift
    end
    WingetPublisher.new(transport: transport).publish(tag, files, @checksums)
    assert_empty responses
    calls
  end

  def test_creates_only_version_branch_and_three_real_manifests_without_upstream_pr
    calls = publish([nil, { 'object' => { 'sha' => 'base-sha' } }, {}, nil, {}, nil, {}, nil, {}])
    assert_equal ['POST', '/git/refs', { 'ref' => 'refs/heads/azctx-2026.11.0', 'sha' => 'base-sha' }], calls[2]
    writes = calls.select { |method, path, _| method == 'PUT' && path.start_with?('/contents/') }
    assert_equal 3, writes.length
    writes.each do |_, path, data|
      name = path.split('/').last
      assert_equal "/contents/manifests/w/whiteducksoftware/azctx/2026.11.0/#{name}", path
      assert_equal @files.fetch(name), Base64.strict_decode64(data.fetch('content'))
      assert_equal 'azctx-2026.11.0', data.fetch('branch')
      refute data.key?('sha')
    end
    refute calls.any? { |_, path, _| path.include?('/pulls') }
  end

  def test_existing_branch_updates_with_sha_and_unchanged_files_do_not_commit
    responses = [{ 'object' => { 'sha' => 'branch-sha' } }]
    @files.each_value { |content| responses << { 'sha' => 'existing-sha', 'content' => Base64.strict_encode64(content) } }
    calls = publish(responses)
    assert calls.all? { |method, _, _| method == 'GET' }

    responses = [{ 'object' => { 'sha' => 'branch-sha' } }]
    @files.each_value { responses.concat([{ 'sha' => 'previous-sha', 'content' => Base64.strict_encode64('old') }, {}]) }
    calls = publish(responses)
    assert calls.select { |method, _, _| method == 'PUT' }.all? { |_, _, data| data.fetch('sha') == 'previous-sha' }
  end

  def test_invalid_or_prerelease_artifacts_never_reach_network
    transport = ->(*) { flunk 'invalid artifacts reached the publisher API' }
    publisher = WingetPublisher.new(transport: transport)
    %w[2026.11.0-rc.1 2026.11.0-SNAPSHOT main ../main].each do |tag|
      assert_raises(RuntimeError) { publisher.publish(tag, @files, @checksums) }
    end
    mutations = [
      ->(files) { files.delete('whiteducksoftware.azctx.yaml') },
      ->(files) { files['private-key.txt'] = 'synthetic-secret' },
      ->(files) { files.transform_values! { |content| content.gsub('2026.11.0', '2026.11.1') } },
      ->(files) { files.transform_values! { |content| content.gsub('2.61.0', '2.60.0') } },
      ->(files) { files.transform_values! { |content| content.gsub('github.com', 'untrusted.example') } },
      ->(files) { files.transform_values! { |content| content.gsub('a' * 64, 'c' * 64) } }
    ]
    mutations.each do |mutate|
      files = @files.dup
      mutate.call(files)
      assert_raises(RuntimeError) { publisher.publish('2026.11.0', files, @checksums) }
    end
  end

  def test_unavailable_fork_base_never_creates_a_branch_or_writes_files
    calls = []
    transport = ->(method, path, _) { calls << [method, path]; nil }
    error = assert_raises(RuntimeError) do
      WingetPublisher.new(transport: transport).publish('2026.11.0', @files, @checksums)
    end
    assert_equal 'winget fork base branch is unavailable', error.message
    assert_equal [['GET', '/git/ref/heads/azctx-2026.11.0'], ['GET', '/git/ref/heads/master']], calls
  end

  def test_http_denial_does_not_leak_token_or_make_a_later_write
    request = nil
    response = Struct.new(:code).new('403')
    session = Object.new
    session.define_singleton_method(:request) { |value| request = value; response }
    Net::HTTP.stub(:start, ->(*, **, &block) { block.call(session) }) do
      error = assert_raises(RuntimeError) do
        WingetPublisher.new(token: 'synthetic-test-token').publish('2026.11.0', @files, @checksums)
      end
      assert_match(/HTTP 403/, error.message)
      refute_includes error.message, 'synthetic-test-token'
      assert_equal 'Bearer synthetic-test-token', request['Authorization']
      assert_equal 'api.github.com', request.uri.host
      assert_match(%r{\A/repos/whiteducksoftware/winget-pkgs/}, request.uri.path)
    end
  end

  def test_conflicting_update_aborts_without_writing_remaining_manifests
    responses = [
      Struct.new(:code, :body).new('200', '{"object":{"sha":"branch-sha"}}'),
      Struct.new(:code, :body).new('200', '{"sha":"previous-sha","content":"b2xk"}'),
      Struct.new(:code, :body).new('409', 'synthetic denial body')
    ]
    requests = []
    session = Object.new
    session.define_singleton_method(:request) do |request|
      requests << request
      responses.shift || raise('unexpected later request')
    end
    Net::HTTP.stub(:start, ->(*, **, &block) { block.call(session) }) do
      error = assert_raises(RuntimeError) do
        WingetPublisher.new(token: 'synthetic-test-token').publish('2026.11.0', @files, @checksums)
      end
      assert_match(/HTTP 409/, error.message)
      refute_includes error.message, 'synthetic-test-token'
      refute_includes error.message, 'synthetic denial body'
    end
    assert_equal %w[GET GET PUT], requests.map(&:method)
    assert_equal 'previous-sha', JSON.parse(requests.last.body).fetch('sha')
  end

  def test_artifact_reader_hashes_synthetic_archive_bytes_and_preserves_manifest_bytes
    directory = File.expand_path("../../.release-check/winget-reader-test-#{Process.pid}", __dir__)
    refute File.exist?(directory), 'do not overwrite an existing fixture directory'
    begin
      FileUtils.mkdir_p("#{directory}/winget/manifests/w/whiteducksoftware/azctx/2026.11.0")
      @files.each do |name, content|
        File.binwrite("#{directory}/winget/manifests/w/whiteducksoftware/azctx/2026.11.0/#{name}", content)
      end
      @checksums.each_key { |name| File.binwrite("#{directory}/#{name}", 'abc') }
      files, checksums = WingetPublisher.read_artifacts(directory)
      assert_equal @files, files
      assert_equal @checksums.keys.sort, checksums.keys.sort
      assert_equal ['ba7816bf8f01cfea414140de5dae2223b00361a396177a9cb410ff61f20015ad'], checksums.values.uniq
    ensure
      FileUtils.rm_rf(directory)
    end
  end
end
