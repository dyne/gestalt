import fs from 'node:fs';
import path from 'node:path';
import { loadScipIndex, type ScipIndex } from './lib/index.js';

export interface ScipFileInfo {
  path: string;
  language: string;
  size: number;
}

const MAX_PARENT_SEARCH = 5;
const GESTALT_SCIP_DIR = path.join('.gestalt', 'scip');

function isDirectory(filePath: string): boolean {
  try {
    return fs.statSync(filePath).isDirectory();
  } catch {
    return false;
  }
}

function isFile(filePath: string): boolean {
  try {
    return fs.statSync(filePath).isFile();
  } catch {
    return false;
  }
}

function extractLanguageFromFilename(fileName: string): string {
  if (fileName === 'index.scip') {
    return 'default';
  }
  const match = fileName.match(/^index-([^.]+)\.scip$/i);
  if (match) {
    return match[1].toLowerCase();
  }
  return 'unknown';
}

function listScipFiles(directory: string): ScipFileInfo[] {
  let entries: string[] = [];
  try {
    entries = fs.readdirSync(directory);
  } catch {
    return [];
  }

  const files: ScipFileInfo[] = [];
  for (const entry of entries) {
    if (!entry.endsWith('.scip')) {
      continue;
    }
    const filePath = path.join(directory, entry);
    try {
      const stats = fs.statSync(filePath);
      if (!stats.isFile()) {
        continue;
      }
      files.push({
        path: filePath,
        language: extractLanguageFromFilename(entry),
        size: stats.size,
      });
    } catch {
      continue;
    }
  }

  files.sort((left, right) => left.path.localeCompare(right.path));
  return files;
}

function resolveStartDirectory(startDir?: string): string {
  if (!startDir) {
    return process.cwd();
  }
  const resolved = path.resolve(startDir);
  if (isFile(resolved) && resolved.endsWith('.scip')) {
    return path.dirname(resolved);
  }
  return resolved;
}

function findNearestGestaltScipDir(startDir?: string): string | null {
  const resolvedStart = resolveStartDirectory(startDir);

  if (isDirectory(resolvedStart)) {
    const directFiles = listScipFiles(resolvedStart);
    const isGestaltScipDir = resolvedStart.endsWith(GESTALT_SCIP_DIR);
    if (directFiles.length > 0 || isGestaltScipDir) {
      return resolvedStart;
    }
  }

  let currentDir = resolvedStart;
  for (let depth = 0; depth <= MAX_PARENT_SEARCH; depth += 1) {
    const candidate = path.join(currentDir, GESTALT_SCIP_DIR);
    if (isDirectory(candidate)) {
      return candidate;
    }
    const parentDir = path.dirname(currentDir);
    if (parentDir === currentDir) {
      break;
    }
    currentDir = parentDir;
  }

  return null;
}

export function discoverScipFiles(startDir?: string): ScipFileInfo[] {
  if (startDir) {
    const resolvedStart = path.resolve(startDir);
    if (isFile(resolvedStart) && resolvedStart.endsWith('.scip')) {
      const stats = fs.statSync(resolvedStart);
      return [
        {
          path: resolvedStart,
          language: extractLanguageFromFilename(path.basename(resolvedStart)),
          size: stats.size,
        },
      ];
    }
  }

  const scipDir = findNearestGestaltScipDir(startDir);
  if (!scipDir) {
    return [];
  }

  return listScipFiles(scipDir);
}

export function findScipFileByLanguage(language: string, startDir?: string): string | null {
  const normalizedLanguage = language.toLowerCase();
  const files = discoverScipFiles(startDir);
  const match = files.find((file) => file.language === normalizedLanguage);
  return match ? match.path : null;
}

export function loadAllScipFiles(targetPath: string): Map<string, ScipIndex> {
  const resolvedPath = path.resolve(targetPath);

  if (isFile(resolvedPath)) {
    if (!resolvedPath.endsWith('.scip')) {
      throw new Error(`SCIP file must end with .scip: ${resolvedPath}`);
    }
    const language = extractLanguageFromFilename(path.basename(resolvedPath));
    const index = loadScipIndex(resolvedPath);
    return new Map([[language, index]]);
  }

  if (!isDirectory(resolvedPath)) {
    throw new Error(`SCIP path not found: ${resolvedPath}`);
  }

  const files = listScipFiles(resolvedPath);
  const indexes = new Map<string, ScipIndex>();
  for (const file of files) {
    const index = loadScipIndex(file.path);
    indexes.set(file.language, index);
  }
  return indexes;
}
