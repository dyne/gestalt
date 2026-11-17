const quote = value => "'" + value.replaceAll("'", "'\\'" + "'") + "'";

/** Codex's native Windows hook runner uses COMSPEC /C by default. Its command
 * is ASCII data; Windows PowerShell transports raw streams to Node without
 * cmd expansion of paths, hook JSON, capability text or JavaScript source. */
export function nodeHookCommand(program, node = process.execPath, platform = process.platform) {
  if (platform !== 'win32') return quote(node) + ' --input-type=module -e ' + quote(program);
  // Repeated role paths and the startup emitter otherwise exceed Windows'
  // command-line limit when two native hook events carry the same handler.
  const loader = "const{inflateRawSync}=await import('node:zlib');await import('data:text/javascript;base64,'+inflateRawSync(Buffer.from('" + deflateRawSync(Buffer.from(program)).toString('base64') + "','base64')).toString('base64'))";
  const psLiteral = value => "'" + value.replaceAll("'", "''") + "'";
  const nodeArguments = '--input-type=module -e "' + loader + '"';
  const bridge = `$ErrorActionPreference='Stop'
$ProgressPreference='SilentlyContinue'
$i=New-Object Diagnostics.ProcessStartInfo
$i.FileName=${psLiteral(node)};$i.Arguments=${psLiteral(nodeArguments)};$i.UseShellExecute=$false;$i.CreateNoWindow=$true
$i.RedirectStandardInput=$true;$i.RedirectStandardOutput=$true;$i.RedirectStandardError=$true
$p=[Diagnostics.Process]::Start($i)
try {
 $o=$p.StandardOutput.BaseStream.CopyToAsync([Console]::OpenStandardOutput())
 $e=$p.StandardError.BaseStream.CopyToAsync([Console]::OpenStandardError())
 try {[void][Console]::OpenStandardInput().CopyToAsync($p.StandardInput.BaseStream).GetAwaiter().GetResult()} catch [IO.IOException] {}
 $p.StandardInput.Close();$p.WaitForExit();[void]$o.GetAwaiter().GetResult();[void]$e.GetAwaiter().GetResult()
 $code=$p.ExitCode
} finally {$p.Dispose()}
exit $code`;
  const command = 'powershell.exe -NoLogo -NoProfile -NonInteractive -EncodedCommand ' + Buffer.from(bridge, 'utf16le').toString('base64');
  if (command.length > 30000) throw new Error('NATIVE_HOOK_COMMAND_TOO_LONG: reduce selected role metadata before enabling startup guidance');
  return command;
}

export function associationHookProgram(managerUrl, context, associate) {
  return (associate ? `const api=await import(${JSON.stringify(managerUrl)});
if(api.SESSION_SCRATCH_API_VERSION!==1||typeof api.associateSessionScratch!=='function')throw Error('managed scratch association API unavailable');
const write=s=>b=>new Promise((resolve,reject)=>s.write(b,e=>e?reject(e):resolve()));
const status=await api.associateSessionScratch({argv:[],cwd:process.cwd(),environment:{...process.env}},{stdin:process.stdin,stdout:write(process.stdout),stderr:write(process.stderr)});
process.exitCode=status;if(status===0)` : '') + `process.stdout.write(${JSON.stringify(context)});`;
}
import { deflateRawSync } from 'node:zlib';
