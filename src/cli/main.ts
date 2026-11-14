#!/usr/bin/env node
import type { Writable } from 'node:stream';
import { runCli } from './dispatch.ts';
import type { Bytes, CommandStreams } from './contracts.ts';

declare const GESTALT_BUILD_VERSION: string;
declare const GESTALT_BUILD_HELP: string;

function write(stream: Writable, bytes: Bytes): Promise<void> {
  return new Promise((resolve, reject) => {
    stream.write(bytes, error => error ? reject(error) : resolve());
  });
}

export async function main(): Promise<number> {
  const streams: CommandStreams = {
    stdin: process.stdin,
    stdout: bytes => write(process.stdout, bytes),
    stderr: bytes => write(process.stderr, bytes),
  };
  return runCli({ argv: process.argv.slice(2), cwd: process.cwd(), environment: process.env }, streams,
    { version: GESTALT_BUILD_VERSION, help: GESTALT_BUILD_HELP });
}

main().then(code => { process.exitCode = code; }).catch(error => {
  console.error(`gestalt: ${error instanceof Error ? error.message : 'CLI failed'}`);
  process.exitCode = 1;
});
