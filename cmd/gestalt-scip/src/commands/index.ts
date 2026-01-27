import * as fs from 'fs';
import * as path from 'path';
import * as zlib from 'zlib';
import * as http from 'http';
import * as https from 'https';
import { spawn } from 'child_process';
import { scip } from '../bundle/scip.js';

const DEFAULT_OUTPUT = path.join('.gestalt', 'scip', 'index.scip');
const RECENT_THRESHOLD_MS = 10 * 60 * 1000;

interface IndexerConfig {
  language: string;
  name: string;
  version: string;
  binary: string;
}

const INDEXERS: Record<string, IndexerConfig> = {
  go: {
    language: 'go',
    name: 'scip-go',
    version: 'v0.1.26',
    binary: 'scip-go',
  },
  typescript: {
    language: 'typescript',
    name: 'scip-typescript',
    version: 'v0.3.13',
    binary: 'scip-typescript',
  },
  python: {
    language: 'python',
    name: 'scip-python',
    version: 'v0.3.9',
    binary: 'scip-python',
  },
};

export interface IndexOptions {
  path?: string;
  output?: string;
  force?: boolean;
}

export async function indexCommand(options: IndexOptions): Promise<void> {
  const repoPath = (options.path ?? '.').trim();
  if (!repoPath) {
    throw new Error('Path is required.');
  }

  const outputPath = (options.output ?? DEFAULT_OUTPUT).trim();
  if (!outputPath) {
    throw new Error('Output path is required.');
  }

  const { outputFile, scipDir } = resolveScipOutput(outputPath);
  const repoStat = safeStat(repoPath);
  if (!repoStat) {
    throw new Error(`Path not found: ${repoPath}`);
  }
  if (!repoStat.isDirectory()) {
    throw new Error(`Path is not a directory: ${repoPath}`);
  }

  if (!options.force) {
    try {
      const outputStat = fs.statSync(outputFile);
      if (outputStat) {
        try {
          const { recent, ageMs } = recentIndexAge(outputFile, RECENT_THRESHOLD_MS);
          if (recent) {
            console.warn(
              `Warning: Index was created ${formatDuration(ageMs)} ago. Use --force to re-index.`
            );
            return;
          }
        } catch (err) {
          console.warn(`Warning: Failed to read index metadata: ${formatError(err)}`);
        }
        console.log(`Index exists at ${outputFile}. Use --force to re-index.`);
        return;
      }
    } catch (err) {
      if (!isMissingFileError(err)) {
        throw new Error(`Unable to access output path: ${formatError(err)}`);
      }
    }
  }

  const languages = detectLanguages(repoPath);
  if (languages.length === 0) {
    throw new Error('No supported languages detected.');
  }
  console.log(`Detected languages: ${languages.join(', ')}`);

  const indexerPaths = new Map<string, string>();
  for (const language of languages) {
    console.log(`Checking indexer for ${language}...`);
    try {
      const indexerPath = await ensureIndexer(language, repoPath);
      indexerPaths.set(language, indexerPath);
    } catch (err) {
      console.warn(`Warning: Failed to get indexer for ${language}: ${formatError(err)}`);
    }
  }

  if (indexerPaths.size === 0) {
    throw new Error('No supported languages detected.');
  }

  if (scipDir !== '.') {
    fs.mkdirSync(scipDir, { recursive: true });
  }

  const scipIndexes: string[] = [];
  const indexedLanguages: string[] = [];
  for (const [language, indexerPath] of indexerPaths.entries()) {
    console.log(`Indexing ${language} code...`);
    const scipOut = path.join(scipDir, `index-${language}.scip`);
    try {
      await runIndexer(language, repoPath, indexerPath, scipOut);
      scipIndexes.push(scipOut);
      indexedLanguages.push(language);
    } catch (err) {
      console.warn(`Warning: Indexing ${language} failed: ${formatError(err)}`);
    }
  }

  if (scipIndexes.length === 0) {
    throw new Error('No indexes were generated.');
  }

  buildMergedIndex(scipIndexes, outputFile);

  const projectRoot = path.resolve(repoPath);
  const metadata = buildMetadata(projectRoot, indexedLanguages);
  try {
    saveMetadata(outputFile, metadata);
  } catch (err) {
    console.warn(`Warning: Failed to save index metadata: ${formatError(err)}`);
  }

  console.log('Indexing complete!');
  console.log(`  Index: ${outputFile}`);
}

function resolveScipOutput(outputPath: string): { outputFile: string; scipDir: string } {
  const cleaned = outputPath.trim();
  if (!cleaned) {
    throw new Error('output path is required');
  }

  const stat = safeStat(cleaned);
  if (stat) {
    if (stat.isDirectory()) {
      return {
        outputFile: path.join(cleaned, 'index.scip'),
        scipDir: cleaned,
      };
    }
    if (path.extname(cleaned) !== '.scip') {
      throw new Error('output path must end with .scip or be a directory');
    }
    return { outputFile: cleaned, scipDir: path.dirname(cleaned) };
  }

  const ext = path.extname(cleaned);
  if (!ext) {
    const trimmed = cleaned.replace(/[\\/]+$/, '') || '.';
    return {
      outputFile: path.join(trimmed, 'index.scip'),
      scipDir: trimmed,
    };
  }
  if (ext !== '.scip') {
    throw new Error('output path must end with .scip or be a directory');
  }
  return { outputFile: cleaned, scipDir: path.dirname(cleaned) };
}

function safeStat(target: string): fs.Stats | null {
  try {
    return fs.statSync(target);
  } catch (err) {
    return null;
  }
}

function detectLanguages(root: string): string[] {
  const languages: string[] = [];
  const seen = new Set<string>();

  if (hasMarkerFile(root, 'go.mod') || hasFileExtension(root, ['.go'])) {
    addLanguage(languages, seen, 'go');
  }
  if (
    hasMarkerFile(root, 'package.json') ||
    hasMarkerFile(root, 'tsconfig.json') ||
    hasFileExtension(root, ['.ts', '.tsx', '.js', '.jsx'])
  ) {
    addLanguage(languages, seen, 'typescript');
  }
  if (hasMarkerFile(root, 'setup.py') || hasMarkerFile(root, 'requirements.txt') || hasFileExtension(root, ['.py'])) {
    addLanguage(languages, seen, 'python');
  }

  return languages;
}

function hasMarkerFile(root: string, name: string): boolean {
  return fs.existsSync(path.join(root, name));
}

function hasFileExtension(root: string, extensions: string[]): boolean {
  if (extensions.length === 0) {
    return false;
  }
  const normalized = new Set(extensions.map((ext) => ext.toLowerCase()));
  const stack = [root];
  while (stack.length > 0) {
    const current = stack.pop();
    if (!current) continue;
    let entries: fs.Dirent[];
    try {
      entries = fs.readdirSync(current, { withFileTypes: true });
    } catch (err) {
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
      if (!entry.isFile()) continue;
      const ext = path.extname(entry.name).toLowerCase();
      if (normalized.has(ext)) {
        return true;
      }
    }
  }
  return false;
}

function shouldSkipDir(name: string): boolean {
  return name === '.git' || name === '.gestalt' || name === 'node_modules' || name === 'vendor';
}

function addLanguage(languages: string[], seen: Set<string>, language: string): void {
  if (seen.has(language)) return;
  seen.add(language);
  languages.push(language);
}

async function ensureIndexer(language: string, repoRoot: string): Promise<string> {
  const indexer = INDEXERS[language];
  if (!indexer) {
    throw new Error(`unknown indexer language: ${language}`);
  }
  const existing = findExistingIndexer(indexer, repoRoot);
  if (existing) {
    return existing;
  }
  const targetDir = path.join(repoRoot, '.gestalt', 'scip');
  fs.mkdirSync(targetDir, { recursive: true });
  const destination = path.join(targetDir, resolveBinaryName(indexer.binary));
  const url = indexerURL(indexer);
  await downloadBinary(url, destination);
  return destination;
}

function findExistingIndexer(indexer: IndexerConfig, repoRoot: string): string | null {
  const localPath = path.join(repoRoot, '.gestalt', 'scip');
  for (const candidate of indexerCandidates(indexer.binary)) {
    const candidatePath = path.join(localPath, candidate);
    if (fileExists(candidatePath)) {
      return candidatePath;
    }
  }

  const nodeModules = path.join(repoRoot, 'node_modules', '.bin');
  for (const candidate of indexerCandidates(indexer.binary)) {
    const candidatePath = path.join(nodeModules, candidate);
    if (fileExists(candidatePath)) {
      return candidatePath;
    }
  }

  const onPath = findOnPath(indexer.binary);
  if (onPath) {
    return onPath;
  }
  return null;
}

function findOnPath(binary: string): string | null {
  const pathEnv = process.env.PATH || '';
  const parts = pathEnv.split(path.delimiter).filter(Boolean);
  const candidates = indexerCandidates(binary);
  for (const dir of parts) {
    for (const candidate of candidates) {
      const candidatePath = path.join(dir, candidate);
      if (fileExists(candidatePath)) {
        return candidatePath;
      }
    }
  }
  return null;
}

function indexerCandidates(binary: string): string[] {
  if (process.platform !== 'win32') {
    return [binary];
  }
  const base = binary.replace(/\.exe$/i, '');
  return [binary, `${base}.exe`, `${base}.cmd`, `${base}.ps1`];
}

function resolveBinaryName(binary: string): string {
  if (process.platform !== 'win32') {
    return binary;
  }
  return binary.endsWith('.exe') ? binary : `${binary}.exe`;
}

function indexerURL(indexer: IndexerConfig): string {
  const version = normalizeVersion(indexer.version);
  let asset = '';
  if (indexer.name === 'scip-go') {
    asset = `scip-go_${version}_${goOSName()}_${goArchName()}.tar.gz`;
  } else if (indexer.name === 'scip-typescript') {
    asset = `scip-typescript_${version}_${nodeOSName()}_${nodeArchName()}.tar.gz`;
  } else if (indexer.name === 'scip-python') {
    asset = `scip-python_${version}_${nodeOSName()}_${nodeArchName()}.tar.gz`;
  } else {
    throw new Error(`unknown indexer asset pattern: ${indexer.name}`);
  }
  return `https://github.com/sourcegraph/${indexer.name}/releases/download/${indexer.version}/${asset}`;
}

function normalizeVersion(version: string): string {
  return version.trim().replace(/^v/, '');
}

function goOSName(): string {
  if (process.platform === 'linux') return 'linux';
  if (process.platform === 'darwin') return 'darwin';
  if (process.platform === 'win32') return 'windows';
  throw new Error(`unsupported os: ${process.platform}`);
}

function goArchName(): string {
  if (process.arch === 'x64') return 'amd64';
  if (process.arch === 'arm64') return 'arm64';
  throw new Error(`unsupported arch: ${process.arch}`);
}

function nodeOSName(): string {
  if (process.platform === 'linux') return 'linux';
  if (process.platform === 'darwin') return 'macos';
  if (process.platform === 'win32') return 'windows';
  throw new Error(`unsupported os: ${process.platform}`);
}

function nodeArchName(): string {
  if (process.arch === 'x64') return 'x64';
  if (process.arch === 'arm64') return 'arm64';
  throw new Error(`unsupported arch: ${process.arch}`);
}

async function downloadBinary(url: string, destination: string): Promise<void> {
  let payload: Buffer;
  if (url.startsWith('file://')) {
    payload = fs.readFileSync(url.replace('file://', ''));
  } else {
    payload = await fetchBuffer(url);
  }

  if (url.endsWith('.tar.gz') || url.endsWith('.tgz')) {
    extractTarGz(payload, destination);
    return;
  }
  writeAtomic(payload, destination, 0o755);
}

function fetchBuffer(url: string): Promise<Buffer> {
  return new Promise((resolve, reject) => {
    const client = url.startsWith('https:') ? https : http;
    const request = client.get(url, (response) => {
      if (response.statusCode && response.statusCode >= 300 && response.statusCode < 400 && response.headers.location) {
        const next = new URL(response.headers.location, url).toString();
        response.resume();
        fetchBuffer(next).then(resolve).catch(reject);
        return;
      }
      if (!response.statusCode || response.statusCode < 200 || response.statusCode >= 300) {
        response.resume();
        reject(new Error(`download indexer: unexpected status ${response.statusCode || 'unknown'}`));
        return;
      }
      const chunks: Buffer[] = [];
      response.on('data', (chunk) => chunks.push(Buffer.from(chunk)));
      response.on('end', () => resolve(Buffer.concat(chunks)));
    });
    request.on('error', reject);
  });
}

function extractTarGz(buffer: Buffer, destination: string): void {
  const decompressed = zlib.gunzipSync(buffer);
  const target = path.basename(destination);
  const altTarget = target.endsWith('.exe') ? target.replace(/\.exe$/i, '') : target;
  let offset = 0;
  while (offset + 512 <= decompressed.length) {
    const header = decompressed.subarray(offset, offset + 512);
    if (header.every((byte) => byte === 0)) {
      break;
    }
    const name = header.toString('utf8', 0, 100).replace(/\0.*$/, '');
    const sizeText = header.toString('utf8', 124, 136).replace(/\0.*$/, '').trim();
    const size = sizeText ? Number.parseInt(sizeText, 8) : 0;
    const typeFlag = header[156];
    offset += 512;
    const fileData = decompressed.subarray(offset, offset + size);
    const paddedSize = Math.ceil(size / 512) * 512;

    if ((typeFlag === 0 || typeFlag === 48) && (path.basename(name) === target || path.basename(name) === altTarget)) {
      writeAtomic(fileData, destination, 0o755);
      return;
    }

    offset += paddedSize;
  }
  throw new Error(`indexer archive missing ${target}`);
}

function writeAtomic(payload: Buffer, destination: string, mode: number): void {
  const tempPath = `${destination}.tmp`;
  fs.writeFileSync(tempPath, payload, { mode });
  if (process.platform !== 'win32') {
    fs.chmodSync(tempPath, mode);
  }
  try {
    fs.renameSync(tempPath, destination);
  } catch (err) {
    try {
      fs.unlinkSync(destination);
    } catch (removeErr) {
      if (!isMissingFileError(removeErr)) {
        fs.unlinkSync(tempPath);
        throw removeErr;
      }
    }
    fs.renameSync(tempPath, destination);
  }
}

function isMissingFileError(error: unknown): boolean {
  return error instanceof Error && 'code' in error && (error as NodeJS.ErrnoException).code === 'ENOENT';
}

async function runIndexer(language: string, repoRoot: string, indexerPath: string, outputFile: string): Promise<void> {
  const args = indexerArgs(language, outputFile, repoRoot);
  await runCommand(indexerPath, args, repoRoot);
}

function indexerArgs(language: string, outputFile: string, repoRoot: string): string[] {
  if (language === 'go') {
    return ['--output', outputFile, '--project-root', '.', '--module-root', '.', '--repository-root', '.', '--skip-tests'];
  }
  if (language === 'typescript') {
    const args = ['index', '--output', outputFile];
    const project = ensureTypeScriptProject(repoRoot);
    if (project) {
      args.push(project);
    }
    return args;
  }
  if (language === 'python') {
    return ['index', '--output', outputFile];
  }
  throw new Error(`unsupported language: ${language}`);
}

function ensureTypeScriptProject(repoRoot: string): string | null {
  if (!repoRoot) {
    return null;
  }
  const configPath = path.join(repoRoot, 'tsconfig.json');
  if (fileExists(configPath)) {
    return configPath;
  }
  const scipDir = path.join(repoRoot, '.gestalt', 'scip');
  fs.mkdirSync(scipDir, { recursive: true });
  const generatedPath = path.join(scipDir, 'tsconfig.gestalt.json');
  if (fileExists(generatedPath)) {
    return generatedPath;
  }

  const config = {
    compilerOptions: {
      allowJs: true,
      checkJs: false,
      jsx: 'preserve',
      module: 'ESNext',
      target: 'ES2020',
      noEmit: true,
      skipLibCheck: true,
    },
    include: ['../**/*.ts', '../**/*.tsx', '../**/*.js', '../**/*.jsx'],
    exclude: [
      '../**/node_modules/**',
      '../**/.gestalt/**',
      '../**/vendor/**',
      '../**/.git/**',
      '../**/dist/**',
      '../**/build/**',
      '../**/coverage/**',
    ],
  };
  const payload = `${JSON.stringify(config, null, 2)}\n`;
  fs.writeFileSync(generatedPath, payload, { mode: 0o644 });
  return generatedPath;
}

function fileExists(target: string): boolean {
  try {
    fs.accessSync(target, fs.constants.F_OK);
    return true;
  } catch {
    return false;
  }
}

async function runCommand(command: string, args: string[], cwd: string): Promise<string> {
  return new Promise((resolve, reject) => {
    const child = spawn(command, args, { cwd, stdio: 'pipe' });
    let output = '';

    if (child.stdout) {
      child.stdout.on('data', (data) => {
        output += data.toString();
      });
    }
    if (child.stderr) {
      child.stderr.on('data', (data) => {
        output += data.toString();
      });
    }

    child.on('error', (err) => {
      reject(err);
    });
    child.on('close', (code) => {
      if (code === 0) {
        resolve(output.trim());
      } else {
        reject(new Error(output.trim() || `exit code ${code}`));
      }
    });
  });
}

function buildMergedIndex(inputs: string[], outputPath: string): void {
  if (inputs.length === 0) {
    throw new Error('no scip indexes to merge');
  }

  const outputAbsolute = path.resolve(outputPath);
  const tempPath = `${outputPath}.tmp`;
  if (inputs.length === 1) {
    const inputAbsolute = path.resolve(inputs[0]);
    if (inputAbsolute === outputAbsolute) {
      return;
    }
    const payload = fs.readFileSync(inputs[0]);
    fs.writeFileSync(tempPath, payload, { mode: 0o644 });
  } else {
    mergeIndexes(inputs, tempPath);
  }

  try {
    fs.renameSync(tempPath, outputPath);
  } catch (err) {
    if (!isMissingFileError(err)) {
      fs.unlinkSync(tempPath);
      throw err;
    }
    fs.renameSync(tempPath, outputPath);
  }
}

function mergeIndexes(inputs: string[], outputPath: string): void {
  const merged = new scip.Index();
  const documents: Array<InstanceType<typeof scip.Document>> = [];
  const externalSymbols: Array<InstanceType<typeof scip.SymbolInformation>> = [];
  const documentPaths = new Set<string>();
  const symbolIds = new Set<string>();

  for (const input of inputs) {
    const payload = fs.readFileSync(input);
    const index = scip.Index.deserializeBinary(payload);

    if (!merged.metadata && index.metadata) {
      merged.metadata = index.metadata;
    }

    for (const doc of index.documents) {
      const relativePath = doc.relative_path;
      if (!relativePath) {
        throw new Error(`document with empty relative path in ${input}`);
      }
      if (documentPaths.has(relativePath)) {
        throw new Error(`duplicate document path ${relativePath} in ${input}`);
      }
      documentPaths.add(relativePath);
      documents.push(doc);
    }

    for (const symbol of index.external_symbols) {
      const symbolId = symbol.symbol;
      if (!symbolId) {
        continue;
      }
      if (symbolIds.has(symbolId)) {
        continue;
      }
      symbolIds.add(symbolId);
      externalSymbols.push(symbol);
    }
  }

  merged.documents = documents;
  merged.external_symbols = externalSymbols;

  fs.mkdirSync(path.dirname(outputPath), { recursive: true });
  const payload = Buffer.from(merged.serializeBinary());
  fs.writeFileSync(outputPath, payload, { mode: 0o644 });
}

function recentIndexAge(indexPath: string, thresholdMs: number): { recent: boolean; ageMs: number } {
  const metaPath = metadataPath(indexPath);
  if (!fileExists(metaPath)) {
    return { recent: false, ageMs: 0 };
  }
  const raw = fs.readFileSync(metaPath, 'utf8');
  const meta = JSON.parse(raw);
  const createdAt = meta?.created_at ? new Date(meta.created_at) : null;
  if (!createdAt || Number.isNaN(createdAt.getTime())) {
    return { recent: false, ageMs: 0 };
  }
  let ageMs = Date.now() - createdAt.getTime();
  if (ageMs < 0) {
    ageMs = 0;
  }
  return { recent: ageMs < thresholdMs, ageMs };
}

function metadataPath(indexPath: string): string {
  return `${indexPath}.meta.json`;
}

function buildMetadata(projectRoot: string, languages: string[]): Record<string, unknown> {
  return {
    created_at: new Date().toISOString(),
    project_root: projectRoot,
    languages: [...languages],
    files_hashed: '',
  };
}

function saveMetadata(indexPath: string, metadata: Record<string, unknown>): void {
  const metaPath = metadataPath(indexPath);
  fs.mkdirSync(path.dirname(metaPath), { recursive: true });
  const payload = `${JSON.stringify(metadata, null, 2)}\n`;
  fs.writeFileSync(metaPath, payload, { mode: 0o644 });
}

function formatDuration(ms: number): string {
  const totalSeconds = Math.round(ms / 1000);
  const minutes = Math.floor(totalSeconds / 60);
  const seconds = totalSeconds % 60;
  if (minutes > 0) {
    return `${minutes}m${seconds}s`;
  }
  return `${seconds}s`;
}

function formatError(err: unknown): string {
  if (err instanceof Error) {
    return err.message;
  }
  return String(err);
}
