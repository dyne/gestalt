import assert from 'node:assert/strict';
import fs from 'node:fs';
import {createRequire} from 'node:module';
import {mkdtemp,mkdir,readFile,writeFile,readdir,rm,stat} from 'node:fs/promises';
import {tmpdir} from 'node:os';
import {join,resolve} from 'node:path';
import test from 'node:test';
import {sessionScratch} from './session-scratch-fixture.mjs';

const source=await readFile(resolve('public/gestalt'),'utf8');
const program=source.match(/^\/\/ BEGIN SESSION SCRATCH RUNTIME\n([\s\S]*?)^\/\/ END SESSION SCRATCH RUNTIME/m)[1];
const require=createRequire(import.meta.url);
const runtime=(overrides={})=>new Function('require',program+'\nreturn {scratchAllocate,scratchRelease,scratchEnvironment};')(
  name=>name==='node:fs'?{...fs,...overrides}:require(name));
async function fixture(t) {
  const root=await mkdtemp(join(tmpdir(),'containment ')),workspace=join(root,'outer workspace');
  await mkdir(workspace,{mode:0o700});t.after(()=>rm(root,{recursive:true,force:true}));
  return{root,workspace};
}
async function nestedMobile(t) {
  const f=await fixture(t),outer=await sessionScratch(f.workspace,'outer-mobile'),selected=join(outer.context.directory,'inner workspace');
  await mkdir(selected,{mode:0o700});await writeFile(join(selected,'payload'),'retained');
  return{...f,outer,selected,lock:join(outer.context.directory,'.containment-lock')};
}
async function unpublished(workspace){await assert.rejects(stat(join(workspace,'tmp')),{code:'ENOENT'});}

test('ordinary workspace CLI and service publication retains v1 context and existing lifetime locks',async t=>{
  const f=await fixture(t),api=runtime();
  const cli=api.scratchAllocate(f.workspace,'cli-session','cli-controller');
  const service=api.scratchAllocate(f.workspace,'workspace-service','shared-tool');
  for(const own of [cli,service]){
    const env=api.scratchEnvironment({GESTALT_SESSION_TMP_CONTEXT:JSON.stringify(own.context)});
    for(const key of ['TMPDIR','TMP','TEMP'])assert.equal(env[key],own.context.directory);
    assert.ok((await stat(own.lock)).isDirectory());assert.equal(api.scratchRelease(own),true);
  }
  await assert.rejects(stat(join(f.workspace,'.containment-lock')),{code:'ENOENT'});
});

test('nested owner publication locks all attributed ancestors outermost first and releases in reverse',async t=>{
  const f=await nestedMobile(t),first=runtime().scratchAllocate(f.selected,'cli-session','cli-controller');
  const selected=join(first.context.directory,'deeper workspace');await mkdir(selected,{mode:0o700});
  const expected=[f.lock,join(first.context.directory,'.containment-lock')],acquired=[],released=[];let witnessed=false,publishing=false;
  const api=runtime({
    mkdirSync(path,...args){if(path.endsWith('/.containment-lock')&&path.startsWith(f.root))acquired.push(path);return fs.mkdirSync(path,...args);},
    rmdirSync(path,...args){if(path.endsWith('/.containment-lock')&&path.startsWith(f.root))released.push(path);return fs.rmdirSync(path,...args);},
    openSync(path,...args){
      if(publishing&&args[0]==='wx'&&typeof path==='string'&&path.startsWith(join(selected,'tmp/sessions'))&&path.endsWith('/.gestalt-owner.json')){
        witnessed=true;for(const lock of expected)assert.equal(fs.statSync(lock).mode&0o777,0o700);
      }
      return fs.openSync(path,...args);
    },
  });
  let second;publishing=true;
  try{second=api.scratchAllocate(selected,'workspace-service','shared-tool');}finally{publishing=false;}
  assert.ok(witnessed,'marker publication must occur while all ancestor fences are held');
  assert.deepEqual(acquired,expected);assert.deepEqual(released,[...expected].reverse());
  for(const lock of expected)await assert.rejects(stat(lock),{code:'ENOENT'});
  const marker=JSON.parse(await readFile(second.context.markerPath,'utf8'));assert.equal(marker.ownerKind,'workspace-service');
  await stat(first.lock);assert.equal(api.scratchRelease(second),true);assert.equal(api.scratchRelease(first),true);
});

for(const kind of ['cli-session','workspace-service'])test('released '+kind+' ancestor remains attributed during nested publication',async t=>{
  const f=await fixture(t),api=runtime(),outer=api.scratchAllocate(f.workspace,kind,'outer-controller');
  assert.equal(api.scratchRelease(outer),true);
  const selected=join(outer.context.directory,'nested workspace');await mkdir(selected,{mode:0o700});
  const inner=api.scratchAllocate(selected,'cli-session','inner-controller');
  assert.equal(JSON.parse(await readFile(outer.context.markerPath,'utf8')).ownerId,outer.context.ownerId);
  assert.equal(JSON.parse(await readFile(inner.context.markerPath,'utf8')).workspacePath,selected);
  assert.equal(api.scratchRelease(inner),true);
});

test('live or unknown containment claims are never stolen or expired and publish no nested owner',async t=>{
  const f=await nestedMobile(t);await mkdir(f.lock,{mode:0o700});
  const identity=await stat(f.lock),record=join(f.lock,'unknown.json');
  await writeFile(record,'{"pid":2147483646,"createdAt":"2000-01-01T00:00:00.000Z"}',{mode:0o600});
  fs.utimesSync(f.lock,new Date(0),new Date(0));
  for(const kind of ['cli-session','workspace-service'])assert.throws(()=>runtime().scratchAllocate(f.selected,kind,'blocked'),/CONTAINMENT_BUSY/);
  await unpublished(f.selected);assert.equal((await stat(f.lock)).ino,identity.ino);
  assert.equal(await readFile(record,'utf8'),'{"pid":2147483646,"createdAt":"2000-01-01T00:00:00.000Z"}');
});

test('a busy inner ancestor releases only the already acquired outer fence',async t=>{
  const f=await nestedMobile(t),first=runtime().scratchAllocate(f.selected,'cli-session','outer-controller');
  const selected=join(first.context.directory,'deeper workspace'),innerLock=join(first.context.directory,'.containment-lock');
  await mkdir(selected,{mode:0o700});await mkdir(innerLock,{mode:0o700});const identity=await stat(innerLock);
  assert.throws(()=>runtime().scratchAllocate(selected,'workspace-service','blocked'),/CONTAINMENT_BUSY/);
  await unpublished(selected);await assert.rejects(stat(f.lock),{code:'ENOENT'});
  assert.equal((await stat(innerLock)).ino,identity.ino);await stat(first.lock);
});

test('unknown ancestor marker or partial lease provenance fails before publication without repair',async t=>{
  const f=await nestedMobile(t),marker=JSON.parse(await readFile(f.outer.context.markerPath,'utf8'));
  await writeFile(f.outer.context.markerPath,JSON.stringify({...marker,schemaVersion:99}));
  assert.throws(()=>runtime().scratchAllocate(f.selected,'cli-session','blocked'),/INVALID_CONTEXT/);
  await unpublished(f.selected);await assert.rejects(stat(f.lock),{code:'ENOENT'});
  await rm(f.outer.context.markerPath);
  assert.throws(()=>runtime().scratchAllocate(f.selected,'cli-session','unmarked'));
  await unpublished(f.selected);await assert.rejects(stat(f.lock),{code:'ENOENT'});
  await writeFile(f.outer.context.markerPath,JSON.stringify(marker),{mode:0o600});
  const leases=join(f.outer.context.directory,'leases');await mkdir(leases,{mode:0o700});await writeFile(join(leases,'partial.json'),'{',{mode:0o600});
  assert.throws(()=>runtime().scratchAllocate(f.selected,'workspace-service','blocked'));
  await unpublished(f.selected);assert.equal(await readFile(join(leases,'partial.json'),'utf8'),'{');
});

for(const changed of ['createdAt','providerThreads'])test('ancestor provenance revalidation distinguishes '+changed,async t=>{
  const f=await nestedMobile(t),marker=JSON.parse(await readFile(f.outer.context.markerPath,'utf8'));let edited=false;
  const replacement=changed==='createdAt'?{...marker,createdAt:'2000-01-01T00:00:00.000Z'}:
    {...marker,providerThreads:[{provider:'codex',threadId:'observed-thread'}]};
  const api=runtime({mkdirSync(path,...args){
    const result=fs.mkdirSync(path,...args);
    if(path===f.lock&&!edited){edited=true;fs.writeFileSync(f.outer.context.markerPath,JSON.stringify(replacement));}
    return result;
  }});
  if(changed==='createdAt'){
    assert.throws(()=>api.scratchAllocate(f.selected,'cli-session','changed'),/INVALID_CONTEXT/);await unpublished(f.selected);
  }else{const own=api.scratchAllocate(f.selected,'cli-session','associated');assert.equal(api.scratchRelease(own),true);}
  assert.ok(edited);assert.deepEqual(JSON.parse(await readFile(f.outer.context.markerPath,'utf8')),replacement);
  await assert.rejects(stat(f.lock),{code:'ENOENT'});
});

for(const change of ['retire ancestor','replace ancestor','replace workspace','insert alias'])
  test('retirement revalidation rejects '+change+' without resurrecting the selected workspace',async t=>{
    const f=await nestedMobile(t),retired=join(f.root,'retired');let changed=false;
    const api=runtime({mkdirSync(path,...args){
      if(path!==f.lock||changed)return fs.mkdirSync(path,...args);
      changed=true;
      if(change==='retire ancestor'||change==='replace ancestor'){
        fs.renameSync(f.outer.context.directory,retired);
        if(change==='replace ancestor'){
          fs.mkdirSync(f.outer.context.directory,{mode:0o700});
          fs.copyFileSync(join(retired,'.gestalt-owner.json'),f.outer.context.markerPath);
          fs.mkdirSync(f.selected,{mode:0o700});
        }
        return fs.mkdirSync(path,...args);
      }
      const result=fs.mkdirSync(path,...args);fs.renameSync(f.selected,retired);
      if(change==='insert alias')fs.symlinkSync(retired,f.selected);else fs.mkdirSync(f.selected,{mode:0o700});
      return result;
    }});
    assert.throws(()=>api.scratchAllocate(f.selected,'cli-session','retired'));
    assert.ok(changed);await unpublished(f.selected);
    const payload=change.includes('ancestor')?join(retired,'inner workspace/payload'):join(retired,'payload');
    assert.equal(await readFile(payload,'utf8'),'retained');
    if(change==='retire ancestor')await assert.rejects(stat(f.outer.context.directory),{code:'ENOENT'});
  });

test('containment release never removes a replacement fence instance',async t=>{
  const f=await nestedMobile(t),held=join(f.outer.context.directory,'.held-test-fence');let replaced=false,foreign;
  const api=runtime({openSync(path,...args){
    if(typeof path==='string'&&path.startsWith(join(f.selected,'tmp/sessions'))&&path.endsWith('/.gestalt-owner.json')&&!replaced){
      replaced=true;fs.renameSync(f.lock,held);fs.mkdirSync(f.lock,{mode:0o700});foreign=fs.statSync(f.lock).ino;
    }
    return fs.openSync(path,...args);
  }});
  assert.throws(()=>api.scratchAllocate(f.selected,'cli-session','fence-instance'),/CONTAINMENT_FENCE_CHANGED/);
  assert.ok(replaced);assert.equal((await stat(f.lock)).ino,foreign);await stat(held);
  const names=await readdir(join(f.selected,'tmp/sessions'));assert.equal(names.filter(name=>name.startsWith('cli-')).length,1);
});
