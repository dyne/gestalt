import * as fs from 'fs';
import * as path from 'path';
import { scip } from '../bundle/scip.js';
import { detectLanguages, ensureIndexer, runIndexer } from '../lib/indexers.js';

const DEFAULT_OUTPUT = path.join('.gestalt', 'scip', 'index.scip');
const RECENT_THRESHOLD_MS = 10 * 60 * 1000;

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
      await runIndexer(language, repoPath, scipOut, indexerPath);
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
  } catch {
    return null;
  }
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

function fileExists(target: string): boolean {
  try {
    fs.accessSync(target, fs.constants.F_OK);
    return true;
  } catch {
    return false;
  }
}

function isMissingFileError(error: unknown): boolean {
  return error instanceof Error && 'code' in error && (error as NodeJS.ErrnoException).code === 'ENOENT';
}
