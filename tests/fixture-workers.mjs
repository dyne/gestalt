import {execFileSync} from 'node:child_process';
import {readFile} from 'node:fs/promises';
import assert from 'node:assert/strict';

// Teardown owns these workers independently of the supervisor under test. A
// failing shutdown assertion must not leave its SIGTERM-resistant fixture alive.
function identity(pid) {
  return execFileSync('ps', ['-o', 'lstart=', '-p', String(pid)], {encoding:'utf8', timeout:2000}).trim();
}
export function workerProgram(records, body) {
  return `{
const fs=require('node:fs');
const identity=process.platform==='linux'?fs.readFileSync('/proc/self/stat','utf8').split(') ').at(-1).split(' ')[19]:require('node:child_process').execFileSync('ps',['-o','lstart=','-p',String(process.pid)],{encoding:'utf8'}).trim();
fs.appendFileSync(${JSON.stringify(records)},JSON.stringify({pid:process.pid,identity})+'\\n');
}
${body}`;
}
export async function stopFixtureWorkers(records) {
  const text = await readFile(records, 'utf8').catch(error => {
    if (error.code === 'ENOENT') return '';
    throw error;
  });
  for (const {pid, identity:expected} of text.trim().split('\n').filter(Boolean).map(JSON.parse)) {
    assert.ok(Number.isSafeInteger(pid) && pid > 1 && expected);
    const running = async () => {
      try {
        if (process.platform !== 'linux') return identity(pid) === expected;
        const fields = (await readFile(`/proc/${pid}/stat`, 'utf8')).split(') ').at(-1).split(' ');
        return fields[19] === expected && fields[0] !== 'Z';
      } catch (error) {
        if (error.code === 'ENOENT' || error.status === 1) return false;
        throw error;
      }
    };
    if (!await running()) continue;
    try {process.kill(pid, 'SIGKILL');} catch (error) {if (error.code !== 'ESRCH') throw error;}
    const deadline = Date.now() + 2000;
    while (await running()) {
      assert.ok(Date.now() < deadline, `fixture worker ${pid} survived fallback teardown`);
      await new Promise(resolve => setTimeout(resolve, 20));
    }
  }
}
