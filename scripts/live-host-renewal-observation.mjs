import assert from 'node:assert/strict';
import {createHash} from 'node:crypto';

export const stable=value=>JSON.stringify(value,(_key,item)=>item&&typeof item==='object'&&!Array.isArray(item)?Object.fromEntries(Object.keys(item).sort().map(key=>[key,item[key]])):item);
const digest=value=>createHash('sha256').update(stable(value)).digest('hex');

// This fixture has no credentials. Null is valid only when Native itself reports
// no account and the actual selected provider explicitly needs no OpenAI auth.
export function controlledObservation(configuration,reply,account){
 assert.equal(reply.modelProvider,'controlled');
 assert.equal(configuration.config.model_providers.controlled.requires_openai_auth,false);
 assert.deepEqual(Object.keys(account).filter(key=>key!=='workspaceRouting').sort(),['account','requiresOpenaiAuth']);
 if(Object.hasOwn(account,'workspaceRouting'))assert.equal(account.workspaceRouting,null);
 assert.equal(account.account,null);assert.equal(account.requiresOpenaiAuth,false);
 const sessionFlags=Object.fromEntries(['model','modelProvider','cwd','approvalPolicy','activePermissionProfile','sandbox','reasoningEffort','runtimeWorkspaceRoots','instructionSources'].filter(key=>Object.hasOwn(reply,key)).map(key=>[key,reply[key]]));
 for(const key of ['model','modelProvider','cwd','approvalPolicy','activePermissionProfile','sandbox','reasoningEffort'])assert.ok(Object.hasOwn(sessionFlags,key));
 return {sessionFlags,accountDigest:digest({providerId:reply.modelProvider,accountId:account.account})};
}

// Trusted producer obligation: inspect actual Native work before unloading it.
// No process-liveness assertion substitutes for any RPC below.
export async function quiesceNative(request,knownThreads){
 const loaded=await request('thread/loaded/list',{limit:64});
 assert.ok(Array.isArray(loaded.data)&&loaded.data.length<=64);assert.equal(loaded.nextCursor,null);
 const threads=[...new Set([...knownThreads,...loaded.data])];assert.ok(threads.length>0&&threads.length<=64);
 const observed=[];
 for(const threadId of threads){
  const {thread}=await request('thread/read',{threadId,includeTurns:true});
  assert.equal(thread.id,threadId);assert.ok(['idle','notLoaded'].includes(thread.status?.type));
  assert.ok(Array.isArray(thread.turns));assert.equal(thread.turns.some(turn=>turn.status==='inProgress'),false);
  const isLoaded=loaded.data.includes(threadId);
  assert.equal(isLoaded,thread.status.type==='idle');
  // Native's terminal RPC requires a loaded thread. For a departed child,
  // require BOTH an actual notLoaded read and absence in its loaded registry.
  if(isLoaded){
   const terminals=await request('thread/backgroundTerminals/list',{threadId,limit:64});
   assert.deepEqual(terminals.data,[]);assert.equal(terminals.nextCursor,null);
  }
  observed.push({threadId,status:thread.status.type,turns:thread.turns.length,terminals:isLoaded?0:'unloaded'});
 }
 for(const threadId of threads.toReversed()){
  const result=await request('thread/unsubscribe',{threadId});
  assert.ok(['unsubscribed','notSubscribed','notLoaded'].includes(result.status));
 }
 const after=await request('thread/loaded/list',{limit:64});
 assert.deepEqual(after.data,[]);assert.equal(after.nextCursor,null);
 return {observed,unloaded:true};
}
