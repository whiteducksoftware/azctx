$verificationFilePath = Join-Path $PSScriptRoot './tools/VERIFICATION.txt'
$azctxBinaryPath = Join-Path $PSScriptRoot './tools/azctx_windows_amd64.zip'

$valueReplaceRegex = "(?<=Value:)\s*\w+"

$fileHash = (Get-FileHash $azctxBinaryPath | Select-Object -ExpandProperty Hash).ToLower()
[regex]::Replace((Get-Content $verificationFilePath -Raw), $valueReplaceRegex, " $fileHash") | Out-File $verificationFilePath -Encoding utf8

