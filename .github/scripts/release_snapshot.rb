require 'fileutils'
require 'json'
require 'yaml'

Dir.chdir(File.expand_path('../..', __dir__))
mode, channel = ARGV
raise 'usage: release_snapshot.rb unix|windows|windows-render-only stable|rc; or defer-homebrew' unless
  (ARGV == ['defer-homebrew']) ||
  (ARGV.length == 2 && %w[unix windows windows-render-only].include?(mode) && %w[stable rc].include?(channel))

FileUtils.mkdir_p('.release-check')
if mode == 'defer-homebrew'
  # GoReleaser places custom_block first; native brew style must normalize it
  # before the separate, stable-only tap job can publish the cask.
  config = YAML.safe_load(File.read('.goreleaser.yaml'))
  config.fetch('homebrew_casks').each { |cask| cask['skip_upload'] = true }
  File.write('.release-check/release.yaml', YAML.dump(config))
  exit
end

windows = mode.start_with?('windows')
native_windows = RUBY_PLATFORM.match?(/mswin|mingw/)
raise 'windows packaging requires native Windows; use windows-render-only for cross-builds' if mode == 'windows' && !native_windows
if mode == 'windows' && channel == 'stable'
  raise 'native choco is required; no package is fabricated' unless system('choco', '--version')
end

config = YAML.safe_load(File.read(windows ? '.goreleaser-windows.yaml' : '.goreleaser.yaml'))
config['dist'] = ".release-check/#{mode}-#{channel}"
config['snapshot'] = { 'version_template' => '{{ .Version }}' }
config.fetch('release')['disable'] = true
config.fetch('homebrew_casks', []).each { |cask| cask['skip_upload'] = true }
config.fetch('winget', []).each { |winget| winget['skip_upload'] = true }
config.fetch('chocolateys', []).each { |choco| choco['skip_publish'] = true }
path = ".release-check/#{mode}-#{channel}.yaml"
File.write(path, YAML.dump(config))
skip = %w[publish announce before]
skip << 'chocolatey' if channel == 'rc' || mode == 'windows-render-only'
environment = {
  'GITHUB_TOKEN' => nil, 'GH_TOKEN' => nil, 'GITLAB_TOKEN' => nil, 'GITEA_TOKEN' => nil,
  'HOMEBREW_TAP_TOKEN' => nil, 'WINGET_TOKEN' => nil, 'CHOCO_API_KEY' => nil,
  'RELEASE_MAKE_LATEST' => 'false', 'SYFT_CHECK_FOR_APP_UPDATE' => 'false',
  'GORELEASER_CURRENT_TAG' => channel == 'rc' ? '2026.11.0-rc.1' : '2026.11.0'
}
raise 'publisher-disabled snapshot failed' unless system(
  environment, 'goreleaser', 'release', '--snapshot', "--skip=#{skip.join(',')}", '--clean', '--config', path
)

dist = config.fetch('dist')
archives = Dir.glob("#{dist}/*.{tar.gz,zip}")
raise 'missing snapshot archives' unless archives.length == (windows ? 2 : 5)
raise 'missing archive SBOMs' unless Dir.glob("#{dist}/*.sbom.json").length >= archives.length
if windows
  manifests = Dir.glob("#{dist}/**/*.installer.yaml")
  raise 'missing real GoReleaser winget manifest' unless manifests.length == 1
  manifest = YAML.safe_load(File.read(manifests.first))
  raise 'winget dependency floor' unless manifest.fetch('Dependencies').fetch('PackageDependencies') ==
                                        [{ 'PackageIdentifier' => 'Microsoft.AzureCLI', 'MinimumVersion' => '2.61.0' }]
  year = manifest.fetch('ReleaseDate').split('-').first
  locale = YAML.safe_load(File.read(Dir.glob("#{dist}/winget/**/*.locale.en-US.yaml").fetch(0)))
  raise 'winget release-year copyright' unless locale.fetch('Copyright') == "Copyright #{year} white duck GmbH"
  Dir.glob("#{dist}/winget/**/*.yaml").each do |file|
    File.write("#{file}.json", JSON.generate(YAML.safe_load(File.read(file))))
  end
  if mode == 'windows' && channel == 'stable'
    packages = Dir.glob("#{dist}/*.nupkg")
    raise 'native Chocolatey did not produce a package' unless packages.length == 1 && File.size?(packages.first)
    nuspec = File.read("#{dist}/azctx.choco/azctx.nuspec")
    raise 'Chocolatey metadata' unless nuspec.include?('<authors>white duck GmbH</authors>') &&
                                       nuspec.include?('<owners>white duck GmbH</owners>') &&
                                       nuspec.include?('id="azure-cli" version="2.61.0"') &&
                                       nuspec.include?("Copyright #{year} white duck GmbH")
    puts "PASS: native choco pack produced #{packages.first}"
  else
    raise 'unexpected Chocolatey output' unless Dir.glob("#{dist}/*.nupkg").empty?
    puts 'Chocolatey generation skipped; no native package was generated or validated'
  end
else
  cask = File.read("#{dist}/homebrew/Casks/azctx.rb")
  raise 'missing declarative quarantine step' unless cask.include?('postflight_steps do') &&
                                                  cask.include?('"{{staged_path}}/azctx"')
  raise 'unexpected Linux cask' if cask.include?('on_linux')
  puts "PASS: real GoReleaser cask rendered at #{dist}/homebrew/Casks/azctx.rb (not yet style-normalized)"
end
puts "PASS: #{mode}/#{channel} archives and SBOMs; GitHub, Homebrew, winget and Chocolatey publishers disabled"
