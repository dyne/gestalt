import * as crypto from 'crypto';
import * as fs from 'fs';
import * as path from 'path';

export interface IndexMetadata {
  created_at: string;
  project_root: string;
  languages?: string[];
  files_hashed: string;
}

const languageExtensions: Record<string, string[]> = {
  go: ['.go'],
  typescript: ['.ts', '.tsx', '.js', '.jsx'],
  python: ['.py'],
};

export function metadataPath(indexPath: string): string {
  return `${indexPath}.meta.json`;
}

export function loadMetadata(indexPath: string): IndexMetadata {
  const payload = fs.readFileSync(metadataPath(indexPath), 'utf8');
  return JSON.parse(payload) as IndexMetadata;
}

export function saveMetadata(indexPath: string, metadata: IndexMetadata): void {
  const target = metadataPath(indexPath);
  fs.mkdirSync(path.dirname(target), { recursive: true });
  const payload = `${JSON.stringify(metadata, null, 2)}\n`;
  fs.writeFileSync(target, payload, { mode: 0o644 });
}

export function buildMetadata(projectRoot: string, languages: string[]): IndexMetadata {
  const root = projectRoot.trim();
  if (!root) {
    throw new Error('project root is required');
  }
  const absRoot = path.resolve(root);
  const normalized = normalizeLanguages(languages);
  const filesHashed = hashSourceFiles(absRoot, normalized);
  return {
    created_at: new Date().toISOString(),
    project_root: absRoot,
    languages: normalized.length > 0 ? normalized : undefined,
    files_hashed: filesHashed,
  };
}

export function hashSourceFiles(root: string, languages: string[]): string {
  const trimmedRoot = root.trim();
  if (!trimmedRoot) {
    throw new Error('root is required');
  }

  const extensions = extensionsForLanguages(languages);
  const files: string[] = [];
  const stack = [trimmedRoot];

  while (stack.length > 0) {
    const current = stack.pop();
    if (!current) continue;
    let entries: fs.Dirent[];
    try {
      entries = fs.readdirSync(current, { withFileTypes: true });
    } catch {
      continue;
    }
    for (const entry of entries) {
      if (entry.isDirectory()) {
        if (shouldSkipDir(entry.name)) {
          continue;
        }
        stack.push(path.join(current, entry.name));
        continue;
      }
      if (!entry.isFile() || entry.isSymbolicLink()) {
        continue;
      }
      const ext = path.extname(entry.name).toLowerCase();
      if (!extensions.has(ext)) {
        continue;
      }
      const fullPath = path.join(current, entry.name);
      const rel = path.relative(trimmedRoot, fullPath).split(path.sep).join('/');
      files.push(rel);
    }
  }

  files.sort();
  const hasher = crypto.createHash('sha256');
  for (const rel of files) {
    hasher.update(rel);
    hasher.update(Buffer.from([0]));
    const fullPath = path.join(trimmedRoot, rel.split('/').join(path.sep));
    const content = fs.readFileSync(fullPath);
    hasher.update(content);
    hasher.update(Buffer.from([0]));
  }

  return hasher.digest('hex');
}

export function recentIndexAge(indexPath: string, thresholdMs: number): { recent: boolean; ageMs: number } {
  let metadata: IndexMetadata;
  try {
    metadata = loadMetadata(indexPath);
  } catch (err) {
    if (isMissingFileError(err)) {
      return { recent: false, ageMs: 0 };
    }
    throw err;
  }
  const createdAt = metadata?.created_at ? new Date(metadata.created_at) : null;
  if (!createdAt || Number.isNaN(createdAt.getTime())) {
    return { recent: false, ageMs: 0 };
  }
  let ageMs = Date.now() - createdAt.getTime();
  if (ageMs < 0) {
    ageMs = 0;
  }
  return { recent: ageMs < thresholdMs, ageMs };
}

function extensionsForLanguages(languages: string[]): Set<string> {
  const extensions = new Set<string>();
  if (languages.length === 0) {
    for (const set of Object.values(languageExtensions)) {
      for (const ext of set) {
        extensions.add(ext);
      }
    }
    return extensions;
  }
  for (const language of languages) {
    const set = languageExtensions[language];
    if (!set) continue;
    for (const ext of set) {
      extensions.add(ext);
    }
  }
  return extensions;
}

function normalizeLanguages(languages: string[]): string[] {
  if (languages.length === 0) {
    return [];
  }
  const seen = new Set<string>();
  const normalized: string[] = [];
  for (const language of languages) {
    const trimmed = language.trim().toLowerCase();
    if (!trimmed) continue;
    if (!languageExtensions[trimmed]) continue;
    if (seen.has(trimmed)) continue;
    seen.add(trimmed);
    normalized.push(trimmed);
  }
  normalized.sort();
  return normalized;
}

function shouldSkipDir(name: string): boolean {
  return name === '.git' || name === '.gestalt' || name === 'node_modules' || name === 'vendor';
}

function isMissingFileError(error: unknown): boolean {
  return error instanceof Error && 'code' in error && (error as NodeJS.ErrnoException).code === 'ENOENT';
}
