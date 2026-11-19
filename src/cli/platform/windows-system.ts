import { execFile, spawn } from 'node:child_process';
import path from 'node:path';
import { promisify } from 'node:util';
import { assertWindowsReadReply, windowsReadArguments, windowsReadFailure } from '../runtime/windows-scratch-read.mjs';

export type WindowsReadHelper = () => Promise<string>;
type WindowsRequest = Record<string, unknown>;
type ReadReply = { stdout: Buffer; stderr: Buffer };
type WindowsRead = (executable: string, argv: string[]) => Promise<ReadReply>;
const execute = promisify(execFile);

function canonicalLocalDrivePath(value: unknown): boolean {
  if (typeof value !== 'string') return false;
  const plain = value.startsWith('\\\\?\\') ? value.slice(4) : value;
  if (!/^[a-z]:\\/i.test(plain) || /[\x00-\x1f\x7f<>:"|?*\/]/.test(plain.slice(2)) ||
      path.win32.normalize(plain) !== plain) return false;
  if (plain.length === 3) return true;
  return plain.slice(3).split('\\').every(component => component && !/[ .]$/.test(component) &&
    !/^(?:con|prn|aux|nul|conin\$|conout\$|com[0-9¹²³]|lpt[0-9¹²³])(?:\.|$)/i.test(component));
}

function completeReadRejection(error: unknown): string | undefined {
  const result = error as (Partial<ReadReply> & { code?: unknown; signal?: unknown; killed?: unknown }) | undefined;
  if (result?.code !== 1 || result.signal !== null || result.killed !== false ||
      !Buffer.isBuffer(result.stdout) || result.stdout.length !== 0 || !Buffer.isBuffer(result.stderr)) return undefined;
  return result.stderr.toString('utf8');
}

function unsupportedAce(diagnostic: string | undefined, pathCount: number): boolean {
  if (diagnostic === undefined) return false;
  const match = /^GESTALT_STORAGE_CHECK_V1 ace error=50 index=(0|[1-9][0-9]{0,2})\n$/.exec(diagnostic);
  return match !== null && match[0] === diagnostic && Number(match[1]) < pathCount;
}

// Selection is independent of host platform so the routing contract can be
// exercised without launching a Windows executable.
export async function routeWindowsStorageRead(request: WindowsRequest, helper: WindowsReadHelper | undefined,
  legacy: (request: WindowsRequest) => Promise<void>, read: WindowsRead): Promise<void> {
  const paths = request.operation === 'assert-private' ? [request.path] : request.operation === 'assert-namespace' ? request.paths : null;
  if (!helper || !Array.isArray(paths) || !paths.length || paths.length > 128 ||
      !Array.from(paths).every(canonicalLocalDrivePath)) return legacy(request);
  const argv = windowsReadArguments(request);
  // Bound the total UTF-16 length conservatively for quoting, including argv[0]
  // and the trailing NUL. Ineligible requests retain their original adapter.
  const units = (value: string) => 2 * value.length + 3;
  const argumentUnits = 1 + argv.reduce((length, value) => length + units(value), 0);
  if (argumentUnits >= 32767) return legacy(request);
  const executable = await helper();
  const unc = /^\\\\\?\\UNC\\/i.test(executable) ? '\\\\' + executable.slice(8) : executable;
  if (!/\.exe$/i.test(executable) || /[\x00-\x1f\x7f]/.test(executable))
    throw new Error('Windows storage read helper must be an absolute executable path');
  if (/^\\\\[^?.\\][^\\]*\\[^\\]+\\/.test(unc)) return legacy(request);
  if (!canonicalLocalDrivePath(executable)) throw new Error('Windows storage read helper must be an absolute executable path');
  if (argumentUnits + units(executable) > 32767) return legacy(request);
  let reply: ReadReply;
  try { reply = await read(executable, argv); }
  catch (error) {
    const diagnostic = completeReadRejection(error);
    if (unsupportedAce(diagnostic, paths.length)) return legacy(request);
    if (request.operation === 'assert-private' && diagnostic === 'GESTALT_STORAGE_CHECK_V1 ace error=5 index=0\n')
      throw new Error('Windows private storage: ACL permits another principal', { cause: error });
    throw new Error(windowsReadFailure(error), { cause: error });
  }
  if (reply.stderr.length !== 0) throw new Error('Windows native storage read produced unexpected diagnostics');
  assertWindowsReadReply(reply.stdout);
}

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
  if ($request.operation -eq 'assert-namespace') {
    $trusted = @($sid.Value, 'S-1-5-18', 'S-1-5-32-544', 'S-1-5-80-956008885-3418522649-1831038044-1853292631-2271478464')
    $adminMembers = $null
    function Is-TrustedNamespacePrincipal([string]$identity) {
      if ($identity -in $trusted) { return $true }
      if ($identity -notmatch '^S-1-(5-21|12-1)-') { return $false }
      if ($null -eq $script:adminMembers) {
        $script:adminMembers = @(Get-LocalGroupMember -SID 'S-1-5-32-544' -ErrorAction Stop | Where-Object { $_.ObjectClass -eq 'User' } | ForEach-Object { $_.SID.Value })
      }
      return $identity -in $script:adminMembers
    }
    $mutating = [Security.AccessControl.FileSystemRights]::Delete -bor [Security.AccessControl.FileSystemRights]::DeleteSubdirectoriesAndFiles -bor [Security.AccessControl.FileSystemRights]::ChangePermissions -bor [Security.AccessControl.FileSystemRights]::TakeOwnership -bor [Security.AccessControl.FileSystemRights]::WriteData -bor [Security.AccessControl.FileSystemRights]::WriteAttributes -bor [Security.AccessControl.FileSystemRights]::WriteExtendedAttributes
    foreach ($ancestor in $request.paths) {
      $item = Get-Item -LiteralPath $ancestor -Force
      if (-not $item.PSIsContainer -or ($item.Attributes -band [IO.FileAttributes]::ReparsePoint) -ne 0) { throw "Redirected namespace parent: $ancestor" }
      $acl = $item.GetAccessControl()
      $raw = New-Object Security.AccessControl.RawSecurityDescriptor($acl.GetSecurityDescriptorBinaryForm(), 0)
      if ($null -eq $raw.DiscretionaryAcl) { throw "Unrestricted namespace parent ACL: $ancestor" }
      if (-not (Is-TrustedNamespacePrincipal $acl.GetOwner([Security.Principal.SecurityIdentifier]).Value)) { throw "Untrusted namespace parent owner: $ancestor" }
      foreach ($rule in $acl.GetAccessRules($true, $true, [Security.Principal.SecurityIdentifier])) {
        if (($rule.PropagationFlags -band [Security.AccessControl.PropagationFlags]::InheritOnly) -ne 0) { continue }
        if ($rule.AccessControlType -eq 'Allow' -and ($rule.FileSystemRights -band $mutating) -ne 0 -and -not (Is-TrustedNamespacePrincipal $rule.IdentityReference.Value)) { throw "Untrusted namespace parent can redirect managed storage: $ancestor" }
      }
    }
  } elseif ($request.operation -eq 'create-directory') {
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

export function startWindowsSystem(request: WindowsRequest) {
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

async function legacyWindowsSystem(request: WindowsRequest): Promise<void> {
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

export async function windowsSystem(request: WindowsRequest, helper?: WindowsReadHelper): Promise<void> {
  if (process.platform !== 'win32') throw new Error('Windows OS adapter requires Windows');
  return routeWindowsStorageRead(request, helper, legacyWindowsSystem, (executable, argv) =>
    execute(executable, argv, { encoding: 'buffer', timeout: 10000, maxBuffer: 16384, windowsHide: true, shell: false }));
}
