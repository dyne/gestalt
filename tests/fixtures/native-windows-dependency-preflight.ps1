# Disposable CI account only. No registry, service or running-host changes.
param([Parameter(Mandatory=$true)][string]$Repository, [Parameter(Mandatory=$true)][string]$Node)
$ErrorActionPreference = 'Stop'
$principal = New-Object Security.Principal.WindowsPrincipal([Security.Principal.WindowsIdentity]::GetCurrent())
if ($principal.IsInRole([Security.Principal.WindowsBuiltInRole]::Administrator)) { throw 'Preflight requires an ordinary standard-user token' }
$local = [Environment]::GetFolderPath([Environment+SpecialFolder]::LocalApplicationData)
if (-not $local) { throw 'Private standard-user local data unavailable' }
$scratch = Join-Path $local ('Temp\gestalt-dependency-' + [Guid]::NewGuid().ToString('N'))
New-Item -ItemType Directory -Path $scratch | Out-Null
$env:TMP = $scratch
$env:TEMP = $scratch
Set-Location -LiteralPath $Repository
$env:GESTALT_NATIVE_TEST_EVIDENCE = Join-Path $Repository 'native-evidence'
$env:GESTALT_NATIVE_BUILD_DIAGNOSTICS = '1'
New-Item -ItemType Directory -Path $env:GESTALT_NATIVE_TEST_EVIDENCE -Force | Out-Null
@{observedAt=[DateTime]::UtcNow.ToString('o'); administrator=$false; registryWrites=$false; node=$Node; repository=$Repository; scratch=$scratch} | ConvertTo-Json > (Join-Path $env:GESTALT_NATIVE_TEST_EVIDENCE 'dependency-standard-user.json')
& $Node --test --test-reporter=tap tests/native-process-input.test.mjs 1> (Join-Path $env:GESTALT_NATIVE_TEST_EVIDENCE 'dependency-input.tap')
$inputResult = $LASTEXITCODE
if ($null -eq $inputResult -or $inputResult -ne 0) { throw ('Borrowed input contracts failed: ' + $inputResult) }
& $Node tests/native-windows-dependency-preflight.mjs
$result = $LASTEXITCODE
if ($null -eq $result) { throw 'Native dependency preflight exit code unavailable' }
exit $result
