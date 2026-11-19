param([switch]$Consumer, [string]$Repository, [string]$Node, [string]$Git, [string]$GateDeadlineUtc)
$ErrorActionPreference = 'Stop'
$allowed = '^(SystemRoot|WINDIR|ComSpec|SystemDrive|ProgramFiles(\(x86\))?|ProgramW6432|ProgramData|Public|USERPROFILE|LOCALAPPDATA|APPDATA|USERDOMAIN|USERNAME|HOMEDRIVE|HOMEPATH|PATH|PATHEXT|TEMP|TMP|PROCESSOR_ARCHITECTURE|NUMBER_OF_PROCESSORS|OS)$'
$identity = [Security.Principal.WindowsIdentity]::GetCurrent()
$principal = New-Object Security.Principal.WindowsPrincipal($identity)
$admin = $principal.IsInRole([Security.Principal.WindowsBuiltInRole]::Administrator)
$powerShell = Join-Path $env:SystemRoot 'System32\WindowsPowerShell\v1.0\powershell.exe'
function Write-Receipt($Destination, $Value) {
  $Value | ConvertTo-Json -Depth 8 | Set-Content -LiteralPath $Destination -Encoding UTF8
}
function File-Sha($File) { (Get-FileHash -LiteralPath $File -Algorithm SHA256).Hash.ToLowerInvariant() }

if ($Consumer) {
  if ($admin) { throw 'The probe requires a standard user' }
  foreach ($entry in @(Get-ChildItem Env:)) {
    if ($entry.Name -notmatch $allowed) { [Environment]::SetEnvironmentVariable($entry.Name, $null, 'Process') }
  }
  $evidence = Join-Path $Repository 'native-evidence/windows-native-read-pair'
  $cutoff = [DateTimeOffset]::Parse($GateDeadlineUtc, [Globalization.CultureInfo]::InvariantCulture)
  if ([DateTimeOffset]::UtcNow -ge $cutoff) { throw 'Observation gate expired before fixture provisioning' }
  $env:PROBE_GATE_DEADLINE_UTC = $cutoff.ToString('o')
  $profilePath = [Environment]::GetFolderPath([Environment+SpecialFolder]::UserProfile)
  $localData = [Environment]::GetFolderPath([Environment+SpecialFolder]::LocalApplicationData)
  if (-not $profilePath -or -not $localData) { throw 'Standard-user profile unavailable' }
  $scratch = Join-Path $localData ('Temp\gestalt-final-' + [Guid]::NewGuid().ToString('N'))
  New-Item -ItemType Directory -Path $scratch -Force | Out-Null
  $env:USERPROFILE = $profilePath
  $env:HOME = $profilePath
  $env:TMPDIR = $scratch
  $env:TMP = $scratch
  $env:TEMP = $scratch
  $env:PATH = (Split-Path -Parent $Node) + ';' + (Split-Path -Parent $Git) + ';' + (Split-Path -Parent $powerShell)
  $env:PATHEXT = '.COM;.EXE;.BAT;.CMD'
  $forbidden = @('bash.exe','sh.exe','wsl.exe','cl.exe','gcc.exe','clang.exe','npm.cmd','codex.exe')
  $resolved = @{}
  foreach ($name in $forbidden + @('node.exe','git.exe','powershell.exe')) {
    $resolved[$name] = @(Get-Command $name -All -ErrorAction SilentlyContinue | ForEach-Object { $_.Source })
  }
  Write-Receipt (Join-Path $evidence 'token.json') @{
    observedAt=[DateTime]::UtcNow.ToString('o'); administrator=$admin; sid=$identity.User.Value;
    profile=$profilePath; localAppData=$localData; scratch=$scratch; systemRoot=$env:SystemRoot;
    powershell=@{ path=$powerShell; version=$PSVersionTable.PSVersion.ToString(); sha256=(File-Sha $powerShell) };
    node=@{ path=$Node; sha256=(File-Sha $Node) };
    pathEntries=@($env:PATH -split ';'); resolutions=$resolved
  }
  foreach ($name in $forbidden) {
    if ($resolved[$name].Count) { throw "Unexpected command on the consumer PATH: $name" }
  }
  if ($resolved['node.exe'][0] -ne $Node -or $resolved['git.exe'][0] -ne $Git -or $resolved['powershell.exe'][0] -ne $powerShell) {
    throw 'Consumer executable resolution mismatch'
  }
  & $Node (Join-Path $Repository 'tests/windows-native-read-pair/probe.mjs')
  $exitCode = $LASTEXITCODE
  if ($exitCode -eq 0) { [IO.Directory]::Delete($scratch, $false) }
  exit $exitCode
}

if ($env:GITHUB_ACTIONS -ne 'true' -or $env:RUNNER_OS -ne 'Windows' -or $env:PROBE_RUNNER_ENVIRONMENT -ne 'github-hosted' -or -not $admin) {
  throw 'Use the disposable hosted Windows fixture'
}
$repo = $env:GITHUB_WORKSPACE
$evidence = Join-Path $repo 'native-evidence/windows-native-read-pair'
New-Item -ItemType Directory -Path $evidence -Force | Out-Null
$name = 'gs' + [Guid]::NewGuid().ToString('N').Substring(0, 12)
$work = Join-Path $env:RUNNER_TEMP $name
$source = Join-Path $work 'source'
$copyEvidence = Join-Path $source 'native-evidence/windows-native-read-pair'
$process = $null
$user = $null
$attempted = $false
$observed = $false
$status = $null
$account = 'not-created'
$gate = $null
$launchAt = $null
try {
  $head = (& git -C $repo rev-parse HEAD).Trim()
  $parent = (& git -C $repo rev-parse 'HEAD^').Trim()
  $tree = (& git -C $repo rev-parse 'HEAD^{tree}').Trim()
  if ($head -ne $env:GITHUB_SHA -or $parent -ne '283c74bfee93ca05bd4c82dd9608429434cff490') { throw 'Unexpected transport source' }
  $paths = @(
    'src/cli/platform/windows-system.ts', 'src/cli/runtime/windows-scratch-read.mjs',
    'tests/windows-native-read-pair/probe.mjs', 'tests/windows-native-read-pair/standard-user.ps1'
  )
  $files = @{}
  foreach ($relative in $paths) {
    $original = Join-Path $repo $relative
    $destination = Join-Path $source $relative
    New-Item -ItemType Directory -Path (Split-Path -Parent $destination) -Force | Out-Null
    Copy-Item -LiteralPath $original -Destination $destination
    $expectedBlob = (& git -C $repo rev-parse ("HEAD:" + $relative)).Trim()
    $copiedBlob = (& git -C $repo hash-object --no-filters -- $destination).Trim()
    if ($LASTEXITCODE -ne 0 -or $expectedBlob -ne $copiedBlob) { throw 'Copied probe source differs from tested object' }
    $files[$relative] = File-Sha $destination
  }
  New-Item -ItemType Directory -Path $copyEvidence -Force | Out-Null
  $tools = Join-Path $work 'tools'
  New-Item -ItemType Directory -Path $tools | Out-Null
  $nodePath = Join-Path $tools 'node.exe'
  Copy-Item -LiteralPath (Get-Command node.exe).Source -Destination $nodePath
  $nodeDigest = File-Sha $nodePath
  if ($nodeDigest -ne '86fc4785327db5f5c7558dc6156c543b433c313e813a222e088086222ab4afa6') { throw 'Node bytes differ from R1 consumer builder' }
  $artifact = Join-Path $repo 'native-evidence/windows-native-read-input'
  $pinned = @{
    'gestalt-owned-process.exe'='e3aa02f6ea0e1e559510d1ab2ce4bd08b1745086ea6705296ed4c9ad0dfd9af3';
    'gestalt-owned-process.exe.provenance.json'='348e2519d02cf8b9cbae5280e694584e1564267bd717dfc4fc8ccadaa5b9e238';
    'helper-receipt.json'='0fbd9ab65ffe706d7ac4fcca5c580a1d143f3530f21de7986c40f2dfe0dc4c47'
  }
  $artifactCopy = Join-Path $source 'verified-helper'
  New-Item -ItemType Directory -Path $artifactCopy | Out-Null
  foreach ($file in $pinned.Keys) {
    $from = Join-Path $artifact $file
    $to = Join-Path $artifactCopy $file
    if ((File-Sha $from) -ne $pinned[$file]) { throw 'Unexpected retained artifact bytes' }
    Copy-Item -LiteralPath $from -Destination $to
    if ((File-Sha $to) -ne $pinned[$file]) { throw 'Copied retained artifact bytes differ' }
  }
  $helperReceipt = Get-Content -LiteralPath (Join-Path $artifactCopy 'helper-receipt.json') -Raw | ConvertFrom-Json
  $provenance = Get-Content -LiteralPath (Join-Path $artifactCopy 'gestalt-owned-process.exe.provenance.json') -Raw | ConvertFrom-Json
  if ($helperReceipt.run -ne '44440431389' -or $helperReceipt.attempt -ne '1' -or
      $helperReceipt.head -ne '3235ee2216c7a1e09177f0dbf482ebee350efadc' -or
      $helperReceipt.source.Count -ne 3 -or $provenance.sourceInputs.Count -ne 3 -or
      $provenance.artifactSha256 -ne $pinned['gestalt-owned-process.exe'] -or
      $helperReceipt.helper.sha256 -ne $pinned['gestalt-owned-process.exe'] -or
      $helperReceipt.provenance.sha256 -ne $pinned['gestalt-owned-process.exe.provenance.json']) { throw 'Retained R1 identity mismatch' }
  $expectedPaths = @('src/cli/platform/native/owned-process.c','src/cli/platform/native/windows-storage-check.c','src/cli/platform/native/windows-storage-check.h')
  for ($i = 0; $i -lt $expectedPaths.Count; $i++) {
    $entry = $helperReceipt.source[$i]
    $provEntry = $provenance.sourceInputs[$i]
    $localSource = Join-Path $repo $expectedPaths[$i]
    if ($entry.path -ne $expectedPaths[$i] -or $provEntry.path -ne $entry.path -or
        $provEntry.sha256 -ne $entry.sha256 -or $provEntry.bytes -ne $entry.bytes -or
        (File-Sha $localSource) -ne $entry.sha256 -or (Get-Item -LiteralPath $localSource).Length -ne $entry.bytes) {
      throw 'Retained helper inputs differ from current source'
    }
  }
  $gitPath = Join-Path $env:ProgramFiles 'Git\cmd\git.exe'
  if (-not (Test-Path -LiteralPath $gitPath -PathType Leaf)) { throw 'Expected native Git path unavailable' }
  Write-Receipt (Join-Path $copyEvidence 'inputs.json') @{
    observedAt=[DateTime]::UtcNow.ToString('o'); head=$head; parent=$parent; tree=$tree;
    run=$env:GITHUB_RUN_ID; attempt=$env:GITHUB_RUN_ATTEMPT; image=$env:ImageVersion; imageOS=$env:ImageOS;
    files=$files; nodeSha256=$nodeDigest; nodeVersion='24.22.3'; gitPath=$gitPath;
    runtimeDependencies='No npm, Codex, Agents or helper ownership protocol exercised';
    helperArtifact=@{ id='14491266936'; run='44440431389'; name='final-current-msvc-x64-1'; files=$pinned; source=$helperReceipt.source }
  }
  $password = ConvertTo-SecureString ([Guid]::NewGuid().ToString('N') + '!aA1') -AsPlainText -Force
  $user = New-LocalUser -Name $name -Password $password -AccountNeverExpires
  Add-LocalGroupMember -Group (Get-LocalGroup -SID 'S-1-5-32-545') -Member $user
  & "$env:SystemRoot\System32\icacls.exe" $work /grant ('*' + $user.SID.Value + ':(OI)(CI)F') /T /Q | Out-Null
  if ($LASTEXITCODE -ne 0) { throw 'Fixture-only account grant failed' }
  $script = Join-Path $source 'tests/windows-native-read-pair/standard-user.ps1'
  foreach ($argument in @($script,$source,$nodePath,$gitPath)) {
    if ($argument.Contains('"')) { throw 'Unexpected quote in hosted path' }
  }
  $arguments = '-NoLogo -NoProfile -NonInteractive -File "' + $script + '" -Consumer -Repository "' + $source + '" -Node "' + $nodePath + '" -Git "' + $gitPath + '"'
  $credential = New-Object Management.Automation.PSCredential(($env:COMPUTERNAME + '\' + $name), $password)
  $saved = @{}
  foreach ($entry in @(Get-ChildItem Env:)) {
    if ($entry.Name -notmatch $allowed) {
      $saved[$entry.Name] = $entry.Value
      [Environment]::SetEnvironmentVariable($entry.Name, $null, 'Process')
    }
  }
  try {
    $attempted = $true
    $launchAt = [DateTime]::UtcNow.ToString('o')
    $gate = [Diagnostics.Stopwatch]::StartNew()
    $deadline = [DateTimeOffset]::UtcNow.AddMilliseconds(105000).ToString('o')
    $arguments += ' -GateDeadlineUtc "' + $deadline + '"'
    $process = Start-Process -FilePath $powerShell -Credential $credential -LoadUserProfile -ArgumentList $arguments -PassThru -RedirectStandardOutput (Join-Path $work 'consumer.stdout') -RedirectStandardError (Join-Path $work 'consumer.stderr')
  } finally {
    foreach ($entry in $saved.GetEnumerator()) { [Environment]::SetEnvironmentVariable($entry.Key, $entry.Value, 'Process') }
    $saved.Clear()
  }
  if ($process.Handle -eq [IntPtr]::Zero -or $process.Handle -eq [IntPtr](-1)) { throw 'Direct process handle unavailable' }
  $observed = $process.WaitForExit([Math]::Max(0, 110000 - [int]$gate.ElapsedMilliseconds))
  if (-not $observed) { throw 'Observation deadline; preserve unsupported state for runner disposal' }
  $status = $process.ExitCode
  if ($status -ne 0) { throw "Probe returned $status" }
} finally {
  foreach ($file in @('inputs.json','token.json','phases.ndjson')) {
    $partial = Join-Path $copyEvidence $file
    if (Test-Path -LiteralPath $partial) { Copy-Item -LiteralPath $partial -Destination $evidence }
  }
  foreach ($file in @('consumer.stdout','consumer.stderr')) {
    $partial = Join-Path $work $file
    if (Test-Path -LiteralPath $partial) { Copy-Item -LiteralPath $partial -Destination $evidence }
  }
  if ($process) { $process.Dispose() }
  if ($user -and (($observed -and $status -eq 0) -or -not $attempted)) {
    try { Remove-LocalUser -Name $name -ErrorAction Stop; $account = 'removed' }
    catch { $account = 'retained for hosted runner disposal' }
  } elseif ($user) { $account = 'retained for hosted runner disposal' }
  Write-Receipt (Join-Path $evidence 'harness.json') @{
    observedAt=[DateTime]::UtcNow.ToString('o'); launchAt=$launchAt;
    elapsedMs=$(if ($gate) { $gate.ElapsedMilliseconds } else { $null });
    attempted=$attempted; directExitObserved=$observed; exit=$status; account=$account; workspace=$work;
    deadlineMs=110000; scope='Direct process handle only. No inferred descendant release.'
  }
}
