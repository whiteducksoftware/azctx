require 'yaml'
require 'json'
require 'base64'
require 'digest'
require 'net/http'

# Transfers real GoReleaser manifests, never generates a package or opens a PR.
class WingetPublisher
  REPOSITORY = 'https://api.github.com/repos/whiteducksoftware/winget-pkgs'
  FILE_TYPES = {
    'whiteducksoftware.azctx.yaml' => 'version',
    'whiteducksoftware.azctx.installer.yaml' => 'installer',
    'whiteducksoftware.azctx.locale.en-US.yaml' => 'defaultLocale'
  }.freeze

  def self.read_artifacts(directory)
    manifests = Dir.glob("#{directory}/winget/**/*.yaml")
    raise 'expected exactly three real winget manifests' unless manifests.length == FILE_TYPES.length

    files = manifests.to_h { |path| [File.basename(path), File.binread(path)] }
    checksums = Dir.glob("#{directory}/*.zip").to_h { |path| [File.basename(path), Digest::SHA256.file(path).hexdigest] }
    [files, checksums]
  end

  def self.validate(tag, files, checksums)
    raise 'only stable version tags can publish winget manifests' unless tag.match?(/\Av?\d+\.\d+\.\d+\z/)
    raise 'only the three real winget manifests can be published' unless files.keys.sort == FILE_TYPES.keys.sort
    raise 'expected both verified Windows archives' unless checksums.keys.sort ==
                                                        %w[azctx_Windows_arm64.zip azctx_Windows_x86_64.zip]

    version = tag.delete_prefix('v')
    documents = files.transform_values { |content| YAML.safe_load(content, aliases: false) }
    FILE_TYPES.each do |name, type|
      document = documents.fetch(name)
      raise 'winget manifest identity/version mismatch' unless document.values_at('PackageIdentifier', 'PackageVersion', 'ManifestType') ==
                                                              ['whiteducksoftware.azctx', version, type]
    end
    installer = documents.fetch('whiteducksoftware.azctx.installer.yaml')
    raise 'winget Azure CLI dependency floor' unless installer.fetch('Dependencies').fetch('PackageDependencies') ==
                                                   [{ 'PackageIdentifier' => 'Microsoft.AzureCLI', 'MinimumVersion' => '2.61.0' }]
    installers = installer.fetch('Installers')
    raise 'winget installer architectures' unless installers.map { |item| item.fetch('Architecture') }.sort == %w[arm64 x64]

    installers.each do |item|
      name = item.fetch('Architecture') == 'x64' ? 'azctx_Windows_x86_64.zip' : 'azctx_Windows_arm64.zip'
      raise 'winget archive URL/checksum mismatch' unless item.fetch('InstallerUrl') ==
        "https://github.com/whiteducksoftware/azctx/releases/download/#{tag}/#{name}" &&
        item.fetch('InstallerSha256').downcase == checksums.fetch(name)
    end
  end

  def initialize(token: nil, transport: nil)
    @token = token
    @transport = transport || method(:request)
  end

  def publish(tag, files, checksums)
    self.class.validate(tag, files, checksums)
    version = tag.delete_prefix('v')
    branch = "azctx-#{version}"
    unless @transport.call('GET', "/git/ref/heads/#{branch}", nil)
      base = @transport.call('GET', '/git/ref/heads/master', nil)
      raise 'winget fork base branch is unavailable' unless base

      @transport.call('POST', '/git/refs', { 'ref' => "refs/heads/#{branch}", 'sha' => base.fetch('object').fetch('sha') })
    end
    files.each do |name, content|
      path = "/contents/manifests/w/whiteducksoftware/azctx/#{version}/#{name}"
      previous = @transport.call('GET', "#{path}?ref=#{branch}", nil)
      next if previous && Base64.decode64(previous.fetch('content')) == content

      data = {
        'branch' => branch, 'content' => Base64.strict_encode64(content),
        'message' => "build: update azctx winget manifest for #{tag}\n\n" \
                     'Co-authored-by: Copilot <223556219+Copilot@users.noreply.github.com>'
      }
      data['sha'] = previous.fetch('sha') if previous
      @transport.call('PUT', path, data)
    end
  end

  private

  def request(method, path, data)
    uri = URI("#{REPOSITORY}#{path}")
    request = Net::HTTP.const_get(method.capitalize).new(uri)
    request['Authorization'] = "Bearer #{@token}"
    request['Accept'] = 'application/vnd.github+json'
    request['X-GitHub-Api-Version'] = '2026-03-10'
    if data
      request['Content-Type'] = 'application/json'
      request.body = JSON.generate(data)
    end
    response = Net::HTTP.start(uri.host, uri.port, use_ssl: true, open_timeout: 30, read_timeout: 30) do |http|
      http.request(request)
    end
    return nil if method == 'GET' && response.code == '404'
    raise "winget API request failed (HTTP #{response.code})" unless %w[200 201].include?(response.code)

    JSON.parse(response.body)
  rescue IOError, SystemCallError, SocketError, Timeout::Error, OpenSSL::SSL::SSLError
    raise 'winget API transport failed', cause: nil
  end
end

if $PROGRAM_NAME == __FILE__
  raise 'usage: publish_winget.rb [--check]' unless ARGV.empty? || ARGV == ['--check']

  tag = ENV.fetch('RELEASE_TAG')
  files, checksums = WingetPublisher.read_artifacts('.release-check/windows')
  WingetPublisher.validate(tag, files, checksums)
  if ARGV == ['--check']
    puts 'PASS: stable winget manifests match the verified Windows archives'
  else
    WingetPublisher.new(token: ENV.fetch('GH_TOKEN')).publish(tag, files, checksums)
    puts 'Updated winget fork version branch; no upstream PR opened'
  end
end
