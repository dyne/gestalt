import { constants } from 'node:fs';
import { lstat, open, readdir, mkdir, realpath, writeFile } from 'node:fs/promises';
import path from 'node:path';
import { gunzipSync, gzipSync } from 'node:zlib';
import type { ArchiveMember, ArchivePort } from '../contracts.ts';
import { windowsSystem } from './windows-system.ts';

const maximumArchive = 64 * 1024 * 1024, maximumPayload = 256 * 1024 * 1024, maximumMembers = 10000;
interface Entry extends ArchiveMember { readonly data: Buffer }
function name(value: string): string {
  const normalized = value.replace(/\/$/, '');
  if (!normalized || Buffer.byteLength(normalized) > 4096 || /[\\\x00-\x1f\x7f:]/.test(normalized) ||
      normalized.split('/').some(part => !part || part === '.' || part === '..' || /[. ]$/.test(part) ||
        /^(?:con|prn|aux|nul|com[1-9]|lpt[1-9])(?:\.|$)/i.test(part))) throw new Error('Unsafe archive member path');
  return normalized;
}
const field = (header: Buffer, offset: number, length: number): string => {
  const bytes = header.subarray(offset, offset + length), end = bytes.indexOf(0);
  return bytes.subarray(0, end < 0 ? bytes.length : end).toString('utf8');
};
function octal(header: Buffer, offset: number, length: number): number {
  const text = field(header, offset, length).trim();
  if (!/^[0-7]+$/.test(text)) throw new Error('Invalid tar numeric field');
  const value = Number.parseInt(text, 8);
  if (!Number.isSafeInteger(value)) throw new Error('Tar numeric field exceeds supported size');
  return value;
}
function parse(bytes: Buffer): Entry[] {
  if (bytes.length > maximumArchive) throw new Error('Archive exceeds supported size');
  const tar = gunzipSync(bytes, { maxOutputLength: maximumPayload });
  const entries: Entry[] = [], seen = new Map<string, string>();
  let offset = 0, ended = false, headers = 0, nextPath: string | undefined, nextSize: number | undefined;
  while (offset + 512 <= tar.length) {
    const header = tar.subarray(offset, offset + 512); offset += 512;
    if (header.every(byte => byte === 0)) { ended = true; break; }
    let sum = 0;
    for (let index = 0; index < 512; index++) sum += index >= 148 && index < 156 ? 32 : header[index]!;
    if (sum !== octal(header, 148, 8)) throw new Error('Invalid tar header checksum');
    if (++headers > maximumMembers * 3) throw new Error('Archive contains too many headers');
    const type = header[156], storedSize = octal(header, 124, 12);
    if ([120, 103, 76].includes(type!)) {
      if (storedSize > 65536 || offset + storedSize > tar.length) throw new Error('Invalid tar extended header size');
      const data = tar.subarray(offset, offset + storedSize);
      if (type === 76) nextPath = new TextDecoder('utf-8', { fatal: true }).decode(data).replace(/\0$/, '');
      else {
        let cursor = 0;
        while (cursor < data.length) {
          const space = data.indexOf(32, cursor), lengthText = data.subarray(cursor, space).toString('ascii');
          if (space < cursor || !/^[1-9]\d*$/.test(lengthText)) throw new Error('Invalid PAX record');
          const length = Number(lengthText), end = cursor + length;
          if (!Number.isSafeInteger(length) || end > data.length || end <= space + 2 || data[end - 1] !== 10) throw new Error('Invalid PAX record length');
          const text = new TextDecoder('utf-8', { fatal: true }).decode(data.subarray(space + 1, end - 1)), boundary = text.indexOf('=');
          if (boundary < 1) throw new Error('Invalid PAX attribute');
          const key = text.slice(0, boundary), value = text.slice(boundary + 1);
          if (key === 'linkpath' || key.startsWith('GNU.sparse') || key === 'SCHILY.filetype') throw new Error('Archive contains link or special-file metadata');
          if (type === 103 && ['path', 'size'].includes(key)) throw new Error('Global PAX path/size override refused');
          if (key === 'path') nextPath = value;
          if (key === 'size') { if (!/^\d+$/.test(value) || !Number.isSafeInteger(Number(value))) throw new Error('Invalid PAX size'); nextSize = Number(value); }
          // Times, uid/gid, names and extension attributes are inert data: this
          // extractor creates only ordinary private files/directories itself.
          cursor = end;
        }
      }
      offset += Math.ceil(storedSize / 512) * 512; continue;
    }
    const prefix = field(header, 345, 155), member = name(nextPath ?? ((prefix ? prefix + '/' : '') + field(header, 0, 100)));
    const size = nextSize ?? storedSize; nextPath = undefined; nextSize = undefined;
    if (![0, 48, 53].includes(type!)) throw new Error('Archive contains a link or special file');
    if (size > maximumPayload || offset + size > tar.length || (type === 53 && size !== 0)) throw new Error('Invalid tar member size');
    const folded = member.normalize('NFC').toLowerCase();
    if (seen.has(folded)) throw new Error('Duplicate or case-colliding archive member');
    for (const [previous, kind] of seen) {
      if ((folded.startsWith(previous + '/') && kind !== 'directory') ||
          (previous.startsWith(folded + '/') && type !== 53)) throw new Error('Archive file/directory collision');
    }
    const kind = type === 53 ? 'directory' : 'file'; seen.set(folded, kind);
    entries.push({ path: member, kind, data: tar.subarray(offset, offset + size) });
    if (entries.length > maximumMembers) throw new Error('Archive contains too many members');
    offset += Math.ceil(size / 512) * 512;
  }
  if (!ended || nextPath !== undefined || nextSize !== undefined || tar.subarray(offset).some(byte => byte !== 0) || !entries.length) throw new Error('Incomplete or trailing tar payload');
  return entries;
}
async function readArchive(file: string): Promise<Entry[]> {
  const info = await lstat(file);
  if (!info.isFile() || info.isSymbolicLink() || info.nlink !== 1 || info.size > maximumArchive) throw new Error('Archive must be an ordinary bounded file');
  const handle = await open(file, constants.O_RDONLY | (constants.O_NOFOLLOW || 0));
  try {
    const actual = await handle.stat();
    if (actual.dev !== info.dev || actual.ino !== info.ino || actual.size !== info.size) throw new Error('Archive identity changed');
    const bytes = Buffer.allocUnsafe(info.size + 1); let length = 0;
    while (length < bytes.length) {
      const read = await handle.read(bytes, length, bytes.length - length, null);
      if (!read.bytesRead) break;
      length += read.bytesRead;
    }
    if (length !== info.size) throw new Error('Archive size changed');
    return parse(bytes.subarray(0, length));
  } finally { await handle.close(); }
}

/** Pure Node ustar/gzip boundary. All headers are validated before destination
 * creation; links are never followed and extraction has no external interpreter. */
export class NativeArchives implements ArchivePort {
  async list(archive: string): Promise<readonly ArchiveMember[]> {
    return (await readArchive(archive)).map(({ path, kind }) => ({ path, kind }));
  }
  async extract(archive: string, stagingDirectory: string, expectedRoot: string): Promise<void> {
    const entries = await readArchive(archive), root = name(expectedRoot);
    if (root.includes('/') || entries.some(entry => entry.path !== root && !entry.path.startsWith(root + '/')))
      throw new Error('Unexpected archive root');
    const directory = await realpath(stagingDirectory), info = await lstat(stagingDirectory);
    if (directory !== path.resolve(stagingDirectory) || !info.isDirectory() || info.isSymbolicLink() ||
        (process.getuid && (info.uid !== process.getuid() || (info.mode & 0o077)))) throw new Error('Extraction requires a private real staging directory');
    if (process.platform === 'win32') await windowsSystem({ operation: 'assert-private', path: directory });
    if ((await readdir(directory)).length) throw new Error('Extraction requires an empty staging directory');
    const parents = async (relative: string) => {
      let cursor = directory;
      for (const part of relative.split('/').filter(Boolean)) {
        cursor = path.join(cursor, part);
        try { await mkdir(cursor, { mode: 0o700 }); } catch (error) { if ((error as NodeJS.ErrnoException).code !== 'EEXIST') throw error; }
        const parent = await lstat(cursor);
        if (!parent.isDirectory() || parent.isSymbolicLink()) throw new Error('Redirected archive destination');
      }
    };
    for (const entry of entries) {
      if (entry.kind === 'directory') await parents(entry.path);
      else { await parents(path.posix.dirname(entry.path)); await writeFile(path.join(directory, ...entry.path.split('/')), entry.data, { flag: 'wx', mode: 0o600 }); }
    }
  }
  async create(sourceDirectory: string, destination: string): Promise<void> {
    const entries: Buffer[] = []; let total = 0, count = 0;
    const number = (header: Buffer, offset: number, length: number, value: number) => {
      const text = value.toString(8).padStart(length - 1, '0');
      if (text.length >= length) throw new Error('Tar field exceeds supported size');
      header.write(text + '\0', offset, length, 'ascii');
    };
    const visit = async (file: string, relative: string): Promise<void> => {
      const member = name(relative), info = await lstat(file);
      if (info.isSymbolicLink() || (!info.isDirectory() && !info.isFile()) || (info.isFile() && info.nlink !== 1)) throw new Error('Archive source contains a link or special file');
      if (++count > maximumMembers || (total += info.isFile() ? info.size : 0) > maximumPayload / 2) throw new Error('Archive source exceeds supported size');
      const header = Buffer.alloc(512); let short = member, prefix = '';
      if (Buffer.byteLength(short) > 100) {
        const boundaries = [...member.matchAll(/\//g)].map(match => match.index!);
        const split = boundaries.find(index => Buffer.byteLength(member.slice(0, index)) <= 155 && Buffer.byteLength(member.slice(index + 1)) <= 100);
        if (split === undefined) throw new Error('Archive path exceeds ustar prefix limits');
        prefix = member.slice(0, split); short = member.slice(split + 1);
      }
      header.write(short, 0, 100, 'utf8'); header.write(prefix, 345, 155, 'utf8');
      number(header, 100, 8, info.isDirectory() ? 0o700 : 0o600); number(header, 108, 8, 0); number(header, 116, 8, 0);
      number(header, 124, 12, info.isFile() ? info.size : 0); number(header, 136, 12, 0);
      header.fill(32, 148, 156); header[156] = info.isDirectory() ? 53 : 48; header.write('ustar\0', 257); header.write('00', 263);
      number(header, 148, 8, header.reduce((sum, byte) => sum + byte, 0)); entries.push(header);
      if (info.isFile()) {
        const handle = await open(file, constants.O_RDONLY | (constants.O_NOFOLLOW || 0));
        try { const actual = await handle.stat(); if (actual.ino !== info.ino || actual.dev !== info.dev || actual.size !== info.size) throw new Error('Archive source changed');
          const bytes = await handle.readFile(); if (bytes.length !== info.size) throw new Error('Archive source changed');
          entries.push(bytes, Buffer.alloc((512 - bytes.length % 512) % 512));
        } finally { await handle.close(); }
      } else for (const child of (await readdir(file)).sort()) await visit(path.join(file, child), member + '/' + child);
    };
    await visit(sourceDirectory, path.basename(sourceDirectory)); entries.push(Buffer.alloc(1024));
    const compressed = gzipSync(Buffer.concat(entries));
    if (compressed.length > maximumArchive) throw new Error('Archive exceeds supported size');
    await writeFile(destination, compressed, { flag: 'wx', mode: 0o600 });
  }
}
