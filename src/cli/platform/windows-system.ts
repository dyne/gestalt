import { spawn } from 'node:child_process';
import path from 'node:path';

// Original implementation, MIT licensed (see docs/native-platform-adapters.md).
// Fixed source only: request values are JSON data, never PowerShell or C# source.
export const windowsSystemScript = String.raw`
$ErrorActionPreference = 'Stop'
$ProgressPreference = 'SilentlyContinue'
try {
  $request = [Text.Encoding]::UTF8.GetString([Convert]::FromBase64String($env:GESTALT_NATIVE_REQUEST)) | ConvertFrom-Json
  $sid = [Security.Principal.WindowsIdentity]::GetCurrent().User
  function Assert-PrivateAcl($acl) {
    if ($acl.GetOwner([Security.Principal.SecurityIdentifier]).Value -ne $sid.Value) { throw 'Storage owner differs from current user' }
    $full = $false
    foreach ($rule in $acl.GetAccessRules($true, $true, [Security.Principal.SecurityIdentifier])) {
      if ($rule.AccessControlType -eq 'Allow') {
        if ($rule.IdentityReference.Value -notin @($sid.Value, 'S-1-5-18')) { throw 'ACL permits another principal' }
        if ($rule.IdentityReference.Value -eq $sid.Value -and (($rule.FileSystemRights -band 2032127) -eq 2032127)) { $full = $true }
      }
    }
    if (-not $full) { throw 'Current user lacks private full control' }
  }
  if ($request.operation -eq 'create-directory') {
    if ([IO.Directory]::Exists($request.path) -or [IO.File]::Exists($request.path)) { throw 'Destination already exists' }
    $acl = New-Object Security.AccessControl.DirectorySecurity
    $acl.SetOwner($sid)
    $acl.SetAccessRuleProtection($true, $false)
    $rule = New-Object Security.AccessControl.FileSystemAccessRule($sid, 'FullControl', 'ContainerInherit,ObjectInherit', 'None', 'Allow')
    $acl.AddAccessRule($rule)
    $directory = New-Object IO.DirectoryInfo($request.path)
    $directory.Create($acl)
    Assert-PrivateAcl ($directory.GetAccessControl())
  } elseif ($request.operation -eq 'assert-private') {
    $item = Get-Item -LiteralPath $request.path -Force
    if (($item.Attributes -band [IO.FileAttributes]::ReparsePoint) -ne 0) { throw 'Reparse point refused' }
    Assert-PrivateAcl ($item.GetAccessControl())
  } elseif ($request.operation -eq 'replace' -or $request.operation -eq 'connect-pipe') {
    # Define fixed P/Invoke signatures with Reflection.Emit: no runtime C#/TS
    # compiler, downloaded DLL, source interpolation or persistent build product.
    $assemblyName = New-Object Reflection.AssemblyName('GestaltNativeSystem')
    $assembly = [AppDomain]::CurrentDomain.DefineDynamicAssembly($assemblyName, [Reflection.Emit.AssemblyBuilderAccess]::Run)
    $module = $assembly.DefineDynamicModule('GestaltNativeSystem')
    $type = $module.DefineType('GestaltNativeSystem', [Reflection.TypeAttributes]::Public)
    $attributes = [Reflection.MethodAttributes]::Public -bor [Reflection.MethodAttributes]::Static -bor [Reflection.MethodAttributes]::PinvokeImpl
    $move = $type.DefinePInvokeMethod('MoveFileExW', 'kernel32.dll', $attributes, [Reflection.CallingConventions]::Standard, [bool], [Type[]]@([string], [string], [uint32]), [Runtime.InteropServices.CallingConvention]::Winapi, [Runtime.InteropServices.CharSet]::Unicode)
    $move.SetImplementationFlags([Reflection.MethodImplAttributes]::PreserveSig)
    $peer = $type.DefinePInvokeMethod('GetNamedPipeServerProcessId', 'kernel32.dll', $attributes, [Reflection.CallingConventions]::Standard, [bool], [Type[]]@([Microsoft.Win32.SafeHandles.SafePipeHandle], [uint32].MakeByRefType()), [Runtime.InteropServices.CallingConvention]::Winapi, [Runtime.InteropServices.CharSet]::Auto)
    $peer.SetImplementationFlags([Reflection.MethodImplAttributes]::PreserveSig)
    $native = $type.CreateType()
    if ($request.operation -eq 'replace') {
      if (-not $native.GetMethod('MoveFileExW').Invoke($null, [object[]]@($request.source, $request.path, [uint32]9))) { throw 'Destination-local MoveFileExW failed' }
    } else {
      $pipe = New-Object IO.Pipes.NamedPipeClientStream('.', $request.name, [IO.Pipes.PipeDirection]::InOut, [IO.Pipes.PipeOptions]::Asynchronous, [Security.Principal.TokenImpersonationLevel]::Identification)
      try {
        $pipe.Connect(5000)
        $acl = $pipe.GetAccessControl()
        if ($acl.GetOwner([Security.Principal.SecurityIdentifier]).Value -ne $sid.Value) { throw 'Pipe owner differs from current user' }
        foreach ($rule in $acl.GetAccessRules($true, $true, [Security.Principal.SecurityIdentifier])) {
          if ($rule.AccessControlType -eq 'Allow' -and $rule.IdentityReference.Value -notin @($sid.Value, 'S-1-5-18')) { throw 'Pipe ACL permits another principal' }
        }
        $peerArguments = [object[]]@($pipe.SafePipeHandle, [uint32]0)
        if (-not $native.GetMethod('GetNamedPipeServerProcessId').Invoke($null, $peerArguments)) { throw 'GetNamedPipeServerProcessId failed' }
        [uint32]$serverPid = $peerArguments[1]
        if ($serverPid -ne $request.owner.pid) { throw 'Pipe server PID differs from expected owner' }
        $server = [Diagnostics.Process]::GetProcessById($serverPid)
        if ($server.StartTime.ToUniversalTime().Ticks.ToString() -ne $request.owner.started -or
            -not [String]::Equals($server.MainModule.FileName, $request.owner.executable, [StringComparison]::OrdinalIgnoreCase)) { throw 'Pipe server creation identity differs from expected owner' }
        [Console]::Error.WriteLine('GESTALT_NATIVE_READY')
        $inputTask = [Console]::OpenStandardInput().CopyToAsync($pipe)
        $outputTask = $pipe.CopyToAsync([Console]::OpenStandardOutput())
        # Parent stdin EOF also closes the proxy, including abrupt controller exit.
        $completed = [Threading.Tasks.Task]::WhenAny($inputTask, $outputTask).GetAwaiter().GetResult()
        [void]$completed.GetAwaiter().GetResult()
      } finally { $pipe.Dispose() }
    }
  } else { throw 'Unknown native system operation' }
} catch { [Console]::Error.WriteLine($_.Exception.Message); exit 1 }
`;

export function startWindowsSystem(request: Record<string, unknown>) {
  if (process.platform !== 'win32') throw new Error('Windows OS adapter requires Windows');
  const systemRoot = process.env.SystemRoot;
  if (!systemRoot || !path.isAbsolute(systemRoot)) throw new Error('Windows SystemRoot is unavailable');
  const nativeRequest = { ...request };
  for (const key of ['path', 'source']) if (typeof nativeRequest[key] === 'string') nativeRequest[key] = path.toNamespacedPath(nativeRequest[key]);
  return spawn(path.join(systemRoot, 'System32', 'WindowsPowerShell', 'v1.0', 'powershell.exe'), [
    '-NoLogo', '-NoProfile', '-NonInteractive', '-EncodedCommand', Buffer.from(windowsSystemScript, 'utf16le').toString('base64'),
  ], {
    shell: false, windowsHide: true, stdio: ['pipe', 'pipe', 'pipe'],
    env: { ...process.env, GESTALT_NATIVE_REQUEST: Buffer.from(JSON.stringify(nativeRequest)).toString('base64') },
  });
}

export async function windowsSystem(request: Record<string, unknown>): Promise<void> {
  const child = startWindowsSystem(request);
  child.stdin.end();
  let error = '';
  child.stderr.setEncoding('utf8');
  child.stderr.on('data', bytes => { error = (error + bytes).slice(-8192); });
  child.stdout.resume();
  await new Promise<void>((resolve, reject) => {
    child.once('error', reject);
    child.once('close', code => code === 0 ? resolve() : reject(new Error(`Windows private storage: ${error.trim() || `exit ${code}`}`)));
  });
}
