import assert from 'node:assert/strict';
import {readFile} from 'node:fs/promises';

// This mock's process starts are not browser ownership claims. One transport
// must actually initialize and serve the catalog; canceled zero-input starts
// are admissible only with complete, clean, bounded termination evidence.
export function classifyChildTransports(rawStarts,events,budgetMs=5000) {
  assert.ok(Array.isArray(rawStarts)&&Array.isArray(events));
  assert.ok(Number.isFinite(budgetMs)&&budgetMs>0);
  const starts=events.filter(row=>row.kind==='start'),instances=new Map();
  assert.equal(starts.length,rawStarts.length,'unaccounted MCP process starts');
  for(const raw of rawStarts){
    assert.ok(Number.isSafeInteger(raw.pid)&&raw.pid>1&&Number.isSafeInteger(raw.ppid)&&raw.ppid>0&&/^\d+$/.test(raw.identity), 'unknown MCP start identity');
    assert.ok(!instances.has(raw.pid),'duplicate MCP start PID');
    const matches=starts.filter(row=>row.pid===raw.pid&&row.identity===raw.identity&&row.ppid===raw.ppid);
    assert.equal(matches.length,1,'missing or changed MCP start identity');instances.set(raw.pid,raw);
  }
  const kinds=new Set(['start','input','rpc','notification','served','stdio-end','stdio-close','natural-exit','write-error']);
  for(const row of events){
    assert.ok(kinds.has(row.kind),'unknown or truncated MCP trace');
    assert.equal(instances.get(row.pid)?.identity,row.identity,'unattributed MCP trace identity');
    assert.ok(Number.isFinite(Date.parse(row.at)),'invalid MCP trace timestamp');
    assert.notEqual(row.kind,'write-error','MCP response did not finish writing');
  }
  const serving=[],canceled=[];
  for(const start of starts){
    const rows=events.filter(row=>row.pid===start.pid);
    const protocol=rows.filter(row=>['input','rpc','notification','served','write-error'].includes(row.kind));
    const received=method=>rows.some(row=>row.kind==='rpc'&&row.method===method);
    const served=method=>rows.some(row=>row.kind==='served'&&row.method===method);
    if(received('initialize')&&received('tools/list')&&served('initialize')&&served('tools/list')){
      serving.push(start);continue;
    }
    assert.equal(protocol.length,0,'additional MCP process negotiated or handled protocol');
    const exactly=kind=>{const matches=rows.filter(row=>row.kind===kind);assert.equal(matches.length,1,'missing or ambiguous MCP '+kind);return matches[0];};
    const eof=exactly('stdio-end'),close=exactly('stdio-close'),exit=exactly('natural-exit');
    assert.equal(exit.code,0,'unclean uninitialized MCP exit');
    const times=[start,eof,close,exit].map(row=>Date.parse(row.at));
    assert.ok(times.every((at,i)=>i===0||at>=times[i-1]),'inconsistent MCP cancellation order');
    assert.ok(times[3]-times[0]<=budgetMs,'uninitialized MCP termination exceeded lifecycle budget');
    canceled.push(start);
  }
  assert.equal(serving.length,1,'expected exactly one initialized catalog-serving MCP transport');
  return{serving,canceled,instances:starts};
}

function procIdentity(text,pid) {
  const fields=text.slice(text.lastIndexOf(')')+2).trim().split(' ');
  assert.equal(Number(text.slice(0,text.indexOf(' '))),pid,'unexpected process identity read');
  assert.ok(/^\d+$/.test(fields[19]??'')&&/^[A-Za-z]$/.test(fields[0]??''),'unreadable process identity');
  return{identity:fields[19],state:fields[0]};
}
async function inspectLinuxProcess(instance) {
  // ENOENT means absence only while this procfs is demonstrably readable.
  procIdentity(await readFile('/proc/self/stat','utf8'),process.pid);
  try{return procIdentity(await readFile('/proc/'+instance.pid+'/stat','utf8'),instance.pid);}
  catch(error){
    if(!['ENOENT','ESRCH'].includes(error.code))throw error;
    procIdentity(await readFile('/proc/self/stat','utf8'),process.pid);return null;
  }
}
export async function verifyChildTransports(rawStarts,events,{budgetMs=5000,inspect=inspectLinuxProcess,
  now=()=>performance.now(),pause=ms=>new Promise(done=>setTimeout(done,ms))}={}) {
  const result=classifyChildTransports(rawStarts,events,budgetMs),deadline=now()+budgetMs;
  while(true){
    let present=false;
    for(const instance of result.instances){
      const current=await inspect(instance);
      if(current===null)continue;
      assert.ok(current&&/^[A-Za-z]$/.test(current.state??''),'unknown MCP process state');
      assert.equal(current.identity,instance.identity,'MCP process identity changed; absence unconfirmed');
      present=true; // A live process or unreaped zombie is not absence.
    }
    if(!present)return result;
    assert.ok(now()<deadline,'owned MCP process absence not confirmed within lifecycle budget');
    await pause(25);
  }
}
