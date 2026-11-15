# Test-only original MIT source. Never provisions profiles or live services.
$ErrorActionPreference = 'Stop'
$ProgressPreference = 'SilentlyContinue'
$request = [Text.Encoding]::UTF8.GetString([Convert]::FromBase64String($env:GESTALT_PIPE_FIXTURE)) | ConvertFrom-Json
$sid = [Security.Principal.WindowsIdentity]::GetCurrent().User
$acl = New-Object IO.Pipes.PipeSecurity
$acl.SetOwner($sid)
$acl.SetAccessRuleProtection($true, $false)
$acl.AddAccessRule((New-Object IO.Pipes.PipeAccessRule($sid, 'FullControl', 'Allow')))
$self = [Diagnostics.Process]::GetCurrentProcess()
$identity = @{ pid = $self.Id; started = $self.StartTime.ToUniversalTime().Ticks.ToString(); executable = $self.MainModule.FileName }
for ($index = 0; $index -lt 2; $index++) {
  $pipe = New-Object IO.Pipes.NamedPipeServerStream($request.name, [IO.Pipes.PipeDirection]::InOut, 1, [IO.Pipes.PipeTransmissionMode]::Byte, [IO.Pipes.PipeOptions]::Asynchronous, 4096, 4096, $acl)
  try {
    if ($index -eq 0) { [Console]::Out.WriteLine(($identity | ConvertTo-Json -Compress)) }
    $pipe.WaitForConnection()
    $buffer = New-Object byte[] 4096
    $count = $pipe.Read($buffer, 0, $buffer.Length)
    if ($count -gt 0) { $pipe.Write($buffer, 0, $count); $pipe.Flush() }
  } finally { $pipe.Dispose() }
}
