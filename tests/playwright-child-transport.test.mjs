import assert from 'node:assert/strict';
import test from 'node:test';
import {classifyChildTransports,verifyChildTransports} from './playwright-child-transport-fixture.mjs';

function fixture() {
  const raw=[{pid:101,ppid:100,identity:'1000'},{pid:102,ppid:100,identity:'2000'}];
  const row=(index,kind,ms,extra={})=>({...raw[index],kind,at:new Date(ms).toISOString(),...extra});
  const serving=index=>[row(index,'input',1),row(index,'rpc',1,{method:'initialize'}),
    row(index,'served',2,{method:'initialize'}),row(index,'rpc',2,{method:'tools/list'}),
    row(index,'served',3,{method:'tools/list'})];
  const events=[row(0,'start',0),...serving(0),row(1,'start',10),
    row(1,'stdio-end',11),row(1,'stdio-close',12),row(1,'natural-exit',12,{code:0})];
  return{raw,events,row,serving};
}
test('one serving transport and completely canceled start require actual process absence',async()=>{
  const f=fixture(),inspected=[];
  const result=await verifyChildTransports(f.raw,f.events,{inspect:async instance=>{inspected.push(instance.pid);return null;}});
  assert.equal(result.serving.length,1);assert.equal(result.canceled.length,1);
  assert.deepEqual(inspected,[101,102]);
});
test('one serving transport needs no canceled extra',()=>{
  const f=fixture();assert.equal(classifyChildTransports(f.raw.slice(0,1),f.events.filter(row=>row.pid===101)).serving.length,1);
});
const traceFailures=[
  ['second serving transport',f=>f.events.push(...f.serving(1))],
  ['partial initialize',f=>f.events.push(f.row(1,'rpc',11,{method:'initialize'}))],
  ['extra input without a complete RPC',f=>f.events.push(f.row(1,'input',11,{bytes:1}))],
  ['extra notification',f=>f.events.push(f.row(1,'notification',11,{method:'notifications/initialized'}))],
  ['missing trace',f=>{f.events=[];}],
  ['unaccounted raw start',f=>f.raw.push({pid:103,ppid:100,identity:'3000'})],
  ['unaccounted trace start',f=>f.raw.pop()],
  ['changed raw identity',f=>{f.raw[1]={...f.raw[1],identity:'9999'};}],
  ['unknown start identity',f=>{delete f.raw[1].identity;}],
  ['duplicate raw PID',f=>{f.raw[1]={...f.raw[0]};}],
  ['missing EOF',f=>{f.events=f.events.filter(row=>row.kind!=='stdio-end');}],
  ['missing close',f=>{f.events=f.events.filter(row=>row.kind!=='stdio-close');}],
  ['missing natural exit',f=>{f.events=f.events.filter(row=>row.kind!=='natural-exit');}],
  ['nonzero exit',f=>{f.events.find(row=>row.kind==='natural-exit').code=1;}],
  ['cancellation over budget',f=>{f.events.find(row=>row.kind==='natural-exit').at=new Date(5011).toISOString();}],
  ['inconsistent cancellation order',f=>{f.events.find(row=>row.kind==='stdio-end').at=new Date(13).toISOString();}],
  ['catalog response not served',f=>{f.events=f.events.filter(row=>!(row.kind==='served'&&row.method==='tools/list'));}],
  ['response write failed',f=>f.events.push(f.row(0,'write-error',4))],
  ['truncated trace',f=>f.events.push(f.row(0,'truncated',4))],
  ['unattributed protocol',f=>f.events.push({...f.row(0,'rpc',4),pid:103})],
];
for(const [label,mutate] of traceFailures)test('rejects '+label,()=>{
  const f=fixture();mutate(f);assert.throws(()=>classifyChildTransports(f.raw,f.events));
});
const states=[
  ['live extra',async()=>({identity:'2000',state:'S'})],
  ['unreaped extra',async()=>({identity:'2000',state:'Z'})],
  ['unknown extra',async()=>undefined],
  ['reused PID',async()=>({identity:'3000',state:'S'})],
  ['unreadable process',async()=>{throw Object.assign(new Error('unreadable procfs'),{code:'EACCES'});}],
];
for(const [label,inspectExtra] of states)test('fails closed for '+label,async()=>{
  const f=fixture();let clock=0;
  await assert.rejects(verifyChildTransports(f.raw,f.events,{budgetMs:5,now:()=>clock,pause:async()=>{clock+=25;},
    inspect:async instance=>instance.pid===101?null:inspectExtra()}));
});
test('waits for both original process instances to disappear within the same budget',async()=>{
  const f=fixture();let clock=0,checks=0;
  await verifyChildTransports(f.raw,f.events,{budgetMs:5,now:()=>clock,pause:async()=>{clock+=1;},
    inspect:async instance=>++checks<=2?{identity:instance.identity,state:'S'}:null});
  assert.equal(checks,4);assert.equal(clock,1);
});
