# Approved disposable GitHub-hosted core-slice preparation only.
$ErrorActionPreference = 'Stop'
if ($env:GITHUB_ACTIONS -ne 'true' -or $env:RUNNER_OS -ne 'Windows' -or $env:GESTALT_TEST_RUNNER_ENVIRONMENT -ne 'github-hosted') {
  throw 'Developer Mode preparation is limited to this disposable GitHub-hosted Windows job'
}
$principal = New-Object Security.Principal.WindowsPrincipal([Security.Principal.WindowsIdentity]::GetCurrent())
if (-not $principal.IsInRole([Security.Principal.WindowsBuiltInRole]::Administrator)) {
  throw 'CI builder preparation requires the existing hosted-runner administrator token; the product consumer remains a separate ordinary account'
}
$key = 'HKLM:\SOFTWARE\Microsoft\Windows\CurrentVersion\AppModelUnlock'
$name = 'AllowDevelopmentWithoutDevLicense'
$item = Get-Item -LiteralPath $key
$beforeKind = $item.GetValueKind($name).ToString()
$before = Get-ItemPropertyValue -LiteralPath $key -Name $name
if ($beforeKind -ne 'DWord' -or $before -notin @(0, 1)) { throw 'Unexpected Developer Mode registry state; do not repair or infer OFF' }
if ($before -eq 1) { Set-ItemProperty -LiteralPath $key -Name $name -Value ([int]0) }
$after = Get-ItemPropertyValue -LiteralPath $key -Name $name
$afterKind = (Get-Item -LiteralPath $key).GetValueKind($name).ToString()
$record = @{observedAt=[DateTime]::UtcNow.ToString('o'); runnerEnvironment=$env:GESTALT_TEST_RUNNER_ENVIRONMENT; key=$key; name=$name; before=$before; beforeKind=$beforeKind; after=$after; afterKind=$afterKind; scope='Disposable hosted CI preparation only; product executes later as a standard user'}
$record | ConvertTo-Json | Set-Content -LiteralPath native-evidence/developer-mode-off.json
if ($after -ne 0 -or $afterKind -ne 'DWord') { throw 'Developer Mode OFF readback failed' }
# Keep DWORD0 until this disposable runner is discarded. No re-enabling in cleanup.
