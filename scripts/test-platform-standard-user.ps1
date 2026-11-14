# Ephemeral Windows CI only. Test execution has a standard-user token.
param([Parameter(Mandatory=$true)][string]$Repository, [Parameter(Mandatory=$true)][string]$Node)
$ErrorActionPreference = 'Stop'
# This is a fresh disposable CI account. Use its actual shell folder rather
# than the shared runner work drive, whose parent ACL may permit replacement.
$nativeLocalData = [Environment]::GetFolderPath([Environment+SpecialFolder]::LocalApplicationData)
if (-not $nativeLocalData) { throw 'Standard-user local application data is unavailable' }
$nativeScratch = Join-Path $nativeLocalData ('Temp\gestalt-native-' + [Guid]::NewGuid().ToString('N'))
New-Item -ItemType Directory -Path $nativeScratch -Force | Out-Null
$env:TMP = $nativeScratch
$env:TEMP = $nativeScratch
Set-Location -LiteralPath $Repository
& $Node --test --test-reporter=tap tests/native-platform.test.mjs
exit $LASTEXITCODE
