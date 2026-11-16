# Disposable CI account only. Artifacts were built before this consumer token.
param([Parameter(Mandatory=$true)][string]$Repository, [Parameter(Mandatory=$true)][string]$Node, [Parameter(Mandatory=$true)][string]$GitDirectory)
$ErrorActionPreference = 'Stop'
$principal = New-Object Security.Principal.WindowsPrincipal([Security.Principal.WindowsIdentity]::GetCurrent())
if ($principal.IsInRole([Security.Principal.WindowsBuiltInRole]::Administrator)) { throw 'Core slice requires a standard-user token' }
$localData = [Environment]::GetFolderPath([Environment+SpecialFolder]::LocalApplicationData)
$scratch = Join-Path $localData ('Temp\gestalt-core-slice-' + [Guid]::NewGuid().ToString('N'))
New-Item -ItemType Directory -Path $scratch -Force | Out-Null
# Fresh CI fixture only; never remap an inherited session owner.
if ($env:GESTALT_SESSION_TMP_CONTEXT) { throw 'CI fixture must not remap an inherited scratch owner' }
$env:TMPDIR = $scratch; $env:TMP = $scratch; $env:TEMP = $scratch
$nodeDirectory = Split-Path -Parent $Node
$cursor = [IO.Path]::GetFullPath($GitDirectory); $nativeGit = $null
for ($attempt = 0; $attempt -lt 4; $attempt++) {
  $candidate = Join-Path $cursor 'cmd\git.exe'
  if (Test-Path -LiteralPath $candidate -PathType Leaf) { $nativeGit = $candidate; break }
  $next = Split-Path -Parent $cursor
  if (-not $next -or $next -eq $cursor) { break }; $cursor = $next
}
if (-not $nativeGit) { throw 'Native Git cmd\git.exe is required without a Bash PATH entry' }
$gitCommandDirectory = Split-Path -Parent $nativeGit
$env:PATH = $nodeDirectory + ';' + $gitCommandDirectory + ';' + $env:SystemRoot + '\System32;' + $env:SystemRoot + '\System32\WindowsPowerShell\v1.0;' + $env:SystemRoot
foreach ($command in @('bash.exe', 'sh.exe', 'cl.exe', 'gcc.exe', 'clang.exe', 'tsc.cmd')) {
  if (Get-Command $command -ErrorAction SilentlyContinue) { throw "Consumer PATH unexpectedly provides $command" }
}
$developmentMode = Get-ItemPropertyValue -LiteralPath 'HKLM:\SOFTWARE\Microsoft\Windows\CurrentVersion\AppModelUnlock' -Name 'AllowDevelopmentWithoutDevLicense' -ErrorAction SilentlyContinue
if ($developmentMode -eq 1) { throw 'Native core slice must run without Developer Mode' }
Set-Location -LiteralPath $Repository
$evidence = Join-Path $Repository 'native-evidence/core-slice'
New-Item -ItemType Directory -Path $evidence -Force | Out-Null
@{observedAt=[DateTime]::UtcNow.ToString('o'); sid=[Security.Principal.WindowsIdentity]::GetCurrent().User.Value; standardUser=$true; noBashPath=$true; noConsumerCompilerPath=$true; developerMode=$developmentMode; node=$Node; git=$nativeGit; pathEntries=@($env:PATH -split ';'); TMPDIR=$env:TMPDIR; TMP=$env:TMP; TEMP=$env:TEMP; context=$env:GESTALT_SESSION_TMP_CONTEXT} | ConvertTo-Json -Depth 4 | Set-Content -LiteralPath (Join-Path $evidence 'consumer-token.json')
& $Node tests/native-core-slice.mjs --artifacts native-evidence/core-slice-prebuilt --agents .native-dependencies/agents --evidence $evidence
$result = $LASTEXITCODE
if ($null -eq $result) { throw 'Native core slice exit code was not observed' }
exit $result
