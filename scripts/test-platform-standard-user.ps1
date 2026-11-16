# Ephemeral Windows CI only. Test execution has a standard-user token.
param([Parameter(Mandatory=$true)][string]$Repository, [Parameter(Mandatory=$true)][string]$Node, [ValidateSet('acceptance','windows-correction')][string]$Lane = 'acceptance')
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
New-Item -ItemType Directory -Path $env:GESTALT_NATIVE_TEST_EVIDENCE -Force | Out-Null
$env:GESTALT_NATIVE_BUILD_DIAGNOSTICS = '1'
function Write-RunnerProgress([string]$Stage, [object]$Details) {
  $row = @{observedAt=[DateTime]::UtcNow.ToString('o'); harnessPid=$PID; stage=$Stage; details=$Details} | ConvertTo-Json -Compress
  $bytes = [Text.Encoding]::UTF8.GetBytes($row + "`n")
  $file = [IO.File]::Open((Join-Path $env:GESTALT_NATIVE_TEST_EVIDENCE 'standard-user-progress.jsonl'), [IO.FileMode]::Append, [IO.FileAccess]::Write, [IO.FileShare]::ReadWrite)
  try { $file.Write($bytes, 0, $bytes.Length); $file.Flush($true) } finally { $file.Dispose() }
}
Write-RunnerProgress 'preflight-before' $null
& $Node tests/native-windows-terminal-preflight.mjs
$preflightExit = $LASTEXITCODE
Write-RunnerProgress 'preflight-returned' @{exit=$preflightExit}
if ($null -eq $preflightExit) { throw 'Preflight exit code was not observed' }
if ($preflightExit -ne 0) { exit $preflightExit }
Write-RunnerProgress 'contracts-before' @{lane=$Lane}
if ($Lane -eq 'windows-correction') {
  Write-Output 'Correction lane: primary preflight, focused ownership/terminal/harness contracts and actual pinned Codex TUI. Full storage/native-platform, Unix and repository acceptance remain UNEXECUTED.'
  & $Node --test --test-reporter=tap tests/native-process.test.mjs tests/native-terminal.test.mjs tests/native-harness.test.mjs
} else {
  & $Node --test --test-reporter=tap tests/native-platform.test.mjs tests/native-process.test.mjs tests/native-terminal.test.mjs tests/native-harness.test.mjs
}
$contractsExit = $LASTEXITCODE
Write-RunnerProgress 'contracts-returned' @{exit=$contractsExit; lane=$Lane}
if ($null -eq $contractsExit) { throw 'Contract suite exit code was not observed' }
if ($contractsExit -ne 0) { exit $contractsExit }
Write-RunnerProgress 'codex-before' $null
& $Node tests/native-codex-terminal.mjs
$codexExit = $LASTEXITCODE
Write-RunnerProgress 'codex-returned' @{exit=$codexExit}
if ($null -eq $codexExit) { throw 'Codex terminal script exit code was not observed' }
exit $codexExit
