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
$env:GESTALT_NATIVE_RUNTIME_ARTIFACTS = [IO.Path]::GetFullPath((Join-Path $Repository 'native-evidence/core-slice-prebuilt'))
$env:GESTALT_NATIVE_AGENTS_SOURCE = [IO.Path]::GetFullPath((Join-Path $Repository '.native-dependencies/agents'))
$env:GESTALT_NATIVE_RUNTIME_EVIDENCE = Join-Path $evidence 'runtime'
New-Item -ItemType Directory -Path $env:GESTALT_NATIVE_RUNTIME_EVIDENCE -Force | Out-Null
$env:GESTALT_NATIVE_SETUP_ARTIFACTS = $env:GESTALT_NATIVE_RUNTIME_ARTIFACTS
$env:GESTALT_NATIVE_SETUP_EVIDENCE = $env:GESTALT_NATIVE_RUNTIME_EVIDENCE
$bindingProgram = @'
import assert from 'node:assert/strict';import path from 'node:path';import fs from 'node:fs';import crypto from 'node:crypto';
const root=process.env.GESTALT_NATIVE_RUNTIME_ARTIFACTS,agents=process.env.GESTALT_NATIVE_AGENTS_SOURCE,out=path.join(process.env.GESTALT_NATIVE_RUNTIME_EVIDENCE,'native-binding.json');
const hash=b=>crypto.createHash('sha256').update(b).digest('hex');const read=p=>fs.readFileSync(p);const m=JSON.parse(read(path.join(root,'artifacts.json')));const checked=[];
assert.equal(process.platform,'win32');assert.equal(m.platform,process.platform);assert.equal(m.arch,process.arch);assert.equal(m.abi,process.versions.modules);assert.equal(m.helper,path.win32.join('native','gestalt-owned-process.exe'));assert.equal(m.peer,null);
for(const a of m.artifacts){assert.equal(path.isAbsolute(a.relative),false);assert.equal(a.relative.split(/[\\/]/).includes('..'),false);const b=read(path.join(root,a.relative));assert.equal(b.length,a.bytes);assert.equal(hash(b),a.sha256);checked.push(a);}
assert.equal(hash(read(path.join(root,'gestalt.mjs'))),'216b6ea052ceb2fa4223f8fc51b9129af020adcfca32fbfd70011e5f9a59bf60');assert.equal(read(path.join(root,'gestalt.mjs')).length,336071);
const h=path.join(root,m.helper),p=JSON.parse(read(h+'.provenance.json'));assert.equal(p.artifactSha256,hash(read(h)));assert.deepEqual(p.sourceInputs,m.nativeHelperInputs);assert.deepEqual(p.sourceInputs.map(a=>a.path),['src/cli/platform/native/owned-process.c','src/cli/platform/native/windows-storage-check.c','src/cli/platform/native/windows-storage-check.h']);
for(const a of p.sourceInputs){const b=read(path.join(process.cwd(),a.path));assert.equal(b.length,a.bytes);assert.equal(hash(b),a.sha256);}assert.equal(p.sourceSha256,p.sourceInputs[0].sha256);
assert.equal(m.orgSource.head,'8827347387ee32e2c5bd7eb912f9b80185cdad45');assert.equal(m.orgSource.files.length,15);assert.equal(m.orgSource.setupApi.length,5);for(const a of [...m.orgSource.files,...m.orgSource.setupApi]){const b=read(path.join(agents,a.path));assert.equal(b.length,a.bytes);assert.equal(hash(b),a.sha256);}
fs.writeFileSync(path.join(process.env.GESTALT_NATIVE_RUNTIME_EVIDENCE,'setup-selection-mode.json'),JSON.stringify({observedAt:new Date().toISOString(),platform:process.platform,mode:'setup-selection',policyTests:24,compiledTests:1,TTY:0,setupArtifacts:process.env.GESTALT_NATIVE_SETUP_ARTIFACTS,setupEvidence:process.env.GESTALT_NATIVE_SETUP_EVIDENCE,runtimeArtifacts:root,agentsSource:agents,scope:'Explicit ordinary-account selection; no runtime/TUI/query/model turn'},null,2));
fs.writeFileSync(out,JSON.stringify({observedAt:new Date().toISOString(),pass:true,platform:process.platform,arch:process.arch,node:process.version,abi:process.versions.modules,CLI:hash(read(path.join(root,'gestalt.mjs'))),helperSha256:hash(read(h)),sourceInputs:p.sourceInputs,artifacts:checked,orgFiles:15,setupApi:5,scope:'Actual ordinary-token metadata binding, not runtime behavior'},null,2));
'@
$bindingProgram | & $Node --input-type=module
if ($null -eq $LASTEXITCODE -or $LASTEXITCODE -ne 0) { throw 'Actual ordinary-token fixed manifest preflight failed' }
$closure = @{scope='Affected Windows setup policy only: policy24+compiled1, 25unique; TTY0/no runtime or query replay';startedAt=[DateTime]::UtcNow.ToString('o');complete=$false;stages=@();postcheckErrors=@();terminalCases=0;selectedTests=$null;installedTests=$null;commonTests=$null;policyTests=$null;compiledTests=$null;deferredTests=0;planned=@{terminal=0;selected=25;installed=1;common=24;policy=24;compiled=1};productTimersChanged=$false}
function Save-Closure { $closure.observedAt=[DateTime]::UtcNow.ToString('o'); $closure | ConvertTo-Json -Depth 40 | Set-Content -LiteralPath (Join-Path $evidence 'closure-execution.json') }
function Invoke-ClosureStage([string]$Name,[string[]]$NodeArguments) {
  $out = Join-Path $evidence ($Name + '.tap'); $err = Join-Path $evidence ($Name + '.stderr'); $code=$null;$problem=$null;$started=[DateTime]::UtcNow.ToString('o')
  $priorErrorPreference=$ErrorActionPreference
  try { $ErrorActionPreference='Continue'; $global:LASTEXITCODE=$null; & $Node @NodeArguments 1> $out 2> $err; $code=$global:LASTEXITCODE } catch { $problem=$_.Exception.Message }
  finally { $ErrorActionPreference=$priorErrorPreference }
  $closure.stages += @{name=$Name;argv=@($NodeArguments);exit=$code;error=$problem;startedAt=$started;observedAt=[DateTime]::UtcNow.ToString('o');stdout=$out;stderr=$err;encoding='PowerShell redirected text; raw binary product observations are in per-fixture artifacts'}
  Save-Closure
}
Save-Closure
$policyRequired=@('setup and component parsing reject unsupported policy before any operation','Unix update selection policy preserves all boundaries and ordering','Unix update selection policy preserves gestalt boundaries and ordering','Unix update selection policy preserves serena boundaries and ordering','Unix update selection policy preserves playwright boundaries and ordering','native setup policy Windows all omits deferred operations with retained components=false','native setup policy Windows all omits deferred operations with retained components=true','native setup policy Windows explicit install serena refuses before context and profile work','native setup policy Windows explicit install playwright refuses before context and profile work','native setup policy Windows explicit install mobile refuses before context and profile work','native setup policy Windows explicit update serena refuses before context and profile work','native setup policy Windows explicit update playwright refuses before context and profile work','native setup policy Windows explicit update mobile refuses before context and profile work','native setup policy Windows explicit update-restart serena refuses before context and profile work','native setup policy Windows explicit update-restart playwright refuses before context and profile work','native setup policy Windows explicit update-restart mobile refuses before context and profile work','native setup policy Windows install keeps supported operations and extra selection=false','native setup policy Windows install keeps supported operations and extra selection=true','native setup policy rejects a direct Windows named update before any core or optional operation','native setup policy uses actual host support by default and keeps ordinary Unix Mobile failures visible','native setup policy Windows core-only update preserves extra skills and npm manager ownership','native setup policy Windows does not hide required core failure or continue after manager handoff','native setup policy Windows named component flag forms preserve existing profile bytes before refusal','native setup policy Windows explicit all flag retains the same supported-only update')
$compiledRequired=@('compiled install/update preserves native skills, extra selection and optional Mobile policy')
$policyPattern='^(?:setup\ and\ component\ parsing\ reject\ unsupported\ policy\ before\ any\ operation|Unix\ update\ selection\ policy\ preserves\ all\ boundaries\ and\ ordering|Unix\ update\ selection\ policy\ preserves\ gestalt\ boundaries\ and\ ordering|Unix\ update\ selection\ policy\ preserves\ serena\ boundaries\ and\ ordering|Unix\ update\ selection\ policy\ preserves\ playwright\ boundaries\ and\ ordering|native\ setup\ policy\ Windows\ all\ omits\ deferred\ operations\ with\ retained\ components=false|native\ setup\ policy\ Windows\ all\ omits\ deferred\ operations\ with\ retained\ components=true|native\ setup\ policy\ Windows\ explicit\ install\ serena\ refuses\ before\ context\ and\ profile\ work|native\ setup\ policy\ Windows\ explicit\ install\ playwright\ refuses\ before\ context\ and\ profile\ work|native\ setup\ policy\ Windows\ explicit\ install\ mobile\ refuses\ before\ context\ and\ profile\ work|native\ setup\ policy\ Windows\ explicit\ update\ serena\ refuses\ before\ context\ and\ profile\ work|native\ setup\ policy\ Windows\ explicit\ update\ playwright\ refuses\ before\ context\ and\ profile\ work|native\ setup\ policy\ Windows\ explicit\ update\ mobile\ refuses\ before\ context\ and\ profile\ work|native\ setup\ policy\ Windows\ explicit\ update\-restart\ serena\ refuses\ before\ context\ and\ profile\ work|native\ setup\ policy\ Windows\ explicit\ update\-restart\ playwright\ refuses\ before\ context\ and\ profile\ work|native\ setup\ policy\ Windows\ explicit\ update\-restart\ mobile\ refuses\ before\ context\ and\ profile\ work|native\ setup\ policy\ Windows\ install\ keeps\ supported\ operations\ and\ extra\ selection=false|native\ setup\ policy\ Windows\ install\ keeps\ supported\ operations\ and\ extra\ selection=true|native\ setup\ policy\ rejects\ a\ direct\ Windows\ named\ update\ before\ any\ core\ or\ optional\ operation|native\ setup\ policy\ uses\ actual\ host\ support\ by\ default\ and\ keeps\ ordinary\ Unix\ Mobile\ failures\ visible|native\ setup\ policy\ Windows\ core\-only\ update\ preserves\ extra\ skills\ and\ npm\ manager\ ownership|native\ setup\ policy\ Windows\ does\ not\ hide\ required\ core\ failure\ or\ continue\ after\ manager\ handoff|native\ setup\ policy\ Windows\ named\ component\ flag\ forms\ preserve\ existing\ profile\ bytes\ before\ refusal|native\ setup\ policy\ Windows\ explicit\ all\ flag\ retains\ the\ same\ supported\-only\ update)$'
$compiledPattern='^(?:compiled\ install/update\ preserves\ native\ skills,\ extra\ selection\ and\ optional\ Mobile\ policy)$'
Invoke-ClosureStage 'policy' @('--test','--test-reporter=tap',('--test-name-pattern='+$policyPattern),'tests/native-setup.test.mjs')
Invoke-ClosureStage 'compiled' @('--test','--test-reporter=tap',('--test-name-pattern='+$compiledPattern),'tests/native-setup-transactions.test.mjs')
function Read-Json([string]$Relative) { Get-Content -LiteralPath (Join-Path $evidence $Relative) -Raw | ConvertFrom-Json }
function Require([bool]$Value,[string]$Message) { if (-not $Value) { throw $Message } }
function Capture-Tap([string]$Name,[string[]]$RequiredNames) {
  $record=@{name=$Name;expected=@($RequiredNames);observedAt=[DateTime]::UtcNow.ToString('o');counts=@{};identities=@();outcomes=@();rawFailures=@();captureError=$null;pass=$false}
  try {
    $file=Join-Path $evidence ($Name+'.tap');$text=Get-Content -LiteralPath $file -Raw
    $record.sha256=(Get-FileHash -LiteralPath $file -Algorithm SHA256).Hash;$record.bytes=(Get-Item -LiteralPath $file).Length
    foreach($key in @('tests','pass','fail','skipped','cancelled')){$m=[regex]::Match($text,'(?m)^# '+$key+' (\d+)\s*$');Require $m.Success ($Name+' TAP missing '+$key);$record.counts[$key]=[int]$m.Groups[1].Value}
    $record.identities=@([regex]::Matches($text,'(?m)^# Subtest: (.+)\r?$')|ForEach-Object{$_.Groups[1].Value.TrimEnd("`r")})
    $record.outcomes=@([regex]::Matches($text,'(?m)^(not ok|ok) (\d+) - ([^\r\n]+)')|ForEach-Object{@{status=$_.Groups[1].Value;number=[int]$_.Groups[2].Value;name=($_.Groups[3].Value -split ' # SKIP',2)[0];skipped=($_.Groups[3].Value -match ' # SKIP')}})
    $record.rawFailures=@([regex]::Matches($text,'(?ms)^not ok [^\r\n]*\r?\n.*?(?=^# Subtest:|^1\.\.|\z)')|ForEach-Object{$_.Value})
    Require ($record.counts.tests -eq $RequiredNames.Count -and $record.counts.pass -eq $RequiredNames.Count -and $record.counts.fail -eq 0 -and $record.counts.skipped -eq 0 -and $record.counts.cancelled -eq 0 -and $record.identities.Count -eq $RequiredNames.Count -and $record.outcomes.Count -eq $RequiredNames.Count) ($Name+' TAP exact counts failed')
    foreach($id in $RequiredNames){Require (@($record.identities|Where-Object{$_ -ceq $id}).Count -eq 1) ('Missing/duplicate required test '+$id);Require ([regex]::IsMatch($text,'(?m)^ok \d+ - '+[regex]::Escape($id)+'\r?$')) ('Required test failed/skipped '+$id)}
    $record.pass=$true
  } catch { $record.captureError=$_.Exception.Message }
  return $record
}
# Both actual TAP captures/full failures persist independently before validation.
$policy=Capture-Tap 'policy' $policyRequired;$compiled=Capture-Tap 'compiled' $compiledRequired
@{observedAt=[DateTime]::UtcNow.ToString('o');policy=$policy;compiled=$compiled;scope='25unique required, source policy24/compiled1, no TTY/fullsuite claim'} | ConvertTo-Json -Depth 30 | Set-Content -LiteralPath (Join-Path $evidence 'affected-setup-results.json')
$closure.policyTap=$policy;$closure.compiledTap=$compiled
$closure.policyTests=@($policy.outcomes|Where-Object{-not $_.skipped}).Count;$closure.compiledTests=@($compiled.outcomes|Where-Object{-not $_.skipped}).Count
$closure.selectedTests=$closure.policyTests+$closure.compiledTests;$closure.installedTests=$closure.compiledTests;$closure.commonTests=$closure.policyTests
Save-Closure
try {
  Require ($closure.stages.Count -eq 2) 'Two independent setup stages required'
  foreach($stage in $closure.stages){Require ($null -ne $stage.exit -and $stage.exit -eq 0 -and -not $stage.error) ('Stage failed/unknown '+$stage.name)}
  Require ($policy.pass -eq $true -and $compiled.pass -eq $true -and $closure.selectedTests -eq 25 -and $closure.policyTests -eq 24 -and $closure.compiledTests -eq 1 -and $closure.terminalCases -eq 0) 'Exactly25 required actual tests must pass without skips'
  $mode=Read-Json 'runtime/setup-selection-mode.json';Require ($mode.mode -ceq 'setup-selection' -and $mode.platform -ceq 'win32' -and $mode.policyTests -eq 24 -and $mode.compiledTests -eq 1 -and $mode.TTY -eq 0 -and $mode.setupArtifacts -ceq $env:GESTALT_NATIVE_SETUP_ARTIFACTS -and $mode.setupEvidence -ceq $env:GESTALT_NATIVE_SETUP_EVIDENCE -and $mode.runtimeArtifacts -ceq $env:GESTALT_NATIVE_RUNTIME_ARTIFACTS) 'Actual explicit ordinary setup selection required'
  $binding=Read-Json 'runtime/native-binding.json';$prebuilt=Read-Json 'runtime/prebuilt-setup-binding.json'
  Require ($binding.pass -eq $true -and $binding.CLI -ceq '216b6ea052ceb2fa4223f8fc51b9129af020adcfca32fbfd70011e5f9a59bf60' -and $prebuilt.CLI -ceq $binding.CLI -and $prebuilt.CLIBytes -eq 336071 -and $prebuilt.helperSha256 -ceq $binding.helperSha256 -and $prebuilt.platform -ceq 'win32' -and $prebuilt.sourceBuilderSeparateFromConsumer -eq $true -and $prebuilt.consumerCompilerRequired -eq $false -and $prebuilt.prebuiltRootNeverCleaned -eq $true) 'Strict current prebuilt setup consumption required'
  foreach($artifact in $binding.artifacts){$file=Join-Path $env:GESTALT_NATIVE_RUNTIME_ARTIFACTS $artifact.relative;Require ((Test-Path -LiteralPath $file -PathType Leaf) -and (Get-Item -LiteralPath $file).Length -eq $artifact.bytes -and (Get-FileHash -LiteralPath $file -Algorithm SHA256).Hash -ieq $artifact.sha256) 'Prebuilt source artifacts changed or missing'}
  $selection=Read-Json 'runtime/compiled-setup-selection.json';Require ($selection.platform -ceq 'win32' -and $selection.complete -eq $true -and $selection.observations.Count -eq 22) 'Complete native compiled selection observations required'
  foreach($phase in @('absent-profiles','existing-profiles')){
    $observed=@($selection.observations|Where-Object{$_.phase -ceq $phase});Require ($observed.Count -eq 9) ('All deferred refusals required '+$phase)
    foreach($command in @('install','update','update-restart')){foreach($component in @('serena','playwright','mobile')){
      $one=@($observed|Where-Object{$_.argv.Count -eq 2 -and $_.argv[0] -ceq $command -and $_.argv[1] -ceq $component});Require ($one.Count -eq 1 -and $one[0].result.status -eq 1 -and $one[0].result.stderr -match ('native '+$component+' support is deferred') -and $one[0].result.stderr -notmatch 'SESSION_SCRATCH_INVALID_CONTEXT') 'Exact pre-context refusal missing/failed'
    }}
  }
  foreach($phase in @('install','update-all','update-gestalt')){$one=@($selection.observations|Where-Object{$_.phase -ceq $phase});Require ($one.Count -eq 1 -and $one[0].result.status -eq 0) ('Actual core phase failed '+$phase)}
  Require (@($selection.observations|Where-Object{$_.phase -ceq 'complete' -and $_.result.status -eq 0}).Count -eq 1) 'Native source assertions did not finish'
  $closure.binding=$binding;$closure.prebuilt=$prebuilt;$closure.compiledSelection=$selection;$closure.complete=$true
} catch { $closure.postcheckErrors += $_.Exception.Message }
Save-Closure
if (-not $closure.complete -or $closure.postcheckErrors.Count) { throw ('Affected setup gate failed/unknown: '+($closure.postcheckErrors -join '; ')) }
exit 0
