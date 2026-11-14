# Ephemeral Windows CI only. Test execution has a standard-user token.
param([Parameter(Mandatory=$true)][string]$Repository, [Parameter(Mandatory=$true)][string]$Node, [Parameter(Mandatory=$true)][string]$Scratch)
$ErrorActionPreference = 'Stop'
New-Item -ItemType Directory -Path $Scratch -Force | Out-Null
$env:TMP = $Scratch
$env:TEMP = $Scratch
Set-Location -LiteralPath $Repository
& $Node --test --test-reporter=tap tests/native-platform.test.mjs
exit $LASTEXITCODE
