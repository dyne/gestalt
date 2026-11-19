$ErrorActionPreference = 'Stop'
if ($env:GITHUB_ACTIONS -ne 'true' -or $env:RUNNER_OS -ne 'Windows' -or $env:FINAL_RUNNER_ENVIRONMENT -ne 'github-hosted') {
  throw 'Use only the disposable hosted Windows test job'
}
$identity = [Security.Principal.WindowsIdentity]::GetCurrent()
$principal = New-Object Security.Principal.WindowsPrincipal($identity)
if (-not $principal.IsInRole([Security.Principal.WindowsBuiltInRole]::Administrator)) { throw 'Hosted account fixture setup requires its existing administrator token' }
$evidence = Join-Path $env:GITHUB_WORKSPACE 'native-evidence/final-current/windows-consumer'
$builder = Get-Content -LiteralPath (Join-Path $evidence 'result.json') -Raw | ConvertFrom-Json
if ($builder.state -ne 'pass' -or $builder.head -ne $env:GITHUB_SHA -or $builder.run -ne $env:GITHUB_RUN_ID -or $builder.attempt -ne $env:GITHUB_RUN_ATTEMPT) {
  throw 'Verified builder phase must precede consumer account preparation'
}
$name = 'gs' + [Guid]::NewGuid().ToString('N').Substring(0, 12)
$password = ConvertTo-SecureString ([Guid]::NewGuid().ToString('N') + '!aA1') -AsPlainText -Force
$work = Join-Path $env:RUNNER_TEMP $name
$source = Join-Path $work 'source'
$process = $null; $user = $null; $credential = $null; $observed = $false; $attemptedLaunch = $false; $exitCode = $null
$accountRemoval = 'not-attempted'
$gate = $null; $launchAt = $null; $cutoff = $null
try {
  $user = New-LocalUser -Name $name -Password $password -AccountNeverExpires
  $users = Get-LocalGroup -SID 'S-1-5-32-545'
  Add-LocalGroupMember -Group $users -Member $user
  New-Item -ItemType Directory -Path $work -ErrorAction Stop | Out-Null
  # Absolute workspace exclusions preserve tracked or dependency dist leaves.
  $ignoredEvidence = Join-Path $env:GITHUB_WORKSPACE 'native-evidence'
  $ignoredDist = Join-Path $env:GITHUB_WORKSPACE 'dist'
  & "$env:SystemRoot\System32\robocopy.exe" $env:GITHUB_WORKSPACE $source /E /XJ /XD .git $ignoredEvidence $ignoredDist /XF .npmrc /NFL /NDL /NJH /NJS /NP
  if ($LASTEXITCODE -gt 7) { throw 'Candidate and lockfile dependencies copy failed' }
  $bind = Join-Path $source 'native-evidence/binding'
  New-Item -ItemType Directory -Path $bind -Force | Out-Null
  foreach ($file in @('result.json','manager-inventory.json','agents-inventory.json','package-binding.json')) {
    Copy-Item -LiteralPath (Join-Path $evidence $file) -Destination $bind
  }
  Copy-Item -LiteralPath (Join-Path $evidence 'prebuilt') -Destination (Join-Path $source 'native-evidence/prebuilt') -Recurse
  $tools = Join-Path $work 'tools'
  Copy-Item -LiteralPath (Join-Path $evidence 'private-toolchain/bin') -Destination $tools -Recurse
  $node = Join-Path $tools 'node.exe'
  $gitCursor = Split-Path -Parent (Get-Command git.exe).Source
  $git = $null
  for ($i = 0; $i -lt 4; $i++) {
    $candidate = Join-Path $gitCursor 'cmd\git.exe'
    if (Test-Path -LiteralPath $candidate -PathType Leaf) { $git = $candidate; break }
    $next = Split-Path -Parent $gitCursor
    if (-not $next -or $next -eq $gitCursor) { break }
    $gitCursor = $next
  }
  if (-not $git) { throw 'Native Git cmd executable missing' }
  & "$env:SystemRoot\System32\icacls.exe" $work /grant ('*' + $user.SID.Value + ':(OI)(CI)F') /T /Q | Out-Null
  if ($LASTEXITCODE -ne 0) { throw 'Fixture-only standard account access failed' }
  $script = Join-Path $source 'tests/final-current-transport/standard-user.ps1'
  foreach ($argument in @($script,$source,$node,$git)) {
    if ($argument.IndexOf('"') -ne -1) { throw 'Unexpected quote in a known hosted path' }
  }
  $arguments = '-NoLogo -NoProfile -NonInteractive -File "' + $script + '" -Repository "' + $source + '" -Node "' + $node + '" -Git "' + $git + '"'
  $credential = New-Object Management.Automation.PSCredential(($env:COMPUTERNAME + '\' + $name), $password)
  $allowed = '^(SystemRoot|WINDIR|ComSpec|SystemDrive|ProgramFiles(\(x86\))?|ProgramW6432|ProgramData|Public|USERPROFILE|LOCALAPPDATA|APPDATA|USERDOMAIN|USERNAME|HOMEDRIVE|HOMEPATH|PATH|PATHEXT|TEMP|TMP|PROCESSOR_ARCHITECTURE|NUMBER_OF_PROCESSORS|OS)$'
  $saved = @{}
  foreach ($entry in @(Get-ChildItem Env:)) {
    if ($entry.Name -notmatch $allowed) {
      $saved[$entry.Name] = $entry.Value
      [Environment]::SetEnvironmentVariable($entry.Name, $null, 'Process')
    }
  }
  try {
    $attemptedLaunch = $true
    $launchAt = [DateTimeOffset]::UtcNow
    $gate = [Diagnostics.Stopwatch]::StartNew()
    $cutoff = $launchAt.AddMinutes(45).ToString('o')
    $arguments += ' -GateDeadlineUtc "' + $cutoff + '"'
    $process = Start-Process -FilePath "$env:SystemRoot\System32\WindowsPowerShell\v1.0\powershell.exe" -Credential $credential -LoadUserProfile -ArgumentList $arguments -PassThru -RedirectStandardOutput (Join-Path $work 'consumer.stdout') -RedirectStandardError (Join-Path $work 'consumer.stderr')
  } finally {
    foreach ($entry in $saved.GetEnumerator()) { [Environment]::SetEnvironmentVariable($entry.Key, $entry.Value, 'Process') }
    $saved.Clear()
  }
  $handle = $process.Handle
  if ($handle -eq [IntPtr]::Zero -or $handle -eq [IntPtr](-1)) { throw 'Consumer direct process handle unavailable' }
  $observed = $process.WaitForExit([Math]::Max(0, 2760000 - [int]$gate.ElapsedMilliseconds))
  if (-not $observed) { throw 'Consumer deadline; preserve remaining state for hosted runner disposal' }
  $exitCode = $process.ExitCode
  if ($null -eq $exitCode -or $exitCode -ne 0) { throw "Consumer failed: $exitCode" }
} finally {
  foreach ($file in @('consumer.stdout','consumer.stderr')) {
    $partial = Join-Path $work $file
    if (Test-Path -LiteralPath $partial) { Copy-Item -LiteralPath $partial -Destination $evidence }
  }
  $partial = Join-Path $source 'native-evidence/final-consumer'
  if (Test-Path -LiteralPath $partial) { Copy-Item -LiteralPath $partial -Destination $evidence -Recurse }
  if ($process) { $process.Dispose() }
  if ($user -and (($observed -and $exitCode -eq 0) -or -not $attemptedLaunch)) {
    try { Remove-LocalUser -Name $name -ErrorAction Stop; $accountRemoval = 'removed' }
    catch { $accountRemoval = 'retained for hosted runner disposal'; Write-Warning 'Disposable account removal was not confirmed' }
  } elseif ($user) { $accountRemoval = 'retained for hosted runner disposal' }
  @{observedAt=[DateTime]::UtcNow.ToString('o'); attemptedLaunch=$attemptedLaunch; directExitObserved=$observed; exit=$exitCode;
    launchAt=$(if ($launchAt) { $launchAt.ToString('o') } else { $null }); gateDeadlineUtc=$cutoff;
    elapsedMs=$(if ($gate) { $gate.ElapsedMilliseconds } else { $null }); directGateMs=2760000;
    accountRemoval=$accountRemoval; workspace=$work; inheritance='allowlisted OS/profile variables only, no checkout credentials copied';
    ownershipScope='Direct CI process result only. No inferred product descendant release.' } | ConvertTo-Json -Depth 4 |
      Set-Content -LiteralPath (Join-Path $evidence 'consumer-harness.json') -Encoding UTF8
}
