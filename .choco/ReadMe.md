# Instruction

* Copy the binary to ./tools/azctx_windows_amd64.zip
* Run the ./updateSHA.ps1 script to update ./tools/VERIFICATION.txt
* Run the following PWSH command inside the root directory to push the package (enter API Key first)

```bash
$ApiKey = 'EnterYourKey'

$packagePath = Get-Item *.nupkg | Sort-Object LastWriteTime -Descending | Select-Object -expand Name -first 1 

choco pack
choco apikey --key $ApiKey --source https://push.chocolatey.org/
choco push $packagePath --source https://push.chocolatey.org/
```
