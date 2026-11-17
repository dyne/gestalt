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
# TEST closure mode only. Keep every preceding ordinary-account prerequisite.
$env:GESTALT_CORE_SLICE_READINESS_ONLY = '0'
$env:GESTALT_CORE_SLICE_READINESS_COMPARISON = '0'
$env:GESTALT_NATIVE_RUNTIME_ARTIFACTS = [IO.Path]::GetFullPath((Join-Path $Repository 'native-evidence/core-slice-prebuilt'))
$env:GESTALT_NATIVE_AGENTS_SOURCE = [IO.Path]::GetFullPath((Join-Path $Repository '.native-dependencies/agents'))
$env:GESTALT_NATIVE_RUNTIME_EVIDENCE = Join-Path $evidence 'runtime'
New-Item -ItemType Directory -Path $env:GESTALT_NATIVE_RUNTIME_EVIDENCE -Force | Out-Null
$tar = Join-Path $env:SystemRoot 'System32\tar.exe'
if (-not (Test-Path -LiteralPath $tar -PathType Leaf)) { throw 'Native Windows tar.exe is required only for the PAX test fixture; no PATH or installation fallback' }
$tarItem = Get-Item -LiteralPath $tar
$tarVersion = (& $tar --version 2>&1 | Out-String)
$tarExit = $LASTEXITCODE
@{observedAt=[DateTime]::UtcNow.ToString('o');path=$tar;bytes=$tarItem.Length;sha256=(Get-FileHash -LiteralPath $tar -Algorithm SHA256).Hash;version=$tarVersion;exit=$tarExit;scope='Fixed native OS fixture producer, not a product prerequisite'} | ConvertTo-Json | Set-Content -LiteralPath (Join-Path $evidence 'native-tar.json')
if ($null -eq $tarExit -or $tarExit -ne 0) { throw 'Native fixture tar identity/version preflight failed' }
$bindingProgram = @'
import assert from 'node:assert/strict';import path from 'node:path';import fs from 'node:fs';import crypto from 'node:crypto';
const root=process.env.GESTALT_NATIVE_RUNTIME_ARTIFACTS,agents=process.env.GESTALT_NATIVE_AGENTS_SOURCE,out=path.join(process.env.GESTALT_NATIVE_RUNTIME_EVIDENCE,'native-binding.json');
const hash=b=>crypto.createHash('sha256').update(b).digest('hex');const read=p=>fs.readFileSync(p);const m=JSON.parse(read(path.join(root,'artifacts.json')));const checked=[];
assert.equal(process.platform,'win32');assert.equal(m.platform,process.platform);assert.equal(m.arch,process.arch);assert.equal(m.abi,process.versions.modules);assert.equal(m.helper,path.win32.join('native','gestalt-owned-process.exe'));assert.equal(m.peer,null);
for(const a of m.artifacts){assert.equal(path.isAbsolute(a.relative),false);assert.equal(a.relative.split(/[\\/]/).includes('..'),false);const b=read(path.join(root,a.relative));assert.equal(b.length,a.bytes);assert.equal(hash(b),a.sha256);checked.push(a);}
assert.equal(hash(read(path.join(root,'gestalt.mjs'))),'651db98194c2a24aa2da9f9e9ff9f8df2090c895df1a85ee4527f55c5fcae463');
const h=path.join(root,m.helper),p=JSON.parse(read(h+'.provenance.json'));assert.equal(p.artifactSha256,hash(read(h)));assert.deepEqual(p.sourceInputs,m.nativeHelperInputs);assert.deepEqual(p.sourceInputs.map(a=>a.path),['src/cli/platform/native/owned-process.c','src/cli/platform/native/windows-storage-check.c','src/cli/platform/native/windows-storage-check.h']);
for(const a of p.sourceInputs){const b=read(path.join(process.cwd(),a.path));assert.equal(b.length,a.bytes);assert.equal(hash(b),a.sha256);}assert.equal(p.sourceSha256,p.sourceInputs[0].sha256);
assert.equal(m.orgSource.head,'8827347387ee32e2c5bd7eb912f9b80185cdad45');assert.equal(m.orgSource.files.length,15);assert.equal(m.orgSource.setupApi.length,5);for(const a of [...m.orgSource.files,...m.orgSource.setupApi]){const b=read(path.join(agents,a.path));assert.equal(b.length,a.bytes);assert.equal(hash(b),a.sha256);}
fs.writeFileSync(out,JSON.stringify({observedAt:new Date().toISOString(),pass:true,platform:process.platform,arch:process.arch,node:process.version,abi:process.versions.modules,CLI:hash(read(path.join(root,'gestalt.mjs'))),helperSha256:hash(read(h)),sourceInputs:p.sourceInputs,artifacts:checked,orgFiles:15,setupApi:5,scope:'Actual ordinary-token metadata binding, not runtime behavior'},null,2));
'@
$bindingProgram | & $Node --input-type=module
if ($null -eq $LASTEXITCODE -or $LASTEXITCODE -ne 0) { throw 'Actual ordinary-token fixed manifest preflight failed' }
$closure = @{scope='Current compiled TTY and runtime closure, readiness/parity reused from run44015990174';startedAt=[DateTime]::UtcNow.ToString('o');complete=$false;stages=@();postcheckErrors=@();terminalCases=0;installedTests=0;deferredTests=0;productTimersChanged=$false}
function Save-Closure { $closure.observedAt=[DateTime]::UtcNow.ToString('o'); $closure | ConvertTo-Json -Depth 30 | Set-Content -LiteralPath (Join-Path $evidence 'closure-execution.json') }
function Invoke-ClosureStage([string]$Name,[string[]]$NodeArguments) {
  $out = Join-Path $evidence ($Name + '.tap'); $err = Join-Path $evidence ($Name + '.stderr'); $code=$null;$problem=$null;$started=[DateTime]::UtcNow.ToString('o')
  try { & $Node @NodeArguments 1> $out 2> $err; $code=$LASTEXITCODE } catch { $problem=$_.Exception.Message }
  $closure.stages += @{name=$Name;argv=@($NodeArguments);exit=$code;error=$problem;startedAt=$started;observedAt=[DateTime]::UtcNow.ToString('o');stdout=$out;stderr=$err;encoding='PowerShell redirected text; raw binary product observations are in per-fixture artifacts'}
  Save-Closure
}
Save-Closure
Invoke-ClosureStage 'terminal' @('tests/native-core-slice.mjs','--artifacts','native-evidence/core-slice-prebuilt','--agents','.native-dependencies/agents','--evidence',$evidence)
Invoke-ClosureStage 'runtime' @('--test','--test-reporter=tap','tests/native-archive.test.mjs','tests/native-diagnostics.test.mjs','tests/native-app-server.test.mjs','tests/native-capabilities.test.mjs','tests/native-skills-owner.test.mjs','tests/native-skills-transfer.test.mjs','tests/native-scratch.test.mjs','tests/native-hook-command.test.mjs','tests/native-storage-read.test.mjs','tests/native-runtime-installed.test.mjs')
Invoke-ClosureStage 'deferred' @('--test','--test-reporter=tap','--test-name-pattern=deferred native tool requests','tests/native-cli.test.mjs')
function Read-Json([string]$Relative) { Get-Content -LiteralPath (Join-Path $evidence $Relative) -Raw | ConvertFrom-Json }
function Require([bool]$Value,[string]$Message) { if (-not $Value) { throw $Message } }
function Check-Tap([string]$Name,[string[]]$RequiredNames,[int]$ExpectedSkips) {
  $text=Get-Content -LiteralPath (Join-Path $evidence ($Name+'.tap')) -Raw
  $counts=@{};foreach($key in @('tests','pass','fail','skipped')){ $m=[regex]::Match($text,'(?m)^# '+$key+' (\d+)\s*$');Require $m.Success ($Name+' TAP missing '+$key);$counts[$key]=[int]$m.Groups[1].Value }
  Require ($counts.tests -gt 0 -and $counts.fail -eq 0 -and $counts.skipped -eq $ExpectedSkips -and ($counts.pass+$counts.skipped) -eq $counts.tests) ($Name+' TAP counts invalid')
  $ids=@([regex]::Matches($text,'(?m)^# Subtest: (.+)\r?$') | ForEach-Object { $_.Groups[1].Value.TrimEnd("`r") });Require ($ids.Count -eq $counts.tests) ($Name+' test identity count differs')
  foreach($id in $RequiredNames){Require (@($ids|Where-Object {$_ -ceq $id}).Count -eq 1) ('Missing/duplicate required test '+$id);Require ([regex]::IsMatch($text,'(?m)^ok \d+ - '+[regex]::Escape($id)+'\r?$')) ('Required test failed/skipped '+$id)}
  return @{counts=$counts;identities=$ids;requiredIdentities=@($RequiredNames);scope='Actual TAP, not expected counts or zero-match success'}
}
try {
  Require ($closure.stages.Count -eq 3) 'Three independent stages required'
  foreach($s in $closure.stages){Require ($null -ne $s.exit -and $s.exit -eq 0 -and -not $s.error) ('Stage failed/unknown '+$s.name)}
  $mode=Read-Json 'mode-entry.json';Require ($mode.platform -eq 'win32' -and $mode.mode -eq 'terminal' -and $mode.flag -eq '0' -and $mode.comparisonFlag -eq '0' -and $mode.comparison -eq $false) 'Actual ordinary terminal mode0/0 required'
  $provider=Read-Json 'provider-requests.json';Require (-not $provider.policyError -and $provider.cases.Count -eq 2) 'Terminal provider policy/cases invalid'
  $retained=Read-Json 'retained-fixture.json';$prebinding=Read-Json 'runtime/native-binding.json';Require ($retained.consumer.sha256 -ceq $prebinding.CLI -and $prebinding.CLI -ceq '651db98194c2a24aa2da9f9e9ff9f8df2090c895df1a85ee4527f55c5fcae463') 'Actual terminal copied consumer binding differs'
  foreach($name in @('eof','interrupt')) {
    $r=Read-Json ($name+'/result.json');$f=Read-Json ($name+'/terminal-final.json');$a=Read-Json ($name+'/authority.json');$cfg=Read-Json ($name+'/configuration.json');$t=$r.terminal
    Require ($r.pass -eq $true -and $r.actualInstalledOrgAndWholeIpc -eq $true -and $r.zeroGenerationOrOtherRequests -eq $true) ('Actual TTY authority/IPC gate failed '+$name)
    Require ($t.result.kind -eq 'exit' -and $t.result.code -eq 0 -and $t.claim.kind -eq 'released' -and $t.claim.backend -eq 'windows-job' -and $t.claim.result.kind -eq 'exit' -and $t.claim.result.code -eq 0 -and $t.claim.lease -match '^[a-f0-9-]{36}$') ('Natural0 reportedJob required '+$name)
    Require (-not $f.failure -and $f.claim.kind -eq 'released' -and $f.claim.backend -eq 'windows-job' -and $f.claim.lease -ceq $t.claim.lease -and $f.claim.result.code -eq 0 -and $t.controlSent -eq $true -and $t.submittedBlockedFixtureTurn -eq $true) ('Final matchingJob/control required '+$name)
    Require ($a.pass -eq $true -and $a.installedBindings.Count -eq 15 -and $a.publication.published -eq $true -and $a.publishedFile.planMatches -eq $true -and $a.publishedFile.schemaVersion -eq 1 -and $a.identityMismatch.refused -eq $true -and $a.ipc.ok -eq $true -and $a.ipc.requestHex -ceq $a.ipc.replyHex) ('Canonical publication/wholeIPC/refusal required '+$name)
    foreach($key in @('sid','pid','started','executable')){Require ($a.ipc.host.$key -ceq $cfg.host.$key) ('Expected authenticated host differs '+$key)}
    Require ($a.identity.observerSid -ceq $cfg.host.sid) 'Ordinary principal differs'
    foreach($key in @('GESTALT_SESSION_TMP_CONTEXT','TMPDIR','TMP','TEMP')){Require ($a.inherited.$key -ceq $cfg.inherited.$key) ('Inherited authority/TMP differs '+$key)}
    Require ($t.controlEvents.Count -eq 1 -and $t.controlEvents[0].activeCancellationClaimed -eq $false) 'Exactly one honest control event required'
    if($name -eq 'interrupt'){Require ($t.controlEvents[0].action -eq 'ctrl-c-ui-quit' -and $t.controlEvents[0].intent -eq 'settled-ui-quit' -and $t.controlEvents[0].settledStatusOffset -gt $t.controlEvents[0].workingStatusOffset) 'Settled UI quit, not active cancellation'}else{Require ($t.controlEvents[0].action -eq 'eof') 'EOF control required'}
    $closure.terminalCases++
  }
  $required=@([regex]::Matches((Get-Content -LiteralPath 'tests/native-runtime-installed.test.mjs' -Raw),"(?m)^test\('([^']+)'") | ForEach-Object {$_.Groups[1].Value});Require ($required.Count -eq 11) 'Exact eleven installed contract identities required'
  $closure.runtimeTap=Check-Tap 'runtime' $required 2
  $runtimeText=Get-Content -LiteralPath (Join-Path $evidence 'runtime.tap') -Raw
  $skips=@([regex]::Matches($runtimeText,'(?m)^ok \d+ - (.+?) # SKIP[^\r\n]*')|ForEach-Object{$_.Groups[1].Value})
  $allowedSkips=@('archive round trip preserves an executable skill resource with only private owner permissions','validated runtime-query environment preserves the real inherited v1 context and all TMP values without metadata writes')
  foreach($id in $skips){Require ($id -cin $allowedSkips) ('Unapproved skip '+$id)}
  $closure.installedTests=11
  $id='deferred native tool requests refuse before touching absent or existing profiles';$closure.deferredTap=Check-Tap 'deferred' @($id) 0;Require ($closure.deferredTap.counts.tests -eq 1 -and $closure.deferredTap.counts.pass -eq 1) 'Exactly one deferred test required';$closure.deferredTests=1
  $binding=Read-Json 'runtime/native-binding.json';$installed=Read-Json 'runtime/runtime-build.json';Require ($binding.pass -eq $true -and $binding.CLI -ceq $installed.managerSha256 -and $binding.helperSha256 -ceq $installed.helperSha256 -and $installed.sourceBuilderSeparateFromConsumer -eq $true -and $installed.consumerCompilerRequired -eq $false) 'Installed current artifact binding differs'
  $closure.binding=$binding;$closure.complete=$true
} catch { $closure.postcheckErrors += $_.Exception.Message }
Save-Closure
if (-not $closure.complete -or $closure.postcheckErrors.Count) { throw ('Closure gate failed/unknown: '+($closure.postcheckErrors -join '; ')) }
exit 0
