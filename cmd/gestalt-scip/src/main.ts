#!/usr/bin/env node
import { Command } from 'commander';

if (!process.env.SCIP_MODULE_DIR && typeof import.meta !== 'undefined') {
  const { fileURLToPath } = await import('url');
  const { dirname, join } = await import('path');
  const modulePath = fileURLToPath(import.meta.url);
  const moduleDir = dirname(modulePath);
  process.env.SCIP_MODULE_DIR = join(moduleDir, './lib');
}

const program = new Command();

program
  .name('gestalt-scip')
  .description('Query SCIP code intelligence indexes offline')
  .version('0.1.0');

program.parse();
