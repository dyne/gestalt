import assert from 'node:assert/strict';
import path from 'node:path';
import test from 'node:test';
import { skillCatalog } from '../src/cli/skill-profiles/catalog.ts';
import { skillLaunchOptions } from '../src/cli/skill-profiles/launch-options.ts';
import { profileSkillRules, reconcileSkills } from '../src/cli/skill-profiles/model.ts';
import { parseProfileYaml, serializeProfileYaml } from '../src/cli/skill-profiles/yaml.ts';

test('Mobile v1 and v2 YAML round-trip without losing browser consent or disabled skills', () => {
  const skill = { name: 'calendar', path: path.resolve('skills/calendar/SKILL.md'), enabled: false };
  for (const browser of [undefined, false, true]) {
    const capabilities = browser === undefined ? '' : `capabilities:\n  playwright:\n    enabled: ${browser}\n`;
    const source = `version: ${browser === undefined ? 1 : 2}\nname: work\nskills:\n` +
      `  - name: calendar\n    path: ${JSON.stringify(skill.path)}\n    enabled: false\n${capabilities}`;
    const expected = { name: 'work', skills: [skill], ...(browser === undefined ? {} : { browser }) };
    assert.deepEqual(parseProfileYaml(source), expected);
    assert.deepEqual(parseProfileYaml(serializeProfileYaml(expected)), expected);
    assert.throws(() => parseProfileYaml(`${source}name: second\n`), /Invalid skill profile YAML/);
  }
});

test('profiles rebind updated plugin skills, retain missing entries, and leave managed capabilities alone', () => {
  const skill = (name, file, enabled = false) => ({ name, path: path.resolve(file), enabled });
  const old = skill('writing', 'plugins/cache/example/writing/1/skills/writing/SKILL.md', true);
  const current = skill('writing', 'plugins/cache/example/writing/2/skills/writing/SKILL.md');
  const missing = skill('missing', 'skills/absent/SKILL.md', true);
  const fresh = skill('fresh', 'skills/fresh/SKILL.md', true);
  const fixed = skill('gestalt:using-gestalt', 'skills/gestalt/SKILL.md');
  const browser = skill('gestalt:playwright', 'skills/playwright/SKILL.md');
  const discovered = [current, fresh, fixed, browser];
  const reconciled = reconcileSkills(discovered, [old, missing]);
  assert.deepEqual(reconciled.skills, [{ ...current, enabled: true }, { ...fresh, enabled: false }]);
  assert.deepEqual(reconciled.missing, [{ ...missing, enabled: false }]);
  assert.deepEqual(profileSkillRules(discovered, [old, missing]).rules, [
    { path: current.path, enabled: true }, { path: fresh.path, enabled: false }, { path: fixed.path, enabled: true },
  ]);
  assert.deepEqual(profileSkillRules(discovered), {
    rules: discovered.map(({ path }) => ({ path, enabled: path === fixed.path })), missing: [],
  });
});

test('launch strips Gestalt options and keeps native workspace, config and command boundaries', () => {
  const cwd = path.resolve('workspace');
  const input = ['--', '-C', 'sub', '-p', 'codex', '-c', 'model="custom"', 'resume', '--skill-profile=work', '--', 'literal'];
  const prepared = skillLaunchOptions(input, cwd);
  assert.equal(prepared.profile, 'work');
  assert.equal(prepared.cwd, path.join(cwd, 'sub'));
  assert.deepEqual(prepared.discovery, ['--profile', 'codex', '-c', 'model="custom"']);
  assert.deepEqual(prepared.argv, input.filter(argument => argument !== '--skill-profile=work'));
  for (const options of [['exec'], ['--help'], ['--remote', 'ws://localhost:1234'], ['--no-skill-profile'], ['-c', 'skills.config=[]']])
    assert.equal(skillLaunchOptions(options, cwd).prompt, false);
  assert.equal(skillLaunchOptions([], cwd).prompt, true);
  assert.throws(() => skillLaunchOptions(['--skill-profile', 'work', '-c', 'skills.config=[]'], cwd), /another skill override/);
});

test('native catalog preserves names, descriptions, dependency metadata and discovery warnings', () => {
  const cwd = path.resolve('workspace'), file = path.join(cwd, 'skills', 'calendar', 'SKILL.md');
  const catalog = skillCatalog({ data: [{ cwd, errors: [{ message: 'Limited discovery' }], skills: [{
    path: file, name: 'calendar', enabled: true, description: 'Schedule', interface: { displayName: 'Calendar' },
    dependencies: { tools: [{ type: 'mcp', value: 'reminders', description: 'Reminders' }] },
  }] }] }, cwd);
  assert.deepEqual(catalog, { skills: [{ path: file, name: 'calendar', enabled: true, description: 'Schedule', displayName: 'Calendar',
    dependencies: [{ type: 'mcp', value: 'reminders', description: 'Reminders' }] }], warnings: ['Limited discovery'] });
});
