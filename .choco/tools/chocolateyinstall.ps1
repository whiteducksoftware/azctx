$toolsDir = (Split-Path -parent $MyInvocation.MyCommand.Definition)
$filePath = Join-Path $toolsDir 'azctx_windows_amd64.zip'

Get-ChocolateyUnzip -FileFullPath "$filePath" -Destination $toolsDir
Remove-Item "$filePath"