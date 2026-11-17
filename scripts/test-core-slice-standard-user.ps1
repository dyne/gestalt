# Disposable CI account only. Artifacts were built before this consumer token.
param([Parameter(Mandatory=$true)][string]$Repository, [Parameter(Mandatory=$true)][string]$Node, [Parameter(Mandatory=$true)][string]$GitDirectory)
$ErrorActionPreference = 'Stop'
$principal = New-Object Security.Principal.WindowsPrincipal([Security.Principal.WindowsIdentity]::GetCurrent())
$administrator = $principal.IsInRole([Security.Principal.WindowsBuiltInRole]::Administrator)
if ($administrator) { throw 'Core slice requires a standard-user token' }
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
Set-Location -LiteralPath $Repository
$evidence = Join-Path $Repository 'native-evidence/core-slice'
New-Item -ItemType Directory -Path $evidence -Force | Out-Null
$forbidden = @('bash.exe', 'sh.exe', 'wsl.exe', 'wslconfig.exe', 'lxrun.exe', 'cl.exe', 'gcc.exe', 'clang.exe', 'tsc.cmd')
function Write-ExecutableDiscovery([string]$Name) {
  $resolved = @()
  foreach ($command in $forbidden + @('node.exe', 'npm.cmd', 'git.exe', 'powershell.exe', 'cmd.exe')) {
    foreach ($entry in @(Get-Command $command -All -ErrorAction SilentlyContinue)) {
      $item = Get-Item -LiteralPath $entry.Source -ErrorAction Stop
      $resolved += @{requested=$command; path=$entry.Source; commandType=$entry.CommandType.ToString(); bytes=$item.Length; sha256=(Get-FileHash -LiteralPath $entry.Source -Algorithm SHA256).Hash; description=$item.VersionInfo.FileDescription; product=$item.VersionInfo.ProductName}
    }
  }
  @{observedAt=[DateTime]::UtcNow.ToString('o'); pathEntries=@($env:PATH -split ';'); comSpec=$env:ComSpec; resolutions=$resolved} | ConvertTo-Json -Depth 5 | Set-Content -LiteralPath (Join-Path $evidence $Name)
}
# Retain the exact offending executable even if a future assertion fails. The
# old run captured its name only; it cannot establish that it was a WSL stub.
$env:PATH = $nodeDirectory + ';' + $gitCommandDirectory + ';' + $env:SystemRoot + '\System32;' + $env:SystemRoot + '\System32\WindowsPowerShell\v1.0;' + $env:SystemRoot
Write-ExecutableDiscovery 'executable-discovery-before.json'
# Fixed native OS helpers and the Codex hook shell use absolute system paths.
# Exposing all System32 also exposes optional Bash/WSL aliases on some images.
$env:ComSpec = Join-Path $env:SystemRoot 'System32\cmd.exe'
if (-not (Test-Path -LiteralPath $env:ComSpec -PathType Leaf)) { throw 'Absolute native cmd.exe is required for the existing hook contract' }
$env:PATH = $nodeDirectory + ';' + $gitCommandDirectory + ';' + $env:SystemRoot + '\System32\WindowsPowerShell\v1.0'
Write-ExecutableDiscovery 'executable-discovery-after.json'
foreach ($command in $forbidden) {
  $resolved = @(Get-Command $command -All -ErrorAction SilentlyContinue)
  if ($resolved.Count) { throw ('Consumer PATH unexpectedly provides ' + $command + ': ' + (($resolved | ForEach-Object { $_.Source }) -join ', ')) }
}
$developmentModeKey = 'HKLM:\SOFTWARE\Microsoft\Windows\CurrentVersion\AppModelUnlock'
$developmentModeName = 'AllowDevelopmentWithoutDevLicense'
$developmentModeKind = (Get-Item -LiteralPath $developmentModeKey).GetValueKind($developmentModeName).ToString()
$developmentMode = Get-ItemPropertyValue -LiteralPath $developmentModeKey -Name $developmentModeName
@{observedAt=[DateTime]::UtcNow.ToString('o'); sid=[Security.Principal.WindowsIdentity]::GetCurrent().User.Value; administrator=$administrator; standardUser=$true; noBashPath=$true; noConsumerCompilerPath=$true; developerMode=$developmentMode; developerModeKind=$developmentModeKind; node=$Node; git=$nativeGit; pathEntries=@($env:PATH -split ';'); TMPDIR=$env:TMPDIR; TMP=$env:TMP; TEMP=$env:TEMP; context=$env:GESTALT_SESSION_TMP_CONTEXT} | ConvertTo-Json -Depth 4 | Set-Content -LiteralPath (Join-Path $evidence 'consumer-token.json')
if ($developmentModeKind -ne 'DWord' -or $developmentMode -ne 0) { throw 'Native core slice requires explicit Developer Mode DWORD0 readback' }
& $Node tests/fixtures/native-no-devmode.mjs (Join-Path $evidence 'developer-mode-negative-capability.json')
if ($LASTEXITCODE -ne 0) { throw 'Actual ordinary-user no-Developer-Mode capability check failed' }
& $Node --test --test-reporter=tap '--test-name-pattern=shared capability deadline' tests/native-capabilities.test.mjs
if ($LASTEXITCODE -ne 0) { throw 'Controlled capability deadline preflight failed; parity/readiness unexecuted' }
& $Node tests/native-storage-read-parity.mjs --artifacts native-evidence/core-slice-prebuilt --evidence (Join-Path $evidence 'storage-parity')
if ($LASTEXITCODE -ne 0) { throw 'Required native storage ACL parity preflight failed; readiness unexecuted' }
$env:GESTALT_CORE_SLICE_READINESS_ONLY = '1'
$env:GESTALT_CORE_SLICE_READINESS_COMPARISON = '1'
& $Node tests/native-core-slice.mjs --artifacts native-evidence/core-slice-prebuilt --agents .native-dependencies/agents --evidence $evidence
$result = $LASTEXITCODE
if ($null -eq $result) { throw 'Native core slice exit code was not observed' }
exit $result
