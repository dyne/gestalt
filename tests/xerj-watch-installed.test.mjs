import assert from 'node:assert/strict';
import {spawn} from 'node:child_process';
import {copyFile, mkdtemp, mkdir, readFile, writeFile, rm} from 'node:fs/promises';
import {tmpdir} from 'node:os';
import {join, resolve} from 'node:path';
import net from 'node:net';
import test from 'node:test';

const binary = process.env.GESTALT_TEST_XERJ_BINARY;
test('installed XERJ indexes and watches a home root with external persistent journals',
  {skip: !binary, timeout:60000}, async t => {
    const root = await mkdtemp(join(tmpdir(), 'xerj installed watch '));
    const workspace = join(root, 'home'), managed = join(root, 'managed');
    const codexHome = join(workspace, '.codex'), journals = join(root, 'persistent journals');
    await mkdir(codexHome, {recursive:true});
    await mkdir(join(managed, 'xerj'), {recursive:true});
    await copyFile(binary, join(managed, 'xerj/xerj'));
    await writeFile(join(workspace, 'example.txt'), 'Initial source discovery fixture.\n');
    const listener = net.createServer();
    await new Promise(done => listener.listen(0, '127.0.0.1', done));
    const port = listener.address().port;
    await new Promise(done => listener.close(done));
    const env = {PATH:process.env.PATH, HOME:workspace, GESTALT_HOME:managed, CODEX_HOME:codexHome,
      XERJ_URL:`http://127.0.0.1:${port}`, XERJ_DISABLE_FEEDBACK:'true', GESTALT_XERJ_AUTO_INDEX_HOME:journals};
    const children = [];
    const manager = resolve('public/gestalt');
    function launch(args) {
      const child = spawn('bash', [manager, 'xerj', ...args], {env, cwd:workspace, stdio:['ignore','ignore','pipe']});
      const state = {child, done:[], pending:'', exited:false, events:[], warnings:[], files:0};
      state.closed = new Promise((done, reject) => {child.once('error', reject); child.once('close', code => {state.exited=true; done(code);});});
      child.stderr.on('data', bytes => {
        state.pending += bytes;
        const lines = state.pending.split('\n'); state.pending = lines.pop().slice(-16384);
        for (const line of lines) try {
          const event = JSON.parse(line);
          state.events.push([event.event, event.phase]);
          state.events = state.events.slice(-20);
          if (event.event === 'warning') state.warnings.push(String(event.message).replaceAll(root, '[fixture]').slice(0, 500));
          if (event.event === 'done') {
            state.done.push(event.ok === true);
            if (Number.isFinite(event.files)) state.files = event.files;
          }
        } catch { /* Retain only the observable completion contract. */ }
      });
      children.push(state);
      return state;
    }
    t.after(async () => {
      for (const state of children.reverse()) {
        if (state.exited) continue;
        state.child.kill('SIGTERM');
        const timer = setTimeout(() => state.child.kill('SIGKILL'), 2000);
        try {await state.closed;} finally {clearTimeout(timer);}
      }
      await rm(root, {recursive:true, force:true});
    });
    async function until(predicate, message, state) {
      const deadline = Date.now()+20000;
      while (Date.now()<deadline) {
        if (await predicate()) return;
        assert.equal(state.exited, false, message+' (child exited)');
        await new Promise(done => setTimeout(done, 100));
      }
      assert.fail(message+' (deadline): '+JSON.stringify({events:state.events, warnings:state.warnings.slice(-3)}));
    }
    const backend = launch(['--port', String(port), '--embed-mode', 'lexical']);
    await until(async () => {
      try {
        const key = (await readFile(join(codexHome, 'xerj-data/admin.key'), 'utf8')).trim();
        const response = await fetch(env.XERJ_URL+'/_cluster/health', {headers:{Authorization:'ApiKey '+key}, signal:AbortSignal.timeout(500)});
        return response.ok;
      } catch {return false;}
    }, 'disposable authenticated backend must start', backend);
    const watcher = launch(['autoindex', workspace, '--url', env.XERJ_URL, '--watch', '--no-graph', '--no-semantic', '--yes', '--progress', 'json', '--debounce', '100']);
    await until(() => watcher.done.includes(true), 'initial indexing must succeed', watcher);
    assert.ok(watcher.files > 0, 'initial pass must process the source fixture');
    const passes = watcher.done.length;
    await writeFile(join(workspace, 'example.txt'), 'Updated source discovery fixture with new content.\n');
    await until(() => watcher.done.length>passes && watcher.done.at(-1), 'an edit must complete another indexing pass', watcher);
    assert.equal(watcher.exited, false);
    assert.ok(watcher.done.every(Boolean));
  });
