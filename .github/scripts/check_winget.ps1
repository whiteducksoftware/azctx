param([Parameter(Mandatory)][string]$Directory)

$ErrorActionPreference = 'Stop'
$files = @(Get-ChildItem -LiteralPath $Directory -Filter '*.yaml.json' -Recurse)
if ($files.Count -ne 3) { throw 'expected three real GoReleaser winget manifests converted to JSON' }
foreach ($file in $files) {
    $json = Get-Content -Raw -LiteralPath $file.FullName
    $manifest = $json | ConvertFrom-Json
    $type = $manifest.ManifestType
    $version = $manifest.ManifestVersion
    if ($type -notin @('installer', 'version', 'defaultLocale') -or $version -notmatch '^\d+\.\d+\.\d+$') {
        throw 'unexpected winget schema identifier'
    }
    $schema = Join-Path $Directory "manifest.$type.$version.schema.json"
    Invoke-WebRequest -Uri "https://aka.ms/winget-manifest.$type.$version.schema.json" -OutFile $schema
    if (-not (Test-Json -Json $json -SchemaFile $schema)) { throw "invalid winget $type manifest" }
    Write-Output "PASS: $($file.Name) against Microsoft's winget $type/$version JSON schema"
}
