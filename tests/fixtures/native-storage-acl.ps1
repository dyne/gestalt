# Isolated ordinary-user fixture only. DACL-only changes; no owner/group/SACL write.
$ErrorActionPreference = 'Stop'
$request = [Text.Encoding]::UTF8.GetString([Convert]::FromBase64String($env:GESTALT_STORAGE_PARITY_REQUEST)) | ConvertFrom-Json
$sid = [Security.Principal.WindowsIdentity]::GetCurrent().User
$principal = New-Object Security.Principal.WindowsPrincipal([Security.Principal.WindowsIdentity]::GetCurrent())
if ($principal.IsInRole([Security.Principal.WindowsBuiltInRole]::Administrator)) { throw 'ACL parity requires the ordinary consumer token' }
$fixtureKinds = @('private', 'others-read', 'others-write', 'inherit-only', 'deny-read', 'null', 'empty')
function New-FixtureDescriptor([string]$Kind, [string]$UserSid) {
  if ($Kind -eq 'null') {
    return (New-Object Security.AccessControl.RawSecurityDescriptor([Security.AccessControl.ControlFlags]::DiscretionaryAclPresent, $null, $null, $null, $null))
  } else {
    $extra = switch ($Kind) {
      'private' { '' }
      'others-read' { '(A;;FR;;;WD)' }
      'others-write' { '(A;;WD;;;WD)' }
      'inherit-only' { '(A;OICIIO;WD;;;WD)' }
      'deny-read' { '(D;;0x00000001;;;WD)' }
      'empty' { '' }
      default { throw 'Unknown fixed ACL fixture' }
    }
    $sddl = if ($Kind -eq 'empty') { 'D:P' } elseif ($Kind -eq 'deny-read') { 'D:P' + $extra + '(A;OICI;FA;;;' + $UserSid + ')' } else { 'D:P(A;OICI;FA;;;' + $UserSid + ')' + $extra }
    return (New-Object Security.AccessControl.RawSecurityDescriptor($sddl))
  }
}
if ($request.operation -eq 'preflight') {
  $records = @()
  foreach ($kind in $fixtureKinds) {
    try {
      $descriptor = New-FixtureDescriptor $kind $sid.Value
      $bytes = New-Object byte[] $descriptor.BinaryLength; $descriptor.GetBinaryForm($bytes, 0)
      $roundTrip = New-Object Security.AccessControl.RawSecurityDescriptor($bytes.PSObject.BaseObject, 0)
      $aces = @()
      foreach ($ace in $roundTrip.DiscretionaryAcl) { $aces += @{type=$ace.AceType.ToString();mask=[int64]$ace.AccessMask;flags=[int]$ace.AceFlags;sid=$ace.SecurityIdentifier.Value} }
      $present = ($roundTrip.ControlFlags -band [Security.AccessControl.ControlFlags]::DiscretionaryAclPresent) -ne 0
      $protected = ($roundTrip.ControlFlags -band [Security.AccessControl.ControlFlags]::DiscretionaryAclProtected) -ne 0
      $nullDacl = $null -eq $roundTrip.DiscretionaryAcl
      if (-not $present -or $null -ne $roundTrip.Owner -or $null -ne $roundTrip.Group -or $null -ne $roundTrip.SystemAcl) { throw 'Fixture must contain only a present DACL' }
      if ($kind -eq 'null') {
        if (-not $nullDacl -or $aces.Count -ne 0) { throw 'Null DACL representation changed' }
      } elseif ($kind -eq 'empty') {
        if ($nullDacl -or -not $protected -or $aces.Count -ne 0) { throw 'Empty protected DACL representation changed' }
      } else {
        if ($nullDacl -or -not $protected -or -not @($aces | Where-Object {$_.type -eq 'AccessAllowed' -and $_.sid -eq $sid.Value -and $_.mask -eq 2032127 -and $_.flags -eq 3}).Count) { throw 'Caller full-control allow must remain exact' }
        if ($kind -eq 'deny-read' -and -not @($aces | Where-Object {$_.type -eq 'AccessDenied' -and $_.sid -eq 'S-1-1-0' -and $_.mask -eq 1 -and $_.flags -eq 0}).Count) { throw 'Deny must be exactly FILE_READ_DATA/LIST_DIRECTORY, not READ_CONTROL' }
      }
      $sha = [Security.Cryptography.SHA256]::Create()
      try { $digest = [BitConverter]::ToString($sha.ComputeHash($bytes)).Replace('-', '').ToLowerInvariant() } finally { $sha.Dispose() }
      $records += @{kind=$kind;success=$true;binaryBytes=$bytes.Length;binaryBase64=[Convert]::ToBase64String($bytes);sha256=$digest;controlFlags=[int]$roundTrip.ControlFlags;daclPresent=$present;daclProtected=$protected;nullDacl=$nullDacl;owner=$null;group=$null;sacl=$null;aceCount=$aces.Count;aces=$aces}
    } catch { $records += @{kind=$kind;success=$false;error=$_.Exception.ToString()} }
  }
  $successes = @($records | Where-Object {$_.success}).Count
  @{kind='ordinary-windows-fixture-descriptor-construction';declaredKinds=$fixtureKinds;plannedKinds=$fixtureKinds.Count;constructedKinds=$successes;failedKinds=($records.Count-$successes);pass=($records.Count -eq 7 -and $successes -eq 7);skip=0;records=$records;filesystemApplied=$false;nativeCasesExecuted=0;sid=$sid.Value;administrator=$false} | ConvertTo-Json -Depth 8 -Compress
  if ($records.Count -ne 7 -or $successes -ne 7) { exit 1 }; exit 0
}
if ($request.operation -eq 'apply') {
  $descriptor = New-FixtureDescriptor $request.kind $sid.Value
  $bytes = New-Object byte[] $descriptor.BinaryLength; $descriptor.GetBinaryForm($bytes, 0)
  $assembly = [AppDomain]::CurrentDomain.DefineDynamicAssembly((New-Object Reflection.AssemblyName('GestaltAclFixture')), [Reflection.Emit.AssemblyBuilderAccess]::Run)
  $module = $assembly.DefineDynamicModule('GestaltAclFixture'); $type = $module.DefineType('GestaltAclFixture', [Reflection.TypeAttributes]::Public)
  $attributes = [Reflection.MethodAttributes]::Public -bor [Reflection.MethodAttributes]::Static -bor [Reflection.MethodAttributes]::PinvokeImpl
  $method = $type.DefinePInvokeMethod('SetFileSecurityW', 'advapi32.dll', $attributes, [Reflection.CallingConventions]::Standard, [bool], [Type[]]@([string], [uint32], [byte[]]), [Runtime.InteropServices.CallingConvention]::Winapi, [Runtime.InteropServices.CharSet]::Unicode)
  $method.SetImplementationFlags([Reflection.MethodImplAttributes]::PreserveSig)
  $native = $type.CreateType()
  $fixedArguments = New-Object object[] 3
  $fixedArguments[0] = [string]$request.path
  $fixedArguments[1] = [uint32]2147483652
  $fixedArguments[2] = $bytes.PSObject.BaseObject
  if (-not $native.GetMethod('SetFileSecurityW').Invoke($null, $fixedArguments)) { throw 'DACL-only fixture construction failed' }
} elseif ($request.operation -ne 'inspect') { throw 'Unknown fixture operation' }
$item = Get-Item -LiteralPath $request.path -Force
$acl = $item.GetAccessControl(); $raw = New-Object Security.AccessControl.RawSecurityDescriptor($acl.GetSecurityDescriptorBinaryForm(), 0)
@{sid=$sid.Value;administrator=$false;owner=$acl.GetOwner([Security.Principal.SecurityIdentifier]).Value;group=$acl.GetGroup([Security.Principal.SecurityIdentifier]).Value;
  descriptor=$acl.GetSecurityDescriptorSddlForm([Security.AccessControl.AccessControlSections]::Owner -bor [Security.AccessControl.AccessControlSections]::Group -bor [Security.AccessControl.AccessControlSections]::Access);
  nullDacl=($null -eq $raw.DiscretionaryAcl);aceCount=$(if ($null -eq $raw.DiscretionaryAcl) {$null} else {$raw.DiscretionaryAcl.Count});attributes=$item.Attributes.ToString();
  rules=@($acl.GetAccessRules($true,$true,[Security.Principal.SecurityIdentifier]) | ForEach-Object {@{sid=$_.IdentityReference.Value;type=$_.AccessControlType.ToString();mask=[int64]$_.FileSystemRights;inheritance=$_.InheritanceFlags.ToString();propagation=$_.PropagationFlags.ToString()}})} | ConvertTo-Json -Depth 6 -Compress
