# Ephemeral Windows CI only. Test execution has a standard-user token.
param([Parameter(Mandatory=$true)][string]$Repository, [Parameter(Mandatory=$true)][string]$Node, [switch]$PreflightOnly)
$ErrorActionPreference = 'Stop'
# This is a fresh disposable CI account. Use its actual shell folder rather
# than the shared runner work drive, whose parent ACL may permit replacement.
$nativeLocalData = [Environment]::GetFolderPath([Environment+SpecialFolder]::LocalApplicationData)
if (-not $nativeLocalData) { throw 'Standard-user local application data is unavailable' }
$nativeScratch = Join-Path $nativeLocalData ('Temp\gestalt-native-' + [Guid]::NewGuid().ToString('N'))
New-Item -ItemType Directory -Path $nativeScratch -Force | Out-Null
$env:TMP = $nativeScratch
$env:TEMP = $nativeScratch
$env:GESTALT_NATIVE_CODEX_FIXTURE = Join-Path $nativeLocalData ('GestaltNativeCodex-' + [Guid]::NewGuid().ToString('N'))
New-Item -ItemType Directory -Path $env:GESTALT_NATIVE_CODEX_FIXTURE | Out-Null
Set-Location -LiteralPath $Repository
$env:GESTALT_NATIVE_TEST_EVIDENCE = Join-Path $Repository 'native-evidence'
& $Node tests/native-windows-terminal-preflight.mjs
if ($LASTEXITCODE -ne 0) { exit $LASTEXITCODE }
if ($PreflightOnly) {
  Write-Output 'Correction lane: preflight only; full adapter/ownership/Codex and repository acceptance remain unexecuted.'
  exit 0
}
& $Node --test --test-reporter=tap tests/native-platform.test.mjs tests/native-process.test.mjs tests/native-terminal.test.mjs tests/native-harness.test.mjs
if ($LASTEXITCODE -ne 0) { exit $LASTEXITCODE }
& $Node tests/native-codex-terminal.mjs
exit $LASTEXITCODE
