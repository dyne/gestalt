import { randomUUID } from 'node:crypto';
import { mkdir, readFile, readdir, rename, rm, writeFile } from 'node:fs/promises';
import path from 'node:path';
import { absent, optionalStat, type SetupContext } from '../setup/context.ts';
import { profileName, type SkillProfile } from './model.ts';
import type { ProfileList } from './tui.ts';
import { parseProfileYaml, serializeProfileYaml } from './yaml.ts';

/** Store Mobile-compatible profiles in the selected Gestalt home and read the workspace default. */
export class FileSkillProfiles {
  private readonly root: string;
  constructor(private readonly context: SetupContext) {
    this.root = path.join(context.paths.gestaltHome, 'skill-profiles');
  }

  private async checkRoot(): Promise<void> {
    for (const directory of [this.context.paths.gestaltHome, this.root]) {
      const info = await optionalStat(directory);
      if (info && (!info.isDirectory() || info.isSymbolicLink() ||
          (process.getuid && (info.uid !== process.getuid() || (info.mode & 0o022) !== 0))))
        throw new Error(`Skill profiles need an owned directory: ${directory}`);
    }
  }

  /** Ignore missing documents, preserving parse failures as named diagnostics. */
  private async read(file: string): Promise<SkillProfile | undefined> {
    const info = await optionalStat(file);
    if (!info) return;
    if (!info.isFile() || info.isSymbolicLink() || info.nlink !== 1 || info.size > 64 * 1024)
      throw new Error('Expected an ordinary skill-profile file of at most 64 KiB.');
    return parseProfileYaml(await readFile(file, 'utf8'));
  }

  async list(): Promise<ProfileList> {
    await this.checkRoot();
    const profiles: SkillProfile[] = [], warnings: string[] = [];
    const files = await readdir(this.root).catch(error => { if (absent(error)) return []; throw error; });
    for (const file of files.sort()) {
      if (!file.endsWith('.yml') || file.startsWith('.')) continue;
      try {
        const selected = await this.read(path.join(this.root, file));
        if (!selected) continue;
        if (`${selected.name}.yml` !== file) throw new Error('Profile name differs from its filename.');
        profiles.push(selected);
      } catch (error) { warnings.push(`${file}: ${error instanceof Error ? error.message : 'Could not read profile.'}`); }
    }
    return { profiles, warnings };
  }

  async defaults(workspace: string): Promise<SkillProfile | undefined> {
    return this.read(path.join(workspace, 'gestalt-skills.yml'));
  }

  /** Atomically replace the named profile without exposing a partially written YAML file. */
  async save(profile: SkillProfile): Promise<void> {
    const bytes = Buffer.from(serializeProfileYaml(profile));
    if (bytes.length > 64 * 1024) throw new Error('The skill profile exceeds 64 KiB.');
    await this.checkRoot();
    const file = path.join(this.root, `${profileName(profile.name)}.yml`);
    const previous = await optionalStat(file);
    if (previous && (!previous.isFile() || previous.nlink !== 1)) throw new Error('Refusing to overwrite a linked or special profile file.');
    if (process.platform === 'win32') {
      const storage = await this.context.storage(this.context.paths.gestaltHome);
      await storage.createDirectory(this.root);
      await storage.replaceFile(file, bytes);
      return;
    }
    await mkdir(this.root, { recursive: true, mode: 0o700 });
    await this.checkRoot();
    const temporary = path.join(this.root, `.profile-${randomUUID()}.tmp`);
    await writeFile(temporary, bytes, { flag: 'wx', mode: 0o600 });
    try { await rename(temporary, file); }
    finally { await rm(temporary, { force: true }); }
  }

  async delete(name: string): Promise<boolean> {
    await this.checkRoot();
    const file = path.join(this.root, `${profileName(name)}.yml`);
    const current = await optionalStat(file);
    if (!current) return false;
    if (!current.isFile() || current.nlink !== 1) throw new Error('Refusing to delete a linked or special profile file.');
    if (process.platform === 'win32') {
      const storage = await this.context.storage(this.context.paths.gestaltHome);
      await storage.assertPrivate(file, 'file');
    }
    await rm(file);
    return true;
  }
}
