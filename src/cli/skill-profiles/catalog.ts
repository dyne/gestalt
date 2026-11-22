import { spawn } from 'node:child_process';
import { createInterface } from 'node:readline';
import { realpath } from 'node:fs/promises';
import path from 'node:path';
import type { CommandInvocation, CommandStreams } from '../contracts.ts';
import { RuntimeCommands, diagnosticEnvironment } from '../runtime/commands.ts';
import { managedChild } from '../runtime/child.ts';
import { SetupContext, setupPaths } from '../setup/context.ts';
import { manageSkills } from '../runtime/skills-owner.mjs';
import { record, skillKey, validSkillPath, type CatalogSkill } from './model.ts';
import type { Catalog } from './tui.ts';

const string = (value: unknown): string | undefined => typeof value === 'string' ? value : undefined;

/** Convert the native catalog to display metadata and path-keyed skills. */
export function skillCatalog(result: unknown, cwd: string): Catalog {
  const data = record(result)?.data;
  const entry = Array.isArray(data) && data.map(record).find(item => typeof item?.cwd === 'string' && skillKey(item.cwd) === skillKey(cwd));
  if (!entry || !Array.isArray(entry.skills)) throw new Error('Codex did not return a skill catalog for this workspace.');
  const skills = new Map<string, CatalogSkill>();
  for (const value of entry.skills as unknown[]) {
    const skill = record(value);
    if (!skill || typeof skill.name !== 'string' || typeof skill.enabled !== 'boolean' || !validSkillPath(skill.path))
      throw new Error('Codex returned invalid skill metadata.');
    if (skill.name === 'gestalt:xerj') continue;
    const ui = record(skill.interface);
    const optional = { displayName: string(ui?.displayName), description: string(skill.description),
      shortDescription: string(ui?.shortDescription) ?? string(skill.shortDescription), brandColor: string(ui?.brandColor) };
    const tools = record(skill.dependencies)?.tools;
    const dependencies = Array.isArray(tools) ? tools.map(record).flatMap(tool =>
      typeof tool?.type === 'string' && typeof tool.value === 'string'
        ? [{ type: tool.type, value: tool.value, ...(typeof tool.description === 'string' ? { description: tool.description } : {}) }] : []) : [];
    skills.set(skillKey(skill.path), { path: skill.path, name: skill.name, enabled: skill.enabled,
      ...(optional.displayName ? { displayName: optional.displayName } : {}),
      ...(optional.description ? { description: optional.description } : {}),
      ...(optional.shortDescription ? { shortDescription: optional.shortDescription } : {}),
      ...(optional.brandColor ? { brandColor: optional.brandColor } : {}), dependencies });
  }
  const warnings = Array.isArray(entry.errors) ? entry.errors.map(error => string(record(error)?.message) ?? 'Codex reported a skill discovery error.') : [];
  return { skills: [...skills.values()].sort((a, b) => a.path.localeCompare(b.path)), warnings };
}

/** Query a short-lived app server using Gestalt's normal Codex home. */
export async function discoverSkills(invocation: CommandInvocation, streams: CommandStreams, args: readonly string[]): Promise<Catalog> {
  const cwd = await realpath(invocation.cwd);
  const paths = await setupPaths(invocation.environment);
  const environment = { ...invocation.environment, CODEX_HOME: paths.codexHome, GESTALT_HOME: paths.gestaltHome };
  const commands = new RuntimeCommands({ ...invocation, environment }, streams, paths.manager);
  const codex = await commands.resolve('codex');
  if (!codex) throw new Error('codex is required to discover skills.');
  const argv = [...args, 'app-server', '--listen', 'stdio://'];
  const env = diagnosticEnvironment(environment);
  const native = process.platform === 'win32' ? managedChild(commands, codex, argv, { cwd, env, stdio: ['pipe', 'pipe', 'ignore'] }) : undefined;
  const child = native ?? spawn(codex, argv, { cwd, env, stdio: ['pipe', 'pipe', 'ignore'] });
  const closed = new Promise<void>(resolve => child.once('close', resolve));
  const lines = createInterface({ input: child.stdout });
  let timeout: NodeJS.Timeout | undefined;
  let total = 0;
  const read = new Promise<unknown>((resolve, reject) => {
    const unavailable = () => reject(new Error('Could not read skills from the Codex app server.'));
    child.once('error', unavailable);
    child.stdin.once('error', unavailable);
    child.once('close', unavailable);
    timeout = setTimeout(unavailable, 10_000);
    lines.on('line', line => {
      total += line.length;
      if (total > 8 * 1024 * 1024) return unavailable();
      let message;
      try { message = record(JSON.parse(line)); } catch { return; }
      if (message?.id === 1 && message.result) {
        child.stdin.write(`${JSON.stringify({ method: 'initialized', params: {} })}\n`);
        child.stdin.write(`${JSON.stringify({ id: 2, method: 'skills/list', params: { cwds: [cwd], forceReload: true } })}\n`);
      } else if (message?.id === 2 && message.result) resolve(message.result);
      else if (message?.error) unavailable();
    });
    child.stdin.write(`${JSON.stringify({ id: 1, method: 'initialize', params: {
      clientInfo: { name: 'gestalt-skill-profiles', version: '1' }, capabilities: {},
    } })}\n`);
  });
  let catalog: Catalog;
  try { catalog = skillCatalog(await read, cwd); }
  finally {
    clearTimeout(timeout);
    lines.close();
    child.stdin.end();
    if (native) await native.stop();
    else {
      child.kill();
      const force = setTimeout(() => child.kill('SIGKILL'), 1000);
      await closed;
      clearTimeout(force);
    }
  }
  let report = '';
  const context = new SetupContext(invocation, { ...streams, stdout: async bytes => { report += Buffer.from(bytes); } }, paths, '1');
  if (await manageSkills(context, 'status', 'json') !== 0) return catalog;
  const status = record(JSON.parse(report));
  if (status?.verifiedSource !== true || typeof status.managedRoot !== 'string') return catalog;
  const root = path.join(status.managedRoot, 'skills');
  return { ...catalog, skills: await Promise.all(catalog.skills.map(async skill => {
    const actual = await realpath(skill.path).catch(() => skill.path);
    const parts = path.relative(root, actual).split(path.sep);
    return { ...skill, verified: parts.length === 2 && /^[a-z0-9]+(?:-[a-z0-9]+)*$/.test(parts[0] ?? '') && parts[1] === 'SKILL.md' };
  })) };
}
