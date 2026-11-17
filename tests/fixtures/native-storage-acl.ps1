# Isolated ordinary-user fixture only. DACL-only changes; no owner/group/SACL write.
$ErrorActionPreference = 'Stop'
$request = [Text.Encoding]::UTF8.GetString([Convert]::FromBase64String($env:GESTALT_STORAGE_PARITY_REQUEST)) | ConvertFrom-Json
$sid = [Security.Principal.WindowsIdentity]::GetCurrent().User
$principal = New-Object Security.Principal.WindowsPrincipal([Security.Principal.WindowsIdentity]::GetCurrent())
if ($principal.IsInRole([Security.Principal.WindowsBuiltInRole]::Administrator)) { throw 'ACL parity requires the ordinary consumer token' }
if ($request.operation -eq 'apply') {
  if ($request.kind -eq 'null') {
    $descriptor = New-Object Security.AccessControl.RawSecurityDescriptor([Security.AccessControl.ControlFlags]::DiscretionaryAclPresent, $null, $null, $null, $null)
  } else {
    $extra = switch ($request.kind) {
      'private' { '' }
      'others-read' { '(A;;FR;;;WD)' }
      'others-write' { '(A;;WD;;;WD)' }
      'inherit-only' { '(A;OICIIO;WD;;;WD)' }
      'deny-read' { '(D;;RD;;;WD)' }
      'empty' { '' }
      default { throw 'Unknown fixed ACL fixture' }
    }
    $sddl = if ($request.kind -eq 'empty') { 'D:P' } elseif ($request.kind -eq 'deny-read') { 'D:P' + $extra + '(A;OICI;FA;;;' + $sid.Value + ')' } else { 'D:P(A;OICI;FA;;;' + $sid.Value + ')' + $extra }
    $descriptor = New-Object Security.AccessControl.RawSecurityDescriptor($sddl)
  }
  $bytes = New-Object byte[] $descriptor.BinaryLength; $descriptor.GetBinaryForm($bytes, 0)
  $assembly = [AppDomain]::CurrentDomain.DefineDynamicAssembly((New-Object Reflection.AssemblyName('GestaltAclFixture')), [Reflection.Emit.AssemblyBuilderAccess]::Run)
  $module = $assembly.DefineDynamicModule('GestaltAclFixture'); $type = $module.DefineType('GestaltAclFixture', [Reflection.TypeAttributes]::Public)
  $attributes = [Reflection.MethodAttributes]::Public -bor [Reflection.MethodAttributes]::Static -bor [Reflection.MethodAttributes]::PinvokeImpl
  $method = $type.DefinePInvokeMethod('SetFileSecurityW', 'advapi32.dll', $attributes, [Reflection.CallingConventions]::Standard, [bool], [Type[]]@([string], [uint32], [byte[]]), [Runtime.InteropServices.CallingConvention]::Winapi, [Runtime.InteropServices.CharSet]::Unicode)
  $method.SetImplementationFlags([Reflection.MethodImplAttributes]::PreserveSig)
  $native = $type.CreateType()
  if (-not $native.GetMethod('SetFileSecurityW').Invoke($null, [object[]]@($request.path, [uint32]2147483652, $bytes))) { throw 'DACL-only fixture construction failed' }
} elseif ($request.operation -ne 'inspect') { throw 'Unknown fixture operation' }
$item = Get-Item -LiteralPath $request.path -Force
$acl = $item.GetAccessControl(); $raw = New-Object Security.AccessControl.RawSecurityDescriptor($acl.GetSecurityDescriptorBinaryForm(), 0)
@{sid=$sid.Value;administrator=$false;owner=$acl.GetOwner([Security.Principal.SecurityIdentifier]).Value;group=$acl.GetGroup([Security.Principal.SecurityIdentifier]).Value;
  descriptor=$acl.GetSecurityDescriptorSddlForm([Security.AccessControl.AccessControlSections]::Owner -bor [Security.AccessControl.AccessControlSections]::Group -bor [Security.AccessControl.AccessControlSections]::Access);
  nullDacl=($null -eq $raw.DiscretionaryAcl);aceCount=$(if ($null -eq $raw.DiscretionaryAcl) {$null} else {$raw.DiscretionaryAcl.Count});attributes=$item.Attributes.ToString();
  rules=@($acl.GetAccessRules($true,$true,[Security.Principal.SecurityIdentifier]) | ForEach-Object {@{sid=$_.IdentityReference.Value;type=$_.AccessControlType.ToString();mask=[int64]$_.FileSystemRights;inheritance=$_.InheritanceFlags.ToString();propagation=$_.PropagationFlags.ToString()}})} | ConvertTo-Json -Depth 6 -Compress
