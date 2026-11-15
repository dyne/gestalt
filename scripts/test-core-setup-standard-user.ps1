# Hosted CI only. Run product/tests with an ordinary Windows token and no Bash PATH.
param([Parameter(Mandatory=$true)][string]$Repository, [Parameter(Mandatory=$true)][string]$Node, [Parameter(Mandatory=$true)][string]$GitDirectory)
$ErrorActionPreference = 'Stop'
$localData = [Environment]::GetFolderPath([Environment+SpecialFolder]::LocalApplicationData)
$scratch = Join-Path $localData ('Temp\gestalt-core-' + [Guid]::NewGuid().ToString('N'))
New-Item -ItemType Directory -Path $scratch -Force | Out-Null
$env:TMP = $scratch
$env:TEMP = $scratch
$env:TMPDIR = $scratch
$tools = Join-Path $localData 'gestalt-core-ci-tools'
$nodeDirectory = Split-Path -Parent $Node
# Git for Windows exposes git.exe through both bin (also contains Bash) and
# cmd (native command launcher only). Select cmd from the resolved installation;
# never forward an arbitrary first Git PATH entry into product execution.
$suppliedGitDirectory = $GitDirectory
$gitCursor = [IO.Path]::GetFullPath($GitDirectory)
$nativeGit = $null
for ($attempt = 0; $attempt -lt 4; $attempt++) {
  $candidate = Join-Path $gitCursor 'cmd\git.exe'
  if (Test-Path -LiteralPath $candidate -PathType Leaf) { $nativeGit = [IO.Path]::GetFullPath($candidate); break }
  $next = Split-Path -Parent $gitCursor
  if (-not $next -or $next -eq $gitCursor) { break }
  $gitCursor = $next
}
if (-not $nativeGit) { throw 'The selected native Git installation must provide cmd\git.exe without Bash on PATH' }
$GitDirectory = Split-Path -Parent $nativeGit
foreach ($shell in @('bash.exe', 'sh.exe')) {
  if (Test-Path -LiteralPath (Join-Path $GitDirectory $shell)) { throw 'Native Git command directory unexpectedly contains a shell runtime' }
}
$env:PATH = $nodeDirectory + ';' + $tools + ';' + $GitDirectory + ';' + $env:SystemRoot + '\System32;' + $env:SystemRoot + '\System32\WindowsPowerShell\v1.0;' + $env:SystemRoot
if ($env:PATH -match '\\Git\\(?:usr\\bin|bin)|msys|cygwin') { throw 'Bash runtime must not be available through PATH' }
$principal = New-Object Security.Principal.WindowsPrincipal([Security.Principal.WindowsIdentity]::GetCurrent())
if ($principal.IsInRole([Security.Principal.WindowsBuiltInRole]::Administrator)) { throw 'Core setup requires a standard-user test token' }
$resolvedGit = (Get-Command git.exe).Source
if ([IO.Path]::GetFullPath($resolvedGit) -ne $nativeGit) { throw 'Native Git PATH resolution differs from the selected cmd boundary' }
$gitVersion = & $nativeGit --version
if ($LASTEXITCODE -ne 0) { throw 'Selected native Git command did not execute' }
$harnessTools = @{ suppliedGitDirectory=$suppliedGitDirectory; gitExecutable=$nativeGit; gitVersion=$gitVersion; nodeExecutable=$Node; standardUser=$true; noBashPath=$true; pathEntries=@($env:PATH -split ';'); observedAt=[DateTime]::UtcNow.ToString('o') }
[IO.File]::WriteAllText((Join-Path $Repository 'native-evidence/setup/harness-tools.json'), ($harnessTools | ConvertTo-Json -Depth 4), (New-Object Text.UTF8Encoding($false)))
$npmCli = Join-Path $nodeDirectory 'node_modules\npm\bin\npm-cli.js'
& $Node $npmCli install --global --prefix $tools --ignore-scripts --no-audit --no-fund '@openai/codex@0.164.0'
if ($LASTEXITCODE -ne 0) { throw 'Standard-user Codex dependency install failed' }
Set-Location -LiteralPath $Repository
& $Node scripts/record-native-setup.mjs --verify-copy
if ($LASTEXITCODE -ne 0) { throw 'Copied source verification failed' }
$env:GESTALT_NATIVE_AGENTS_SOURCE = Join-Path $Repository '.native-dependencies/agents'
$env:GESTALT_NATIVE_SETUP_EVIDENCE = Join-Path $Repository 'native-evidence/setup'
$identity = @{ platform='win32'; arch=(& $Node -p process.arch); node=(& $Node --version); abi=(& $Node -p process.versions.modules); standardUser=$true; noBashPath=$true; observedAt=[DateTime]::UtcNow.ToString('o') }
[IO.File]::WriteAllText((Join-Path $env:GESTALT_NATIVE_SETUP_EVIDENCE 'runner.json'), ($identity | ConvertTo-Json), (New-Object Text.UTF8Encoding($false)))
& $Node --test --test-reporter=tap tests/native-cli.test.mjs tests/native-setup.test.mjs tests/native-setup-transactions.test.mjs tests/native-setup-installed.test.mjs
exit $LASTEXITCODE
