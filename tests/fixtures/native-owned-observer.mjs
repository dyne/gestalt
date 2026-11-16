// Native test observation only. These identities never authorize product
// signalling, stale-lock recovery or complete-tree release.
import assert from 'node:assert/strict';
import { spawn } from 'node:child_process';
import { readFile, readdir } from 'node:fs/promises';
import path from 'node:path';
export async function until(check, label, milliseconds = 15000) {
  const deadline = Date.now() + milliseconds;
  while (!await check()) {
    if (Date.now() >= deadline) throw new Error(label);
    await new Promise(resolve => setTimeout(resolve, 25));
  }
}
export async function linuxIdentities(environmentEntry) {
  const records = [];
  for (const pid of (await readdir('/proc')).filter(name => /^\d+$/.test(name))) {
    try {
      if (!(await readFile(`/proc/${pid}/environ`, 'utf8')).split('\0').includes(environmentEntry)) continue;
      const text = await readFile(`/proc/${pid}/stat`, 'utf8'); records.push({ pid: Number(pid), started: text.slice(text.lastIndexOf(')') + 2).split(' ')[19] });
    } catch (error) { if (!['ENOENT', 'ESRCH', 'EACCES', 'EPERM'].includes(error.code)) throw error; }
  }
  return records;
}
export async function assertLinuxAbsent(records) {
  await until(async () => {
    for (const record of records) {
      try { const text = await readFile(`/proc/${record.pid}/stat`, 'utf8'); if (text.slice(text.lastIndexOf(')') + 2).split(' ')[19] === record.started) return false; }
      catch (error) { if (!['ENOENT', 'ESRCH'].includes(error.code)) throw error; }
    }
    return true;
  }, 'captured native process creation identities remain after cleanup');
}
export async function windowsObserver(pid) {
  const script = "$ErrorActionPreference='Stop';$p=[Diagnostics.Process]::GetProcessById([int]$env:GESTALT_TEST_OBSERVE_PID);[void]$p.Handle;[Console]::Out.WriteLine('held:'+ $p.Id + ':' + $p.StartTime.ToUniversalTime().Ticks);[Console]::In.ReadLine()|Out-Null;if(-not $p.WaitForExit(15000)){throw 'Captured native descendant remains alive'};[Console]::Out.WriteLine('absent')";
  const child = spawn(path.join(process.env.SystemRoot, 'System32/WindowsPowerShell/v1.0/powershell.exe'), ['-NoLogo', '-NoProfile', '-NonInteractive', '-EncodedCommand', Buffer.from(script, 'utf16le').toString('base64')],
    { env: { ...process.env, GESTALT_TEST_OBSERVE_PID: String(pid) }, shell: false, stdio: ['pipe', 'pipe', 'pipe'] });
  let stdout = '', stderr = '', exit;
  const completion = new Promise((resolve, reject) => { child.once('error', reject); child.once('close', status => { exit = status; resolve(status); }); });
  void completion.catch(() => {}); child.stdin.on('error', () => {});
  child.stdout.on('data', bytes => stdout += bytes); child.stderr.on('data', bytes => stderr += bytes);
  try { await until(() => { if (exit !== undefined) throw new Error(stderr); return /held:\d+:\d+/.test(stdout); }, 'native observer did not capture a process handle'); }
  catch (error) { if (child.exitCode === null && child.signalCode === null) child.kill('SIGKILL'); await completion.catch(() => {}); throw error; }
  let verified = false;
  return { identity: stdout.match(/held:(\d+):(\d+)/).slice(1), async verify() {
    if (verified) return; child.stdin.end('\n'); assert.equal(await completion, 0, stderr); assert.match(stdout, /absent/); verified = true;
  } };
}
