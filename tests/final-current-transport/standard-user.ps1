[CmdletBinding()]
param(
  [Parameter(Mandatory=$true)][string]$Repository,
  [Parameter(Mandatory=$true)][string]$Node,
  [Parameter(Mandatory=$true)][string]$Git,
  [Parameter(Mandatory=$true)][string]$GateDeadlineUtc
)
$ErrorActionPreference = 'Stop'
$keep = '^(SystemRoot|WINDIR|ComSpec|SystemDrive|ProgramFiles(\(x86\))?|ProgramW6432|ProgramData|Public|USERPROFILE|LOCALAPPDATA|APPDATA|USERDOMAIN|USERNAME|HOMEDRIVE|HOMEPATH|PATH|PATHEXT|TEMP|TMP|PROCESSOR_ARCHITECTURE|NUMBER_OF_PROCESSORS|OS)$'
foreach ($entry in @(Get-ChildItem Env:)) {
  if ($entry.Name -notmatch $keep) { [Environment]::SetEnvironmentVariable($entry.Name, $null, 'Process') }
}
$identity = [Security.Principal.WindowsIdentity]::GetCurrent()
$principal = New-Object Security.Principal.WindowsPrincipal($identity)
if ($principal.IsInRole([Security.Principal.WindowsBuiltInRole]::Administrator)) { throw 'Consumer must be a standard user' }
$cutoff = [DateTimeOffset]::Parse($GateDeadlineUtc, [Globalization.CultureInfo]::InvariantCulture)
if ([DateTimeOffset]::UtcNow -ge $cutoff) { throw 'Whole consumer observation cutoff expired before launch' }
$localData = [Environment]::GetFolderPath([Environment+SpecialFolder]::LocalApplicationData)
if (-not $localData) { throw 'Windows user-local profile could not be resolved' }
$scratch = Join-Path $localData ('Temp\gestalt-final-' + [Guid]::NewGuid().ToString('N'))
New-Item -ItemType Directory -Path $scratch -Force | Out-Null
Set-Location -LiteralPath $Repository
$evidence = Join-Path $Repository 'native-evidence/final-consumer'
New-Item -ItemType Directory -Path $evidence -Force | Out-Null
$env:TMPDIR = $scratch; $env:TMP = $scratch; $env:TEMP = $scratch
$userProfile = [Environment]::GetFolderPath([Environment+SpecialFolder]::UserProfile)
if (-not $userProfile) { throw 'Standard-user profile could not be resolved' }
$env:USERPROFILE = $userProfile
$env:HOME = $userProfile
$nodeDirectory = Split-Path -Parent $Node
$gitDirectory = Split-Path -Parent $Git
$env:ComSpec = Join-Path $env:SystemRoot 'System32\cmd.exe'
if (-not (Test-Path -LiteralPath $env:ComSpec -PathType Leaf)) { throw 'Native absolute cmd.exe is unavailable' }
$env:PATH = $nodeDirectory + ';' + $gitDirectory + ';' + $env:SystemRoot + '\System32\WindowsPowerShell\v1.0'
$env:PATHEXT = '.COM;.EXE;.BAT;.CMD'
$env:GIT_TERMINAL_PROMPT = '0'
$env:GIT_CONFIG_NOSYSTEM = '1'
$env:GIT_CONFIG_GLOBAL = 'NUL'
$env:npm_config_cache = Join-Path $scratch 'npm-cache'
$env:npm_config_userconfig = Join-Path $scratch 'npmrc'
$env:npm_config_globalconfig = 'NUL'
$env:npm_config_update_notifier = 'false'
$env:npm_config_registry = 'https://registry.npmjs.org/'
$env:FORCE_COLOR = '0'
Set-Content -LiteralPath $env:npm_config_userconfig -Value '' -Encoding Ascii
$forbidden = @('bash.exe','sh.exe','wsl.exe','wslconfig.exe','lxrun.exe','cl.exe','gcc.exe','clang.exe','tsc.cmd')
$resolutions = @{}
foreach ($name in $forbidden + @('node.exe','npm.cmd','git.exe','codex.exe','powershell.exe')) {
  $resolutions[$name] = @(Get-Command $name -All -ErrorAction SilentlyContinue | ForEach-Object { $_.Source })
}
@{observedAt=[DateTime]::UtcNow.ToString('o'); sid=$identity.User.Value; administrator=$false; pathEntries=@($env:PATH -split ';'); resolutions=$resolutions; scratch=$scratch; environmentScope='Allowlisted OS/profile paths and fixture-only tools'} |
  ConvertTo-Json -Depth 6 | Set-Content -LiteralPath (Join-Path $evidence 'consumer-token.json') -Encoding UTF8
foreach ($name in $forbidden) {
  if ($resolutions[$name].Count) { throw "Unexpected consumer command on PATH: $name" }
}
if ($resolutions['node.exe'][0] -ne $Node -or $resolutions['git.exe'][0] -ne $Git) { throw 'Consumer executable identity mismatch' }
$key = 'HKLM:\SOFTWARE\Microsoft\Windows\CurrentVersion\AppModelUnlock'
$property = 'AllowDevelopmentWithoutDevLicense'
$observed = @{observedAt=[DateTime]::UtcNow.ToString('o'); altered=$false; negativeSymlink='unexecuted'}
try {
  $registry = Get-Item -LiteralPath $key
  $observed.kind = $registry.GetValueKind($property).ToString()
  $observed.value = $registry.GetValue($property)
} catch { $observed.readError = $_.Exception.Message }
$observed.reason = 'Developer Mode is read-only evidence. Negative symlink capability is outside this R4 schedule.'
$observed | ConvertTo-Json -Depth 4 | Set-Content -LiteralPath (Join-Path $evidence 'developer-mode-observation.json') -Encoding UTF8

$env:FINAL_WINDOWS_DEADLINE_UTC = $cutoff.ToString('o')
$env:FINAL_WINDOWS_PHASE_LOG = Join-Path $evidence 'consumer-phases.ndjson'
& $Node --import ./tests/final-current-transport/phase-observer.mjs tests/final-current-transport/consumer.mjs
$result = $LASTEXITCODE
if ($null -eq $result) { throw 'Consumer exit was not observed' }
exit $result
